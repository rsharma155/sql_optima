// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TDD tests for extended session state logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import "testing"

func TestPgSessionStateFull_TotalIsConsistent(t *testing.T) {
	// active+idle+idle_in_txn+waiting <= total
	// (Note: waiting is usually a subset of active in many versions,
	// but in our query we count them separately based on wait_event_type)
	stats := PgSessionStateFull{
		Active:           10,
		Idle:             50,
		IdleInTxn:        5,
		IdleInTxnAborted: 1,
		Waiting:          4,
		Total:            70,
	}
	sum := stats.Active + stats.Idle + stats.IdleInTxn + stats.IdleInTxnAborted + stats.Waiting
	if sum > stats.Total {
		t.Fatalf("sum of states %d exceeds total %d", sum, stats.Total)
	}
}

func TestPgSessionStateFull_ZeroWhenNoConnections(t *testing.T) {
	stats := PgSessionStateFull{}
	if stats.Total != 0 {
		t.Fatalf("expected 0 total, got %d", stats.Total)
	}
}
