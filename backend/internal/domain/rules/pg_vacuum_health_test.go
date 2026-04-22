package rules

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/domain/evaluation"
)

func TestVacuumHighDeadTuples(t *testing.T) {
	ctx := evaluation.Context{
		"dead_tuple_pct":   25,
		"last_vacuum_hours": 48,
	}

	res := EvaluateVacuumHealth(ctx)

	if res.Severity != "HIGH" {
		t.Errorf("Expected HIGH, got %s", res.Severity)
	}
}

func TestVacuumMedium(t *testing.T) {
	ctx := evaluation.Context{
		"dead_tuple_pct":   15,
		"last_vacuum_hours": 12,
	}

	res := EvaluateVacuumHealth(ctx)

	if res.Severity != "MEDIUM" {
		t.Errorf("Expected MEDIUM, got %s", res.Severity)
	}
}

func TestVacuum_OK(t *testing.T) {
	ctx := evaluation.Context{
		"dead_tuple_pct":   5,
		"last_vacuum_hours": 2,
	}

	res := EvaluateVacuumHealth(ctx)

	if res.Severity != "OK" {
		t.Errorf("Expected OK, got %s", res.Severity)
	}
}