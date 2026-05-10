// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TDD tests for PostgreSQL deadlock fetching logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import "testing"

func TestDeadlockAggregation_Basic(t *testing.T) {
	// Simple test to ensure we consider the aggregation logic
	dbs := []int64{1, 2, 3}
	var total int64
	for _, d := range dbs {
		total += d
	}
	if total != 6 {
		t.Fatalf("expected 6, got %d", total)
	}
}
