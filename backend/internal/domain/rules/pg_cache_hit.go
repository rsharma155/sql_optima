package rules

import "github.com/rsharma155/sql_optima/internal/domain/evaluation"

func EvaluateCacheHit(ctx evaluation.Context) evaluation.Result {
	ratio := ctx["cache_hit_ratio"]

	if ratio < 0.90 {
		return evaluation.Result{
			RuleID:        "pg_cache_hit",
			Severity:      "HIGH",
			Confidence:    0.9,
			Message:       "Low cache hit ratio → disk I/O pressure",
			Recommendation: "Increase shared_buffers or optimize queries",
		}
	}

	if ratio < 0.95 {
		return evaluation.Result{
			RuleID:      "pg_cache_hit",
			Severity:    "MEDIUM",
			Confidence:  0.7,
			Message:     "Cache hit ratio could be improved",
		}
	}

	return evaluation.Result{RuleID: "pg_cache_hit", Severity: "OK", Confidence: 1.0}
}