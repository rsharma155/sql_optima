package rules

import "github.com/rsharma155/sql_optima/internal/domain/evaluation"

func EvaluateMaxMemory(ctx evaluation.Context) evaluation.Result {
	total := ctx["total_memory_gb"]
	maxMem := ctx["max_server_memory_gb"]

	osReserve := total * 0.2
	recommended := total - osReserve

	if maxMem > recommended {
		return evaluation.Result{
			RuleID:        "max_server_memory",
			Severity:      "HIGH",
			Confidence:   0.9,
			Message:       "Max server memory exceeds safe limit",
			Recommendation: "Reduce max_server_memory",
			Context: map[string]float64{
				"recommended": recommended,
			},
		}
	}

	return evaluation.Result{RuleID: "max_server_memory", Severity: "OK", Confidence: 1.0}
}