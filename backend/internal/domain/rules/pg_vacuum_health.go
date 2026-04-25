package rules

import "github.com/rsharma155/sql_optima/internal/domain/evaluation"

func EvaluateVacuumHealth(ctx evaluation.Context) evaluation.Result {
	dead := ctx["dead_tuple_pct"]
	lastVacuumHours := ctx["last_vacuum_hours"]

	if dead > 20 && lastVacuumHours > 24 {
		return evaluation.Result{
			RuleID:         "pg_vacuum_health",
			Severity:       "HIGH",
			Confidence:     0.9,
			Message:        "High dead tuples with stale vacuum → bloat risk",
			Recommendation: "Tune autovacuum or run manual VACUUM",
		}
	}

	if dead > 10 {
		return evaluation.Result{
			RuleID:     "pg_vacuum_health",
			Severity:   "MEDIUM",
			Confidence: 0.7,
			Message:    "Moderate dead tuples detected",
		}
	}

	return evaluation.Result{
		RuleID:     "pg_vacuum_health",
		Severity:   "OK",
		Confidence: 1.0,
	}
}
