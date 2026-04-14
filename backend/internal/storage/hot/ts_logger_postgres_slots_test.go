// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Provides hot functionality for the backend.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import "testing"

func TestLogPostgresReplicationSlots_DedupSignatureStable(t *testing.T) {
	// Pure unit test: validate our signature changes with content.
	// We don't need a real pool for this; we just exercise pgFnv64 usage expectations.
	a := pgFnv64("pg1", 2, "s1", "physical", true, false, "1.000", "0/1", "", "", "")
	b := pgFnv64("pg1", 2, "s1", "physical", true, false, "2.000", "0/1", "", "", "")
	if a == b {
		t.Fatalf("expected signature to differ when retained wal differs")
	}
}

