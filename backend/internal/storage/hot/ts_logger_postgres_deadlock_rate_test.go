// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TDD tests for PostgreSQL deadlock rate computation.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"testing"
)

func TestDeadlockRateComputation_FirstObservation_ReturnsNotOk(t *testing.T) {
	tl := &TimescaleLogger{
		prevPgDeadlocksTotalAllDBs: make(map[string]int64),
	}
	
	rate, ok := tl.ComputePgDeadlockRate("inst1", 10, 60.0)
	if ok {
		t.Fatalf("expected ok=false on first observation")
	}
	if rate != 0 {
		t.Fatalf("expected rate=0 on first observation")
	}
}

func TestDeadlockRateComputation_SixDeadlocksInSixtySeconds(t *testing.T) {
	tl := &TimescaleLogger{
		prevPgDeadlocksTotalAllDBs: make(map[string]int64),
	}
	
	// First observation
	tl.ComputePgDeadlockRate("inst1", 10, 60.0)
	
	// Second observation: 6 more deadlocks in 60s
	rate, ok := tl.ComputePgDeadlockRate("inst1", 16, 60.0)
	if !ok {
		t.Fatalf("expected ok=true on second observation")
	}
	// (16 - 10) * 60 / 60 = 6 deadlocks/min
	if rate != 6.0 {
		t.Fatalf("expected 6.0 deadlocks/min, got %v", rate)
	}
}

func TestDeadlockRateComputation_CounterReset_ClampsToZero(t *testing.T) {
	tl := &TimescaleLogger{
		prevPgDeadlocksTotalAllDBs: make(map[string]int64),
	}
	
	// First observation: 100 deadlocks
	tl.ComputePgDeadlockRate("inst1", 100, 60.0)
	
	// Second observation: 10 deadlocks (restart happened)
	rate, ok := tl.ComputePgDeadlockRate("inst1", 10, 60.0)
	if !ok {
		t.Fatalf("expected ok=true on second observation even after reset")
	}
	if rate != 0 {
		t.Fatalf("expected rate=0 after counter reset, got %v", rate)
	}
}
