package rules

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/domain/evaluation"
)

func TestMaxDOP_HighWaits(t *testing.T) {
	ctx := evaluation.Context{
		"cpu_count":         32,
		"maxdop":           16,
		"parallel_wait_pct": 40,
	}

	res := EvaluateMaxDOP(ctx)

	if res.Severity != "HIGH" {
		t.Errorf("Expected HIGH, got %s", res.Severity)
	}

	if res.Confidence <= 0.8 {
		t.Errorf("Expected confidence > 0.8, got %f", res.Confidence)
	}
}

func TestMaxDOP_Medium(t *testing.T) {
	ctx := evaluation.Context{
		"cpu_count":         16,
		"maxdop":           10,
		"parallel_wait_pct": 10,
	}

	res := EvaluateMaxDOP(ctx)

	if res.Severity != "MEDIUM" {
		t.Errorf("Expected MEDIUM, got %s", res.Severity)
	}
}

func TestMaxDOP_OK(t *testing.T) {
	ctx := evaluation.Context{
		"cpu_count":         8,
		"maxdop":           4,
		"parallel_wait_pct": 5,
	}

	res := EvaluateMaxDOP(ctx)

	if res.Severity != "OK" {
		t.Errorf("Expected OK, got %s", res.Severity)
	}
}