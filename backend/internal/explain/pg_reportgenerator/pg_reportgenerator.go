// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Generate deterministic EXPLAIN performance reports matching explain_analyze_report_layout.md.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package pg_reportgenerator

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain/pg_plandiagnostics"
	"github.com/rsharma155/sql_optima/internal/explain/pg_planmetrics"
	"github.com/rsharma155/sql_optima/internal/explain/pg_planparser"
	"github.com/rsharma155/sql_optima/internal/explain/pg_planrules"
	"github.com/rsharma155/sql_optima/internal/explain/types"
)

// Report matches OUTPUT CONTRACT in explain_analyze_report_layout.md (10 JSON arrays).
type Report struct {
	ExecutionSummary      []ExecutionSummaryRow      `json:"execution_summary"`
	TimeBreakdown         []TimeBreakdownRow         `json:"time_breakdown"`
	TopNodes              []TopNodeRow               `json:"top_nodes"`
	Cardinality             []CardinalityRow           `json:"cardinality"`
	MemoryDisk              []MemoryDiskRow            `json:"memory_disk"`
	TableAccess             []TableAccessRow           `json:"table_access"`
	JoinAnalysis            []JoinAnalysisRow          `json:"join_analysis"`
	Findings                []FindingGridRow           `json:"findings"`
	IndexOpportunities      []IndexOpportunityGridRow  `json:"index_opportunities"`
	TuningRecommendations   []TuningRecommendationGridRow `json:"tuning_recommendations"`
}

type ExecutionSummaryRow struct {
	ExecutionTimeMs          float64 `json:"execution_time_ms"`
	PlanningTimeMs             float64 `json:"planning_time_ms"`
	JitTimeMs                  *float64 `json:"jit_time_ms,omitempty"`
	TotalPlanNodes             int     `json:"total_plan_nodes"`
	MaxPlanDepth               int     `json:"max_plan_depth"`
	TotalRowsProcessed         int64   `json:"total_rows_processed"`
	FinalRowsReturned          int     `json:"final_rows_returned"`
	ParallelWorkersPlanned     int     `json:"parallel_workers_planned"`
	ParallelWorkersLaunched    int     `json:"parallel_workers_launched"`
	PrimaryBottleneck          string  `json:"primary_bottleneck"`
}

type TimeBreakdownRow struct {
	Rank           int     `json:"rank"`
	OperationType  string  `json:"operation_type"`
	NodeCount      int     `json:"node_count"`
	TotalTimeMs    float64 `json:"total_time_ms"`
	TimePercent    float64 `json:"time_percent"`
}

type TopNodeRow struct {
	Rank             int     `json:"rank"`
	NodeType         string  `json:"node_type"`
	RelationName     string  `json:"relation_name,omitempty"`
	ParentNodeType   string  `json:"parent_node_type,omitempty"`
	DepthLevel       int     `json:"depth_level"`
	ActualRows       int     `json:"actual_rows"`
	ActualLoops      int     `json:"actual_loops"`
	RowsProcessed    int64   `json:"rows_processed"`
	ExecutionTimeMs  float64 `json:"execution_time_ms"`
	TimePercent      float64 `json:"time_percent"`
}

type CardinalityRow struct {
	NodeType            string  `json:"node_type"`
	RelationName        string  `json:"relation_name,omitempty"`
	EstimatedRows       int     `json:"estimated_rows"`
	ActualRows          int     `json:"actual_rows"`
	ErrorRatio          float64 `json:"error_ratio"`
	EstimationQuality   string  `json:"estimation_quality"`
}

type MemoryDiskRow struct {
	MemoryPressureLevel string  `json:"memory_pressure_level"`
	DiskSortDetected    bool    `json:"disk_sort_detected"`
	HashSpillDetected   bool    `json:"hash_spill_detected"`
	TempBlocksRead      int     `json:"temp_blocks_read"`
	TempBlocksWritten   int     `json:"temp_blocks_written"`
	LargestSortSpaceMb  float64 `json:"largest_sort_space_mb"`
	PeakHashMemoryMb    float64 `json:"peak_hash_memory_mb"`
}

type TableAccessRow struct {
	RelationName    string `json:"relation_name"`
	ScanType        string `json:"scan_type"`
	NodeCount       int    `json:"node_count"`
	TotalRowsRead   int64  `json:"total_rows_read"`
	ParallelUsed    bool   `json:"parallel_used"`
	LargeScanFlag   bool   `json:"large_scan_flag"`
}

type JoinAnalysisRow struct {
	JoinType       string  `json:"join_type"`
	NodeCount      int     `json:"node_count"`
	TotalTimeMs    float64 `json:"total_time_ms"`
	RowsJoined     int64   `json:"rows_joined"`
	JoinEfficiency string  `json:"join_efficiency"`
}

type FindingGridRow struct {
	Severity        string `json:"severity"`
	Category        string `json:"category"`
	FindingCode     string `json:"finding_code"`
	FindingSummary  string `json:"finding_summary"`
	Evidence        string `json:"evidence"`
}

type IndexOpportunityGridRow struct {
	Priority         string   `json:"priority"`
	OpportunityType  string   `json:"opportunity_type"`
	TableName        string   `json:"table_name"`
	ColumnsInvolved  []string `json:"columns_involved"`
	Reason           string   `json:"reason"`
}

type TuningRecommendationGridRow struct {
	PriorityRank             int     `json:"priority_rank"`
	RecommendationCategory   string  `json:"recommendation_category"`
	ActionSummary            string  `json:"action_summary"`
	ExpectedImpact           string  `json:"expected_impact"`
	ConfidenceScore          float64 `json:"confidence_score"`
}

type Options struct {
	Rules pg_planrules.Options
}

func DefaultOptions() Options {
	return Options{Rules: pg_planrules.DefaultOptions()}
}

// Generate runs the analysis pipeline and returns the 10-table layout contract.
func Generate(plan *types.Plan, opts Options) *Report {
	if opts.Rules.LargeSeqScanRowThreshold <= 0 {
		opts = DefaultOptions()
	}
	nodes := pg_planparser.FlattenPlan(plan)
	totalExec := plan.ExecutionTime
	planning := plan.PlanningTime
	jitMs := jitTimeMs(plan)

	g := pg_planmetrics.ComputeGlobal(nodes, planning, totalExec, derefOrZero(jitMs))

	cats := pg_planmetrics.CategoryTimeAttribution(nodes, totalExec)
	cardSummary := pg_planmetrics.Cardinality(nodes)
	bn := pg_plandiagnostics.ClassifyBottleneck(nodes, totalExec, cats)
	findings := pg_planrules.DetectFindings(nodes, totalExec, cardSummary, opts.Rules)

	byID := make(map[int]pg_planparser.FlatNode, len(nodes))
	for _, n := range nodes {
		byID[n.NodeID] = n
	}

	execSummary := buildExecutionSummary(nodes, g, totalExec, planning, jitMs, bn)
	timeBreak := buildTimeBreakdown(nodes, totalExec)
	top := buildTopNodes(nodes, byID, totalExec, 25)
	cardRows := buildCardinalityRows(nodes)
	memDisk := buildMemoryDisk(nodes, findings)
	tableAcc := buildTableAccess(nodes, opts.Rules.LargeSeqScanRowThreshold)
	joinRows := buildJoinAnalysis(nodes, totalExec)
	findGrid := buildFindingGrid(findings)
	idxGrid := buildIndexOpportunityGrid(inferIndexOpportunities(nodes), findings)
	tuneGrid := buildTuningGrid(bn, findings, idxGrid)

	return &Report{
		ExecutionSummary:    execSummary,
		TimeBreakdown:       timeBreak,
		TopNodes:            top,
		Cardinality:           cardRows,
		MemoryDisk:            memDisk,
		TableAccess:           tableAcc,
		JoinAnalysis:          joinRows,
		Findings:              findGrid,
		IndexOpportunities:    idxGrid,
		TuningRecommendations: tuneGrid,
	}
}

func jitTimeMs(plan *types.Plan) *float64 {
	if plan == nil || plan.Jit == nil {
		return nil
	}
	// types.JitInfo stores counts only; real JIT timing lives in raw JSON. Omit when unknown.
	return nil
}

func derefOrZero(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func buildExecutionSummary(nodes []pg_planparser.FlatNode, g pg_planmetrics.GlobalMetrics, totalExec, planning float64, jit *float64, bn pg_plandiagnostics.Bottleneck) []ExecutionSummaryRow {
	var maxRows int64
	for _, n := range nodes {
		if n.RowsProcessed > maxRows {
			maxRows = n.RowsProcessed
		}
	}
	finalRows := 0
	if len(nodes) > 0 {
		finalRows = nodes[0].ActualRows
	}
	pwPlanned, pwLaunched := gatherWorkerTotals(nodes)
	row := ExecutionSummaryRow{
		ExecutionTimeMs:       totalExec,
		PlanningTimeMs:        planning,
		TotalPlanNodes:        len(nodes),
		MaxPlanDepth:          g.MaxPlanDepth,
		TotalRowsProcessed:    maxRows,
		FinalRowsReturned:     finalRows,
		ParallelWorkersPlanned: pwPlanned,
		ParallelWorkersLaunched: pwLaunched,
		PrimaryBottleneck:     normalizePrimaryBottleneck(bn.Primary),
	}
	if jit != nil {
		row.JitTimeMs = jit
	}
	return []ExecutionSummaryRow{row}
}

func gatherWorkerTotals(nodes []pg_planparser.FlatNode) (planned, launched int) {
	for _, n := range nodes {
		nt := strings.ToLower(n.NodeType)
		if strings.Contains(nt, "gather") {
			planned += n.WorkersPlanned
			launched += n.WorkersLaunched
		}
	}
	return planned, launched
}

func normalizePrimaryBottleneck(b string) string {
	switch strings.ToLower(strings.TrimSpace(b)) {
	case "sort", "join", "scan", "memory", "cpu", "mixed":
		return strings.ToLower(strings.TrimSpace(b))
	case "parallel":
		return "mixed"
	default:
		return "mixed"
	}
}

func buildTimeBreakdown(nodes []pg_planparser.FlatNode, totalExec float64) []TimeBreakdownRow {
	type agg struct {
		ms    float64
		count int
	}
	m := map[string]*agg{}
	for _, n := range nodes {
		lbl := pg_planmetrics.CategoryLabelForNodeType(n.NodeType)
		a := m[lbl]
		if a == nil {
			a = &agg{}
			m[lbl] = a
		}
		a.ms += n.NodeExecutionTime
		a.count++
	}
	rows := make([]TimeBreakdownRow, 0, len(m))
	for op, a := range m {
		rows = append(rows, TimeBreakdownRow{
			OperationType: op,
			NodeCount:     a.count,
			TotalTimeMs:   a.ms,
			TimePercent:   pg_planmetrics.NodeTimePercent(a.ms, totalExec),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].TotalTimeMs != rows[j].TotalTimeMs {
			return rows[i].TotalTimeMs > rows[j].TotalTimeMs
		}
		return rows[i].OperationType < rows[j].OperationType
	})
	for i := range rows {
		rows[i].Rank = i + 1
	}
	return rows
}

func buildTopNodes(nodes []pg_planparser.FlatNode, byID map[int]pg_planparser.FlatNode, totalExec float64, limit int) []TopNodeRow {
	if limit <= 0 {
		limit = 25
	}
	cp := append([]pg_planparser.FlatNode(nil), nodes...)
	sort.SliceStable(cp, func(i, j int) bool {
		if cp[i].NodeExecutionTime != cp[j].NodeExecutionTime {
			return cp[i].NodeExecutionTime > cp[j].NodeExecutionTime
		}
		return cp[i].NodeID < cp[j].NodeID
	})
	if len(cp) > limit {
		cp = cp[:limit]
	}
	out := make([]TopNodeRow, 0, len(cp))
	for i, n := range cp {
		parentType := ""
		if n.ParentNodeID > 0 {
			if p, ok := byID[n.ParentNodeID]; ok {
				parentType = p.NodeType
			}
		}
		loops := n.ActualLoops
		if loops <= 0 {
			loops = 1
		}
		out = append(out, TopNodeRow{
			Rank:            i + 1,
			NodeType:        n.NodeType,
			RelationName:    n.RelationName,
			ParentNodeType:  parentType,
			DepthLevel:      n.DepthLevel,
			ActualRows:      n.ActualRows,
			ActualLoops:     loops,
			RowsProcessed:   n.RowsProcessed,
			ExecutionTimeMs: n.NodeExecutionTime,
			TimePercent:     pg_planmetrics.NodeTimePercent(n.NodeExecutionTime, totalExec),
		})
	}
	return out
}

func buildCardinalityRows(nodes []pg_planparser.FlatNode) []CardinalityRow {
	var out []CardinalityRow
	for _, n := range nodes {
		if n.PlanRows <= 0 || n.ActualRows < 0 {
			continue
		}
		ratio := float64(n.ActualRows) / float64(n.PlanRows)
		if !isFinite(ratio) || ratio <= 0 {
			continue
		}
		q := estimationQualityDoc(ratio)
		out = append(out, CardinalityRow{
			NodeType:          n.NodeType,
			RelationName:      n.RelationName,
			EstimatedRows:     n.PlanRows,
			ActualRows:        n.ActualRows,
			ErrorRatio:        round4(ratio),
			EstimationQuality: q,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		si := severityForRatio(out[i].ErrorRatio)
		sj := severityForRatio(out[j].ErrorRatio)
		if si != sj {
			return si > sj
		}
		return out[i].ErrorRatio > out[j].ErrorRatio
	})
	if len(out) > 30 {
		out = out[:30]
	}
	return out
}

func estimationQualityDoc(ratio float64) string {
	if ratio > 10 || ratio < 0.1 {
		return "severe"
	}
	if ratio >= 0.5 && ratio <= 2 {
		return "accurate"
	}
	return "poor"
}

func severityForRatio(r float64) float64 {
	if r >= 1 {
		return r
	}
	return 1 / r
}

func buildMemoryDisk(nodes []pg_planparser.FlatNode, findings []pg_planrules.Finding) []MemoryDiskRow {
	var tempRead, tempWrite int
	var maxSortKB float64
	var maxHashKB float64
	diskSort := false
	hashSpill := false
	for _, n := range nodes {
		tempRead += n.TempReadBlocks
		tempWrite += n.TempWrittenBlocks
		if strings.Contains(strings.ToLower(n.SortMethod), "external") || strings.EqualFold(n.SortSpaceType, "Disk") {
			diskSort = true
		}
		if n.HashBatches > 1 {
			hashSpill = true
		}
		if n.SortSpaceUsed > 0 {
			kb := float64(n.SortSpaceUsed)
			if kb > maxSortKB {
				maxSortKB = kb
			}
		}
		if n.PeakMemoryUsage > 0 {
			kb := float64(n.PeakMemoryUsage)
			if kb > maxHashKB {
				maxHashKB = kb
			}
		}
	}
	level := "low"
	if diskSort || hashSpill || tempWrite > 0 {
		level = "high"
	} else if tempRead > 0 || maxHashKB > 0 {
		level = "medium"
	}
	// PostgreSQL reports sort space used in kB for many versions.
	return []MemoryDiskRow{{
		MemoryPressureLevel: level,
		DiskSortDetected:    diskSort,
		HashSpillDetected:   hashSpill,
		TempBlocksRead:      tempRead,
		TempBlocksWritten:   tempWrite,
		LargestSortSpaceMb:  round4(maxSortKB / 1024.0),
		PeakHashMemoryMb:    round4(maxHashKB / 1024.0),
	}}
}

func scanTypeLabel(nodeType string) string {
	t := strings.ToLower(nodeType)
	switch {
	case strings.Contains(t, "seq scan"):
		return "Seq"
	case strings.Contains(t, "index only scan"):
		return "Index"
	case strings.Contains(t, "index scan"):
		return "Index"
	case strings.Contains(t, "bitmap heap scan"), strings.Contains(t, "bitmap index scan"):
		return "Bitmap"
	default:
		return ""
	}
}

func buildTableAccess(nodes []pg_planparser.FlatNode, largeScanThreshold int64) []TableAccessRow {
	type agg struct {
		count    int
		rows     int64
		parallel bool
		large    bool
	}
	type pair struct {
		rel string
		st  string
	}
	m := map[pair]*agg{}
	for _, n := range nodes {
		st := scanTypeLabel(n.NodeType)
		if st == "" {
			continue
		}
		rel := strings.TrimSpace(n.RelationName)
		if rel == "" {
			rel = "(unknown)"
		}
		k := pair{rel: rel, st: st}
		a := m[k]
		if a == nil {
			a = &agg{}
			m[k] = a
		}
		a.count++
		a.rows += n.RowsProcessed
		if n.ParallelAware {
			a.parallel = true
		}
		if n.RowsProcessed >= largeScanThreshold && st == "Seq" {
			a.large = true
		}
	}
	out := make([]TableAccessRow, 0, len(m))
	for k, a := range m {
		out = append(out, TableAccessRow{
			RelationName:  k.rel,
			ScanType:      k.st,
			NodeCount:     a.count,
			TotalRowsRead: a.rows,
			ParallelUsed:  a.parallel,
			LargeScanFlag: a.large,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TotalRowsRead != out[j].TotalRowsRead {
			return out[i].TotalRowsRead > out[j].TotalRowsRead
		}
		return out[i].RelationName < out[j].RelationName
	})
	return out
}

func joinBucket(nodeType string) string {
	t := strings.ToLower(nodeType)
	switch {
	case strings.Contains(t, "hash join"):
		return "Hash"
	case strings.Contains(t, "merge join"):
		return "Merge"
	case strings.Contains(t, "nested loop"):
		return "Nested Loop"
	default:
		return ""
	}
}

func joinEfficiencyLabel(timePct float64) string {
	if timePct < 20 {
		return "good"
	}
	if timePct < 40 {
		return "warning"
	}
	return "bad"
}

func buildJoinAnalysis(nodes []pg_planparser.FlatNode, totalExec float64) []JoinAnalysisRow {
	type agg struct {
		count int
		ms    float64
		rows  int64
	}
	m := map[string]*agg{}
	for _, n := range nodes {
		b := joinBucket(n.NodeType)
		if b == "" {
			continue
		}
		a := m[b]
		if a == nil {
			a = &agg{}
			m[b] = a
		}
		a.count++
		a.ms += n.NodeExecutionTime
		a.rows += n.RowsProcessed
	}
	out := make([]JoinAnalysisRow, 0, len(m))
	for jt, a := range m {
		tp := pg_planmetrics.NodeTimePercent(a.ms, totalExec)
		out = append(out, JoinAnalysisRow{
			JoinType:       jt,
			NodeCount:      a.count,
			TotalTimeMs:    round4(a.ms),
			RowsJoined:     a.rows,
			JoinEfficiency: joinEfficiencyLabel(tp),
		})
	}
	order := map[string]int{"Hash": 0, "Merge": 1, "Nested Loop": 2}
	sort.SliceStable(out, func(i, j int) bool {
		oi, oki := order[out[i].JoinType]
		oj, okj := order[out[j].JoinType]
		if !oki {
			oi = 50
		}
		if !okj {
			oj = 50
		}
		if oi != oj {
			return oi < oj
		}
		return out[i].TotalTimeMs > out[j].TotalTimeMs
	})
	return out
}

func buildFindingGrid(findings []pg_planrules.Finding) []FindingGridRow {
	out := make([]FindingGridRow, 0, len(findings))
	for _, f := range findings {
		out = append(out, FindingGridRow{
			Severity:       string(f.Severity),
			Category:       findingCategory(f.Code),
			FindingCode:    f.Code,
			FindingSummary: firstNonEmpty(f.Title, f.Message),
			Evidence:       evidenceString(f),
		})
	}
	return out
}

func findingCategory(code string) string {
	switch code {
	case "disk_sort", "hash_spill":
		return "memory"
	case "large_seq_scan":
		return "scan"
	case "large_intermediate_result", "excessive_nested_loop":
		return "join"
	case "expensive_windowagg":
		return "sort"
	case "parallelism_not_used":
		return "parallel"
	case "severe_misestimation":
		return "scan"
	default:
		return "memory"
	}
}

func evidenceString(f pg_planrules.Finding) string {
	if len(f.Evidence) > 0 {
		b, err := json.Marshal(f.Evidence)
		if err == nil {
			s := string(b)
			if len(s) > 500 {
				return s[:500] + "…"
			}
			return s
		}
	}
	return strings.TrimSpace(f.Message)
}

func firstNonEmpty(a, b string) string {
	a = strings.TrimSpace(a)
	if a != "" {
		return a
	}
	return strings.TrimSpace(b)
}

type rawIndexOpp struct {
	Pattern        string
	Table            string
	Columns          []string
	FromExpression string
	Recommendation string
	NodeID           int
}

func inferIndexOpportunities(nodes []pg_planparser.FlatNode) []rawIndexOpp {
	var out []rawIndexOpp
	for _, n := range nodes {
		nt := strings.ToLower(n.NodeType)
		if strings.Contains(nt, "seq scan") && strings.TrimSpace(n.Filter) != "" {
			out = append(out, rawIndexOpp{
				Pattern:        "Seq Scan + Filter",
				Table:          n.RelationName,
				Columns:        extractCandidateColumns(n.Filter),
				FromExpression: n.Filter,
				Recommendation: "Missing filter index candidate (validate selectivity and existing indexes).",
				NodeID:         n.NodeID,
			})
		}
		if strings.Contains(nt, "sort") && len(n.SortKey) > 0 {
			out = append(out, rawIndexOpp{
				Pattern:        "Sort + Sort Key",
				Table:          n.RelationName,
				Columns:        append([]string(nil), n.SortKey...),
				Recommendation: "Index supporting ORDER BY may avoid sort (consider composite index on sort keys).",
				NodeID:         n.NodeID,
			})
		}
		if strings.Contains(nt, "aggregate") && len(n.GroupKey) > 0 {
			out = append(out, rawIndexOpp{
				Pattern:        "Aggregate + Group Key",
				Table:          n.RelationName,
				Columns:        append([]string(nil), n.GroupKey...),
				Recommendation: "Composite index on GROUP BY keys may reduce sorting/hashing work depending on plan.",
				NodeID:         n.NodeID,
			})
		}
		if strings.Contains(nt, "join") && strings.TrimSpace(n.HashCond) != "" && strings.Contains(nt, "hash") {
			out = append(out, rawIndexOpp{
				Pattern:        "Join + Hash Cond",
				Table:          n.RelationName,
				Columns:        extractCandidateColumns(n.HashCond),
				FromExpression: n.HashCond,
				Recommendation: "Missing join index candidate on hash condition columns (validate on each join input table).",
				NodeID:         n.NodeID,
			})
		}
		if strings.Contains(nt, "join") && strings.TrimSpace(n.MergeCond) != "" && strings.Contains(nt, "merge") {
			out = append(out, rawIndexOpp{
				Pattern:        "Join + Merge Cond",
				Table:          n.RelationName,
				Columns:        extractCandidateColumns(n.MergeCond),
				FromExpression: n.MergeCond,
				Recommendation: "Merge join benefits from pre-sorted inputs; consider indexes matching merge condition columns.",
				NodeID:         n.NodeID,
			})
		}
	}
	type k struct{ p, t, c string }
	seen := map[k]struct{}{}
	filtered := make([]rawIndexOpp, 0, len(out))
	for _, o := range out {
		ck := strings.Join(o.Columns, ",")
		key := k{p: o.Pattern, t: o.Table, c: ck}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, o)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Pattern != filtered[j].Pattern {
			return filtered[i].Pattern < filtered[j].Pattern
		}
		if filtered[i].Table != filtered[j].Table {
			return filtered[i].Table < filtered[j].Table
		}
		return strings.Join(filtered[i].Columns, ",") < strings.Join(filtered[j].Columns, ",")
	})
	return filtered
}

func opportunityTypeFromPattern(pattern string) string {
	switch {
	case strings.Contains(pattern, "Filter"):
		return "filter"
	case strings.Contains(pattern, "Sort"):
		return "order"
	case strings.Contains(pattern, "Group"):
		return "group"
	case strings.Contains(pattern, "Join"):
		return "join"
	default:
		return "filter"
	}
}

func priorityForIndexOpp(o rawIndexOpp, hasDiskSpill bool) string {
	if hasDiskSpill && strings.Contains(o.Pattern, "Filter") {
		return "high"
	}
	if strings.Contains(o.Pattern, "Join") {
		return "high"
	}
	if strings.Contains(o.Pattern, "Sort") || strings.Contains(o.Pattern, "Group") {
		return "medium"
	}
	return "low"
}

func buildIndexOpportunityGrid(opps []rawIndexOpp, findings []pg_planrules.Finding) []IndexOpportunityGridRow {
	hasDisk := false
	for _, f := range findings {
		if f.Code == "disk_sort" || f.Code == "hash_spill" {
			hasDisk = true
			break
		}
	}
	out := make([]IndexOpportunityGridRow, 0, len(opps))
	for _, o := range opps {
		out = append(out, IndexOpportunityGridRow{
			Priority:        priorityForIndexOpp(o, hasDisk),
			OpportunityType: opportunityTypeFromPattern(o.Pattern),
			TableName:       o.Table,
			ColumnsInvolved: append([]string(nil), o.Columns...),
			Reason:          firstNonEmpty(o.Recommendation, o.Pattern),
		})
	}
	return out
}

func buildTuningGrid(bn pg_plandiagnostics.Bottleneck, findings []pg_planrules.Finding, idx []IndexOpportunityGridRow) []TuningRecommendationGridRow {
	var out []TuningRecommendationGridRow
	rank := 1
	add := func(cat, action, impact string, conf float64) {
		out = append(out, TuningRecommendationGridRow{
			PriorityRank:           rank,
			RecommendationCategory: cat,
			ActionSummary:          action,
			ExpectedImpact:         impact,
			ConfidenceScore:        conf,
		})
		rank++
	}
	spillAdded := false
	for _, f := range findings {
		if spillAdded {
			break
		}
		if f.Code == "disk_sort" || f.Code == "hash_spill" {
			add("memory", firstNonEmpty(f.Suggestion, f.Message), "high", 0.82)
			spillAdded = true
		}
	}
	switch normalizePrimaryBottleneck(bn.Primary) {
	case "scan":
		add("indexing", "Reduce scan volume via selective indexes and tighter predicates.", "high", 0.7)
	case "join":
		add("indexing", "Index join keys and refresh statistics to improve join order and method.", "high", 0.72)
	case "sort":
		add("rewrite", "Reduce sort volume or add indexes supporting ORDER BY / GROUP BY; tune work_mem if appropriate.", "medium", 0.65)
	case "memory":
		add("memory", "Address spills (work_mem, statistics, predicate pushdown) before scaling hardware.", "high", 0.78)
	case "mixed":
		add("rewrite", "No single dominant bottleneck — validate top nodes, row estimates, and buffer metrics.", "low", 0.55)
	}
	for _, f := range findings {
		if f.Code == "severe_misestimation" {
			add("rewrite", "Run ANALYZE and consider extended statistics for severe mis-estimates.", "high", 0.74)
			break
		}
	}
	if len(idx) > 0 && rank <= 8 {
		add("indexing", "Review inferred index opportunities (columns are heuristic candidates, not DDL).", "medium", 0.6)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PriorityRank < out[j].PriorityRank })
	// re-number ranks sequentially
	for i := range out {
		out[i].PriorityRank = i + 1
	}
	return out
}

func extractCandidateColumns(expr string) []string {
	expr = strings.ReplaceAll(expr, "(", " ")
	expr = strings.ReplaceAll(expr, ")", " ")
	expr = strings.ReplaceAll(expr, "=", " ")
	expr = strings.ReplaceAll(expr, ">", " ")
	expr = strings.ReplaceAll(expr, "<", " ")
	expr = strings.ReplaceAll(expr, "!", " ")
	expr = strings.ReplaceAll(expr, "~", " ")
	expr = strings.ReplaceAll(expr, "::", " ")
	parts := strings.Fields(expr)
	seen := map[string]struct{}{}
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.Trim(p, ",;"))
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "'") || strings.HasPrefix(p, "\"") {
			continue
		}
		low := strings.ToLower(p)
		switch low {
		case "and", "or", "not", "null", "true", "false", "like", "ilike", "similar", "to", "any", "all", "in", "is", "distinct", "from":
			continue
		}
		if len(p) < 2 {
			continue
		}
		p = strings.Trim(p, `"`)
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
		if len(out) >= 6 {
			break
		}
	}
	return out
}

func isFinite(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}

func round4(x float64) float64 {
	if !isFinite(x) {
		return 0
	}
	return math.Round(x*1e4) / 1e4
}
