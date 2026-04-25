package rules

import "github.com/rsharma155/sql_optima/internal/domain/evaluation"

type RuleHandler func(ctx evaluation.Context) evaluation.Result

type RuleMeta struct {
	RuleID         string
	Threshold      map[string]float64
	ComparisonType string
	Recommended    string
}
