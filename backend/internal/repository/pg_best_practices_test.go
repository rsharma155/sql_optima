package repository

import "testing"

func TestFormatPgSizeHuman_ShowsMBAndRaw(t *testing.T) {
	r := &PgRepository{}
	got := r.formatPgSizeHuman("128", "MB")
	if got == "" || got == "128 MB" {
		t.Fatalf("expected human format with raw, got %q", got)
	}
	if want := "MB (128 MB)"; !contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}

	got2 := r.formatPgSizeHuman("16384", "8kB")
	if !contains(got2, "MB") || !contains(got2, "(16384 8kB)") {
		t.Fatalf("expected MB + raw 8kB, got %q", got2)
	}
}

func TestParsePgSize_KnowsKBUnits(t *testing.T) {
	r := &PgRepository{}
	if got := r.parsePgSize("1024", "kB"); got != 1024*1024 {
		t.Fatalf("expected 1024 kB = 1MB, got %d", got)
	}
	if got := r.parsePgSizeKB("1", "MB"); got != 1024 {
		t.Fatalf("expected 1 MB = 1024 kB, got %d", got)
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || (len(s) >= len(sub) && (func() bool { return indexOf(s, sub) >= 0 })()) }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

