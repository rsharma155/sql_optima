// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Test suite for delta computation logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package storageindex

import "testing"

func TestDelta_Positive(t *testing.T) {
	d, ok := Delta(10, 7)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if d != 3 {
		t.Fatalf("expected 3 got %d", d)
	}
}

func TestDelta_Zero(t *testing.T) {
	d, ok := Delta(7, 7)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if d != 0 {
		t.Fatalf("expected 0 got %d", d)
	}
}

func TestDelta_NegativeCounterReset(t *testing.T) {
	d, ok := Delta(1, 10)
	if ok {
		t.Fatalf("expected ok=false")
	}
	if d != 0 {
		t.Fatalf("expected 0 got %d", d)
	}
}

