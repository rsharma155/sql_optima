package repository

import "testing"

func TestPgRetainedWalMB_Computation(t *testing.T) {
	// 1 MiB in bytes
	const mib = 1024.0 * 1024.0

	got := retainedWalMBFromBytes(0)
	if got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}

	got = retainedWalMBFromBytes(10)
	if got <= 0 || got >= 0.01 {
		t.Fatalf("expected small positive MB, got %v", got)
	}

	// 512 MiB
	got = retainedWalMBFromBytes(int64(512 * mib))
	if got < 511.9 || got > 512.1 {
		t.Fatalf("expected ~512 MB, got %v", got)
	}
}

