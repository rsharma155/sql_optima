package hot

import "testing"

func TestNormalizePgLogSeverity(t *testing.T) {
	if normalizePgLogSeverity("FATAL") != "fatal" {
		t.Fatalf("expected fatal")
	}
	if normalizePgLogSeverity(" panic ") != "panic" {
		t.Fatalf("expected panic")
	}
	if normalizePgLogSeverity("ERR") != "error" {
		t.Fatalf("expected error")
	}
	if normalizePgLogSeverity("") != "unknown" {
		t.Fatalf("expected unknown")
	}
}

