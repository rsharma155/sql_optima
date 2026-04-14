// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Provides hot functionality for the backend.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import "testing"

func TestDiskSigChangesWhenFreeBytesChanges(t *testing.T) {
	a := pgFnv64("pg1", "data", "/x", int64(100), int64(50), int64(40), "50.0")
	b := pgFnv64("pg1", "data", "/x", int64(100), int64(49), int64(40), "51.0")
	if a == b {
		t.Fatalf("expected different signatures")
	}
}

