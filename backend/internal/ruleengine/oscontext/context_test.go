package oscontext

import "testing"

func TestIsOSAwareRule(t *testing.T) {
	if !IsOSAwareRule("PG_SHARED_BUFFERS_001") {
		t.Fatal("expected PG_SHARED_BUFFERS_001 to be OS-aware")
	}
	if IsOSAwareRule("PG_LOG_SLOW_QUERY_023") {
		t.Fatal("logging rule should not be OS-aware")
	}
}

func TestRuleEval_MergeInto_unavailable(t *testing.T) {
	env := map[string]interface{}{"setting": float64(100)}
	RuleEval{}.MergeInto(env)
	if env["os_available"] != float64(0) {
		t.Fatalf("os_available = %v, want 0", env["os_available"])
	}
}

func TestRuleEval_MergeInto_available(t *testing.T) {
	env := map[string]interface{}{}
	RuleEval{
		Available:       true,
		TotalRAMBytes:   16 * 1024 * 1024 * 1024,
		TotalRAMGB:      16,
		TotalRAMPages8k: (16 * 1024 * 1024 * 1024) / 8192,
		AvailableBytes:  8 * 1024 * 1024 * 1024,
		MemoryUsedPct:   50,
		CPUCores:        8,
	}.MergeInto(env)
	if env["os_available"] != float64(1) {
		t.Fatalf("os_available = %v", env["os_available"])
	}
	if env["TotalRAM_pages_8k"].(float64) <= 0 {
		t.Fatal("expected positive TotalRAM_pages_8k")
	}
}
