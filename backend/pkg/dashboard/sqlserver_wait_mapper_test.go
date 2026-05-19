package dashboard

import (
	"testing"
)

func TestCategorizeWaitTypeV2(t *testing.T) {
	tests := []struct {
		waitType string
		expected WaitCategoryV2
	}{
		{"LCK_M_X", WaitV2Locking},
		{"LCK_M_ARBITRARY", WaitV2Locking}, // prefix match
		{"PAGEIOLATCH_SH", WaitV2IOData},
		{"PAGEIOLATCH_UNKNOWN", WaitV2IOData}, // prefix match
		{"PAGELATCH_EX", WaitV2IOData},       // prefix match
		{"WRITELOG", WaitV2IOLog},
		{"SOS_SCHEDULER_YIELD", WaitV2CPU},
		{"RESOURCE_SEMAPHORE", WaitV2Memory},
		{"CXPACKET", WaitV2Parallelism},
		{"ASYNC_NETWORK_IO", WaitV2Network},
		{"UNKNOWN_WAIT_XYZ", WaitV2Other}, // fallthrough
		{"", WaitV2Other},                // empty string safe
	}

	for _, tc := range tests {
		t.Run(tc.waitType, func(t *testing.T) {
			got := CategorizeWaitTypeV2(tc.waitType)
			if got != tc.expected {
				t.Errorf("CategorizeWaitTypeV2(%s) = %s; want %s", tc.waitType, got, tc.expected)
			}
		})
	}
}

func TestComputeWaitDeltasV2(t *testing.T) {
	t.Run("NormalCase", func(t *testing.T) {
		prev := map[string][3]int64{
			"LCK_M_X": {100, 10, 5},
		}
		curr := map[string][3]int64{
			"LCK_M_X": {150, 15, 8},
		}
		got := ComputeWaitDeltasV2(prev, curr)
		if len(got) != 1 {
			t.Fatalf("expected 1 delta row, got %d", len(got))
		}
		d := got[0]
		if d.WaitType != "LCK_M_X" || d.DeltaWaitMs != 50 || d.DeltaSignalWaitMs != 5 || d.DeltaWaitingTasks != 3 || d.RestartDetected {
			t.Errorf("unexpected delta: %+v", d)
		}
	})

	t.Run("RestartDetected", func(t *testing.T) {
		prev := map[string][3]int64{
			"LCK_M_X": {100, 10, 5},
		}
		curr := map[string][3]int64{
			"LCK_M_X": {50, 5, 2},
		}
		got := ComputeWaitDeltasV2(prev, curr)
		if len(got) != 1 {
			t.Fatalf("expected 1 delta row, got %d", len(got))
		}
		d := got[0]
		if !d.RestartDetected || d.DeltaWaitMs != 0 || d.DeltaSignalWaitMs != 0 {
			t.Errorf("expected restart detected and 0 deltas, got: %+v", d)
		}
	})

	t.Run("FirstRun", func(t *testing.T) {
		var prev map[string][3]int64
		curr := map[string][3]int64{
			"LCK_M_X": {150, 15, 8},
		}
		got := ComputeWaitDeltasV2(prev, curr)
		// Based on flow doc, if prev is nil, it should return deltas with DeltaWaitMs=current (or 0 if we prefer)
		// The flow doc Step 1.5 says: "First cycle — no deltas yet, skip writes"
		// So ComputeWaitDeltasV2 could either return current values as deltas or we skip in service.
		// Let's see what ComputeWaitStatsDeltasV2 in redesign doc says:
		// "prev and curr are maps... pv := prev[wt] ... dWait := cv[0] - pv[0]"
		// If prev is nil, pv[0] is 0. So it returns full current values as deltas.
		// But usually we don't want to report full cumulative as a single interval delta.
		// The service in Step 1.5 checks `if prev == nil { continue }`.
		// So let's just ensure it doesn't panic.
		if len(got) != 1 {
			t.Errorf("expected 1 delta row even on first run, got %d", len(got))
		}
	})
}
