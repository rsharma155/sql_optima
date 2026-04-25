package rules

import "github.com/rsharma155/sql_optima/internal/domain/evaluation"

func EvaluateAutovacuum(ctx evaluation.Context) evaluation.Result {
	lag := ctx["autovacuum_lag"]

	if lag > 100000 {
		return evaluation.Result{
			RuleID:         "pg_autovacuum",
			Severity:       "HIGH",
			Confidence:     0.9,
			Message:        "Autovacuum lag is high → risk of bloat",
			Recommendation: "Tune autovacuum or run manual VACUUM",
		}
	}

	if lag > 50000 {
		return evaluation.Result{
			RuleID:     "pg_autovacuum",
			Severity:   "MEDIUM",
			Confidence: 0.7,
			Message:    "Autovacuum lag is moderate",
		}
	}

	return evaluation.Result{RuleID: "pg_autovacuum", Severity: "OK", Confidence: 1.0}
}
