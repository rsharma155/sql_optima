package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain"
	"github.com/rsharma155/sql_optima/internal/explain/analyzer"
	"github.com/rsharma155/sql_optima/internal/explain/parser"
	"github.com/rsharma155/sql_optima/internal/explain/types"
)

const maxExplainPlanBodyBytes = 512 * 1024

var (
	pgExplainStdAnalyzer *analyzer.Analyzer
	pgExplainOptAnalyzer *analyzer.OptimizationAnalyzer
)

func init() {
	pgExplainStdAnalyzer = analyzer.New()
	pgExplainOptAnalyzer = analyzer.NewOptimizationAnalyzer()
}

type pgExplainRequest struct {
	Query string `json:"query,omitempty"`
	Plan  any    `json:"plan"`
}

func writeExplainJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// requireJSONPlanBytes normalizes EXPLAIN (FORMAT JSON) payloads and rejects text plans.
func requireJSONPlanBytes(plan any) ([]byte, error) {
	switch p := plan.(type) {
	case nil:
		return nil, fmt.Errorf("plan is required")
	case string:
		trim := strings.TrimSpace(p)
		if trim == "" {
			return nil, fmt.Errorf("plan is empty")
		}
		if !strings.HasPrefix(trim, "{") && !strings.HasPrefix(trim, "[") {
			return nil, fmt.Errorf("only JSON execution plans are supported: use EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) and paste the JSON output, not the text tree")
		}
		return normalizeExplainJSONPayload([]byte(trim))
	case map[string]interface{}:
		b, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		return normalizeExplainJSONPayload(b)
	case []interface{}:
		b, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		return normalizeExplainJSONPayload(b)
	default:
		return nil, fmt.Errorf("plan must be a JSON object or array")
	}
}

func parseExplainPlanInputJSON(input any) (*types.Plan, string, error) {
	raw, err := requireJSONPlanBytes(input)
	if err != nil {
		return nil, "", err
	}
	s := string(raw)
	pl, err := parser.ParseJSON(s)
	return pl, s, err
}

// normalizeExplainJSONPayload unwraps a top-level single-element array from EXPLAIN (FORMAT JSON).
// Other shapes (wrapped execution_plan_json, Title Case keys) are handled in parser.NormalizePostgresExplainJSON.
func normalizeExplainJSONPayload(raw []byte) ([]byte, error) {
	var asArray []any
	if err := json.Unmarshal(raw, &asArray); err == nil {
		if len(asArray) == 0 {
			return nil, fmt.Errorf("JSON plan array is empty")
		}
		if len(asArray) == 1 {
			return json.Marshal(asArray[0])
		}
		// Some tools may return multiple statements; keep as-is and let parser error clearly.
		return raw, nil
	}
	// Not an array; keep as-is.
	return raw, nil
}

// PgExplainAnalyze parses EXPLAIN FORMAT JSON and returns findings + summary.
func PgExplainAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeExplainJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxExplainPlanBodyBytes)

	var req pgExplainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "invalid JSON body"})
		return
	}

	plan, raw, err := parseExplainPlanInputJSON(req.Plan)
	if err != nil {
		writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": err.Error()})
		return
	}
	result := pgExplainStdAnalyzer.Analyze(plan)
	flat := result.FlattenNodes()
	result.Plan.Plan = result.PlanTree
	queryText := strings.TrimSpace(req.Query)
	if queryText == "" {
		queryText = strings.TrimSpace(result.Plan.Query)
	}
	result.Query = queryText
	result.RawPlan = raw

	g := explain.BuildPlanGraph(result.Plan.Plan)
	bundle := explain.SQLContextBundle{Disclaimer: explain.SQLContextDisclaimer}
	if queryText != "" {
		bundle = explain.AugmentFindingsWithSQL(queryText, result.Findings, flat)
	}
	bundle.HeuristicInsights = explain.BuildHeuristicPlanInsights(&result.Plan.Plan, queryText)

	resp := map[string]any{
		"success":      true,
		"result":       result,
		"plan_graph":   g,
		"plan_mermaid": explain.MermaidFlowchart(g),
		"sql_context":  bundle,
	}

	writeExplainJSON(w, http.StatusOK, resp)
}

// PgExplainOptimize returns a structured optimization report for the given plan.
func PgExplainOptimize(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			writeExplainJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": fmt.Sprintf("panic: %v", err)})
		}
	}()

	if r.Method != http.MethodPost {
		writeExplainJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxExplainPlanBodyBytes)

	var req pgExplainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "invalid JSON body"})
		return
	}

	plan, _, err := parseExplainPlanInputJSON(req.Plan)
	if err != nil {
		writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": err.Error()})
		return
	}
	queryText := strings.TrimSpace(req.Query)
	if queryText == "" {
		queryText = strings.TrimSpace(plan.Query)
	}
	plan.Query = queryText

	aresult := pgExplainStdAnalyzer.Analyze(plan)
	flat := aresult.FlattenNodes()
	aresult.Plan.Plan = aresult.PlanTree
	report := pgExplainOptAnalyzer.GenerateOptimizationReport(&aresult.Plan, aresult.Findings)

	g := explain.BuildPlanGraph(aresult.Plan.Plan)
	bundle := explain.SQLContextBundle{Disclaimer: explain.SQLContextDisclaimer}
	if queryText != "" {
		bundle = explain.AugmentFindingsWithSQL(queryText, aresult.Findings, flat)
	}
	bundle.HeuristicInsights = explain.BuildHeuristicPlanInsights(&aresult.Plan.Plan, queryText)

	resp := map[string]any{
		"success":           true,
		"report":            report,
		"plan_root":         aresult.Plan.Plan,
		"plan_graph":        g,
		"plan_mermaid":      explain.MermaidFlowchart(g),
		"analyzer_findings": aresult.Findings,
		"plan_meta": map[string]float64{
			"planning_time_ms":  aresult.Plan.PlanningTime,
			"execution_time_ms": aresult.Plan.ExecutionTime,
		},
		"sql_context": bundle,
	}

	writeExplainJSON(w, http.StatusOK, resp)
}
