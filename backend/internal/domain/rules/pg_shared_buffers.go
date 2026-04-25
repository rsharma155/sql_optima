package rules

import "github.com/rsharma155/sql_optima/internal/domain/evaluation"

func EvaluateSharedBuffers(ctx evaluation.Context) evaluation.Result {
	shared := ctx["shared_buffers_mb"]
	total := ctx["total_memory_mb"]

	recommended := total * 0.25

	if shared < recommended {
		return evaluation.Result{
			RuleID:         "pg_shared_buffers",
			Severity:       "MEDIUM",
			Confidence:     0.8,
			Message:        "shared_buffers too low",
			Recommendation: "Increase shared_buffers to 25% of RAM",
			Context: map[string]float64{
				"recommended": recommended,
			},
		}
	}

	return evaluation.Result{RuleID: "pg_shared_buffers", Severity: "OK", Confidence: 1.0}
}
