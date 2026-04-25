package rules

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/domain/evaluation"
)

func TestSharedBuffersLow(t *testing.T) {
	ctx := evaluation.Context{
		"shared_buffers_mb": 128,
		"total_memory_mb":   8192,
	}

	res := EvaluateSharedBuffers(ctx)

	if res.Severity != "MEDIUM" {
		t.Errorf("Expected MEDIUM, got %s", res.Severity)
	}
}

func TestSharedBuffers_OK(t *testing.T) {
	ctx := evaluation.Context{
		"shared_buffers_mb": 2048,
		"total_memory_mb":   8192,
	}

	res := EvaluateSharedBuffers(ctx)

	if res.Severity != "OK" {
		t.Errorf("Expected OK, got %s", res.Severity)
	}
}
