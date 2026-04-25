package rules

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/domain/evaluation"
)

func TestMaxMemory_High(t *testing.T) {
	ctx := evaluation.Context{
		"total_memory_gb":      64,
		"max_server_memory_gb": 60,
	}

	res := EvaluateMaxMemory(ctx)

	if res.Severity != "HIGH" {
		t.Errorf("Expected HIGH, got %s", res.Severity)
	}
}

func TestMaxMemory_OK(t *testing.T) {
	ctx := evaluation.Context{
		"total_memory_gb":      64,
		"max_server_memory_gb": 50,
	}

	res := EvaluateMaxMemory(ctx)

	if res.Severity != "OK" {
		t.Errorf("Expected OK, got %s", res.Severity)
	}
}

func TestMaxMemory_RecommendedValue(t *testing.T) {
	ctx := evaluation.Context{
		"total_memory_gb":      64,
		"max_server_memory_gb": 60,
	}

	res := EvaluateMaxMemory(ctx)

	if rec, ok := res.Context["recommended"]; !ok {
		t.Errorf("Expected recommended value in context")
	} else if rec != 51.2 {
		t.Errorf("Expected recommended 51.2, got %f", rec)
	}
}
