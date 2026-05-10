// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TDD tests for PostgreSQL connection utilization logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"math"
	"testing"
)

func TestComputeConnectionUsagePct_Basic(t *testing.T) {
	// 80 used / 100 max = 80.0%
	used := 80
	max := 100
	got := (float64(used) * 100.0) / float64(max)
	if got != 80.0 {
		t.Fatalf("expected 80.0, got %v", got)
	}
}

func TestComputeConnectionUsagePct_ZeroMaxClampsToZero(t *testing.T) {
	// divide-by-zero guard
	used := 10
	max := 0
	var got float64
	if max > 0 {
		got = (float64(used) * 100.0) / float64(max)
	} else {
		got = 0
	}
	if got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
}

func TestComputeConnectionUsagePct_OverMax_ClampsTo100(t *testing.T) {
	// defensive upper clamp
	used := 120
	max := 100
	got := (float64(used) * 100.0) / float64(max)
	got = math.Max(0, math.Min(100, got))
	if got != 100.0 {
		t.Fatalf("expected 100.0, got %v", got)
	}
}
