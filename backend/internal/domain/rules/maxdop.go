package rules

import (
	"math"

	"github.com/rsharma155/sql_optima/internal/domain/evaluation"
)

func EvaluateMaxDOP(ctx evaluation.Context) evaluation.Result {
	cpu := ctx["cpu_count"]
	maxdop := ctx["maxdop"]
	waits := ctx["parallel_wait_pct"]

	recommended := math.Min(8, cpu/2)

	if maxdop > recommended {
		if waits > 20 {
			return evaluation.Result{
				RuleID:      "maxdop",
				Severity:    "HIGH",
				Confidence: 0.92,
				Message:     "High parallel waits due to high MAXDOP",
			}
		}
		return evaluation.Result{
			RuleID:      "maxdop",
			Severity:    "MEDIUM",
			Confidence: 0.75,
		}
	}

	return evaluation.Result{RuleID: "maxdop", Severity: "OK", Confidence: 1.0}
}