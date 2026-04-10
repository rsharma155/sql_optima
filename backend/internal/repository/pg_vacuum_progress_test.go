package repository

import "testing"

func TestVacuumProgressPctGuards(t *testing.T) {
	if got := vacuumProgressPct(0, 0); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
	if got := vacuumProgressPct(100, 0); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
	if got := vacuumProgressPct(100, 50); got < 49.9 || got > 50.1 {
		t.Fatalf("expected ~50, got %v", got)
	}
	if got := vacuumProgressPct(100, 150); got != 100 {
		t.Fatalf("expected clamp to 100, got %v", got)
	}
}

