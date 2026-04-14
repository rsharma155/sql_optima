// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Provides hot functionality for the backend.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import "testing"

func TestPgTableMaintSigDiffers(t *testing.T) {
	a := pgFnv64("pg1", 1, "public", "t1", int64(10), int64(9), int64(1), "10.000")
	b := pgFnv64("pg1", 1, "public", "t1", int64(10), int64(8), int64(2), "20.000")
	if a == b {
		t.Fatalf("expected different sig")
	}
}

