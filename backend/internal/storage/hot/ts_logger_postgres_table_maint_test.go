package hot

import "testing"

func TestPgTableMaintSigDiffers(t *testing.T) {
	a := pgFnv64("pg1", 1, "public", "t1", int64(10), int64(9), int64(1), "10.000")
	b := pgFnv64("pg1", 1, "public", "t1", int64(10), int64(8), int64(2), "20.000")
	if a == b {
		t.Fatalf("expected different sig")
	}
}

