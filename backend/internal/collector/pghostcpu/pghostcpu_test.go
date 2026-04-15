// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for PostgreSQL host CPU parsing helpers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package pghostcpu

import (
	"math"
	"testing"
)

func TestParseLoadAvg(t *testing.T) {
	a, b, c := parseLoadAvg("0.12 0.34 0.56 2/900 12345")
	if math.Abs(a-0.12) > 1e-9 || math.Abs(b-0.34) > 1e-9 || math.Abs(c-0.56) > 1e-9 {
		t.Fatalf("unexpected load triple: %v %v %v", a, b, c)
	}
	a, b, c = parseLoadAvg("")
	if a != 0 || b != 0 || c != 0 {
		t.Fatalf("expected zeros for empty input")
	}
}

func TestCpuSaturationPct(t *testing.T) {
	got := CpuSaturationPct(2.0, 4)
	if math.Abs(got-50.0) > 1e-9 {
		t.Fatalf("expected 50, got %v", got)
	}
	if CpuSaturationPct(1, 0) != 0 {
		t.Fatalf("expected 0 when no cores")
	}
}

func TestCpuPerConnection(t *testing.T) {
	got := CpuPerConnection(10, 5)
	if math.Abs(got-2.0) > 1e-9 {
		t.Fatalf("expected 2, got %v", got)
	}
	if CpuPerConnection(10, 0) != 0 {
		t.Fatalf("expected 0 with no connections")
	}
}
