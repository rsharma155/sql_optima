// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Implements business logic for pg_disk_statfs_test operations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import "testing"

func TestComputeUsedPct_Basic(t *testing.T) {
	if got := computeUsedPct(0, 0); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
	if got := computeUsedPct(100, 100); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
	if got := computeUsedPct(100, 0); got < 99.9 {
		t.Fatalf("expected ~100, got %v", got)
	}
	if got := computeUsedPct(100, 60); got < 39.9 || got > 40.1 {
		t.Fatalf("expected ~40, got %v", got)
	}
}

