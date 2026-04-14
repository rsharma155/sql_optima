// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Provides data access layer for pg control center test functionality.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import "testing"

func TestDeadTupleRatioComputationGuard(t *testing.T) {
	// Pure guard test: ensure formula expectations documented.
	dead := 10.0
	live := 90.0
	got := (dead / (dead + live)) * 100.0
	if got < 9.9 || got > 10.1 {
		t.Fatalf("expected ~10%%, got %v", got)
	}
}

