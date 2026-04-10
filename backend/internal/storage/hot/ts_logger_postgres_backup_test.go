package hot

import "testing"

func TestNormalizeBackupStatus(t *testing.T) {
	if normalizeBackupStatus("SUCCESS") != "success" {
		t.Fatalf("expected success")
	}
	if normalizeBackupStatus(" failed ") != "failed" {
		t.Fatalf("expected failed")
	}
	if normalizeBackupStatus("warn") != "warning" {
		t.Fatalf("expected warning")
	}
	if normalizeBackupStatus("") != "unknown" {
		t.Fatalf("expected unknown")
	}
}

