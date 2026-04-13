package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/rsharma155/sql_optima/internal/missing_index/logging"
	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

type Handler struct {
	analyzeFunc func(ctx context.Context, req *types.AnalysisRequest) (*types.AnalysisResult, error)
}

func NewHandler(analyzeFunc func(ctx context.Context, req *types.AnalysisRequest) (*types.AnalysisResult, error)) *Handler {
	return &Handler{analyzeFunc: analyzeFunc}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(HealthResponse{
		Status:  "healthy",
		Version: "1.0.0",
	})
}

func (h *Handler) RecommendIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Request body too large")
		return
	}

	var req IndexAdvisorRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body")
		return
	}

	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if req.Options == nil {
		req.Options = DefaultRequestOptions()
	}

	analysisReq := &types.AnalysisRequest{
		DatabaseDSN:       req.DatabaseDSN,
		QueryText:         req.QueryText,
		ExecutionPlanJSON: req.ExecutionPlanJSON,
		QueryParams:       req.QueryParams,
		Options:           toTypesOptions(req.Options),
	}

	logging.Info(r.Context(), "Request received", map[string]any{
		"plan_json_keys": len(req.ExecutionPlanJSON),
		"has_plan":       req.ExecutionPlanJSON["Plan"] != nil,
		"query_len":      len(req.QueryText),
	})

	result, err := h.analyzeFunc(r.Context(), analysisReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ANALYSIS_ERROR", err.Error())
		return
	}

	resp := toResponse(result)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func toTypesOptions(opts *RequestOptions) *types.RequestOptions {
	return &types.RequestOptions{
		MaxCandidates:      opts.MaxCandidates,
		MinImprovementPct:  opts.MinImprovementPct,
		StatementTimeoutMs: opts.StatementTimeoutMs,
		IncludeColumns:     opts.IncludeColumns,
	}
}

func toResponse(result *types.AnalysisResult) *IndexAdvisorResponse {
	resp := &IndexAdvisorResponse{
		RecommendationStatus: result.Status,
		Diagnostics:          result.Diagnostics,
		DebugInfo:            result.DebugInfo,
	}

	if result.TopCandidate != nil {
		resp.TopRecommendation = &Recommendation{
			Table:          result.TopCandidate.Table.Schema + "." + result.TopCandidate.Table.Name,
			IndexMethod:    string(result.TopCandidate.IndexMethod),
			IndexStatement: result.TopCandidate.IndexStatement,
			Confidence:     result.TopCandidate.Confidence,
			Reasoning:      result.TopCandidate.Reasoning,
			Evidence: Evidence{
				OriginalTotalCost:     result.TopCandidate.OriginalCost,
				HypotheticalTotalCost: result.TopCandidate.HypotheticalCost,
				ImprovementPct:        result.TopCandidate.ImprovementPct,
				PlanChangeDetected:    result.TopCandidate.PlanChanged,
				IndexUsedInPlan:       result.TopCandidate.IndexUsed,
			},
		}
	}

	for _, alt := range result.Alternatives {
		resp.Alternatives = append(resp.Alternatives, Recommendation{
			Table:          alt.Table.Schema + "." + alt.Table.Name,
			IndexMethod:    string(alt.IndexMethod),
			IndexStatement: alt.IndexStatement,
			Confidence:     alt.Confidence,
			Reasoning:      alt.Reasoning,
			Evidence: Evidence{
				OriginalTotalCost:     alt.OriginalCost,
				HypotheticalTotalCost: alt.HypotheticalCost,
				ImprovementPct:        alt.ImprovementPct,
				PlanChangeDetected:    alt.PlanChanged,
				IndexUsedInPlan:       alt.IndexUsed,
			},
		})
	}

	for _, rej := range result.Rejections {
		resp.Rejections = append(resp.Rejections, RejectedCandidate{
			Candidate:       rej.Candidate,
			RejectionReason: rej.Reason,
		})
	}

	for _, qr := range result.QueryRewrites {
		resp.QueryRewrites = append(resp.QueryRewrites, QueryRewrite{
			OriginalQuery:  qr.OriginalQuery,
			RewrittenQuery: qr.RewrittenQuery,
			AppliedRules:   qr.AppliedRules,
		})
	}

	return resp
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Code:    code,
		Message: message,
		Error:   code,
	})
}
