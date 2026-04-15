package pg_locks_blocking

import "testing"

func TestAnalyzePairs_Empty(t *testing.T) {
	st := AnalyzePairs(nil)
	if st.VictimsDistinct != 0 || st.ChainDepth != 0 || len(st.RootBlockers) != 0 {
		t.Fatalf("unexpected stats: %+v", st)
	}
}

func TestAnalyzePairs_SimpleChain(t *testing.T) {
	// 1 blocks 2 blocks 3
	pairs := []Pair{
		{BlockedPID: 2, BlockingPID: 1},
		{BlockedPID: 3, BlockingPID: 2},
	}
	st := AnalyzePairs(pairs)
	if st.VictimsDistinct != 2 {
		t.Fatalf("victims=%d", st.VictimsDistinct)
	}
	if st.ChainDepth != 3 {
		t.Fatalf("depth=%d", st.ChainDepth)
	}
	if len(st.RootBlockers) != 1 || st.RootBlockers[0] != 1 {
		t.Fatalf("roots=%v", st.RootBlockers)
	}
}

func TestAnalyzePairs_MultipleRoots(t *testing.T) {
	// 10 blocks 11; 20 blocks 21,22
	pairs := []Pair{
		{BlockedPID: 11, BlockingPID: 10},
		{BlockedPID: 21, BlockingPID: 20},
		{BlockedPID: 22, BlockingPID: 20},
	}
	st := AnalyzePairs(pairs)
	if st.VictimsDistinct != 3 {
		t.Fatalf("victims=%d", st.VictimsDistinct)
	}
	if st.ChainDepth != 2 {
		t.Fatalf("depth=%d", st.ChainDepth)
	}
	if len(st.RootBlockers) != 2 || st.RootBlockers[0] != 10 || st.RootBlockers[1] != 20 {
		t.Fatalf("roots=%v", st.RootBlockers)
	}
}

func TestAnalyzePairs_CycleDoesNotLoop(t *testing.T) {
	// 1 blocks 2, 2 blocks 1
	pairs := []Pair{
		{BlockedPID: 2, BlockingPID: 1},
		{BlockedPID: 1, BlockingPID: 2},
	}
	st := AnalyzePairs(pairs)
	if st.VictimsDistinct != 2 {
		t.Fatalf("victims=%d", st.VictimsDistinct)
	}
	// With a cycle, we still want a bounded depth.
	if st.ChainDepth <= 0 || st.ChainDepth > 3 {
		t.Fatalf("depth=%d", st.ChainDepth)
	}
	// Roots may be empty in cycles (everyone is blocked).
}

