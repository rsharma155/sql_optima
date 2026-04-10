package repository

import "testing"

func TestDeadPct(t *testing.T) {
	if got := deadPct(0, 0); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
	if got := deadPct(100, 0); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
	if got := deadPct(90, 10); got < 9.9 || got > 10.1 {
		t.Fatalf("expected ~10, got %v", got)
	}
}

