package hot

import "testing"

func TestComputeWalRateMBPerMin_FirstObservationReturnsNotOk(t *testing.T) {
	tl := NewTimescaleLogger(nil)
	if rate, ok := tl.ComputeWalRateMBPerMin("pg1", 1000, 60); ok || rate != 0 {
		t.Fatalf("expected ok=false and rate=0, got ok=%v rate=%v", ok, rate)
	}
}

func TestComputeWalRateMBPerMin_ComputesRateAndClampsNegative(t *testing.T) {
	tl := NewTimescaleLogger(nil)
	_, _ = tl.ComputeWalRateMBPerMin("pg1", 100*1024*1024, 60) // baseline

	rate, ok := tl.ComputeWalRateMBPerMin("pg1", 160*1024*1024, 60)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if rate < 59.9 || rate > 60.1 {
		t.Fatalf("expected ~60 MB/min, got %v", rate)
	}

	rate2, ok2 := tl.ComputeWalRateMBPerMin("pg1", 10, 60) // counter reset -> clamp
	if !ok2 {
		t.Fatalf("expected ok=true on subsequent calls")
	}
	if rate2 != 0 {
		t.Fatalf("expected clamp to 0, got %v", rate2)
	}
}

