package hot

import (
	"encoding/json"
	"testing"
)

func TestNormalizePerformanceDebtDetails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "whitespace", in: "  \t  "},
		{name: "valid object", in: `{"user_updates":42}`},
		{name: "plain text", in: "Proportional fill algorithm detected issues."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := normalizePerformanceDebtDetails(tt.in)
			if !json.Valid([]byte(out)) {
				t.Fatalf("not valid JSON: %q", out)
			}
			if out == "" {
				t.Fatal("expected non-empty JSON")
			}
		})
	}
	if got := normalizePerformanceDebtDetails(""); got != "{}" {
		t.Fatalf("empty => %q, want {}", got)
	}
}
