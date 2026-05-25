// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for the extended perf-counters TimescaleDB logger.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"testing"
)

// TestLogSqlServerPerfCountersV2Signature is a compile-time check that the
// extended logger function exists with the expected signature.
func TestLogSqlServerPerfCountersV2Signature(t *testing.T) {
	var _ = (*TimescaleLogger)(nil).LogSqlServerPerfCountersV2
}

// TestGetLatestPerfCounterSignature verifies the read helper compiles.
func TestGetLatestPerfCounterSignature(t *testing.T) {
	var _ = (*TimescaleLogger)(nil).GetLatestPerfCounter
}

// TestPerfCounterWriteRowFields is a compile-time check that PerfCounterWriteRow
// has all fields required by the INSERT statement.
func TestPerfCounterWriteRowFields(t *testing.T) {
	row := PerfCounterWriteRow{
		CounterName:  "Batch Requests/sec",
		InstanceName: "",
		CntrValue:    5000,
		CntrType:     272696576,
		RatePerSec:   100.5,
	}
	if row.CounterName == "" {
		t.Error("CounterName must not be empty")
	}
}

func TestPerfCounterWriteHashSkipsUnchanged(t *testing.T) {
	h1 := perfCounterWriteHash(1000, 50.5)
	h2 := perfCounterWriteHash(1000, 50.5)
	h3 := perfCounterWriteHash(1001, 50.5)
	if h1 != h2 {
		t.Error("identical values should produce identical hash")
	}
	if h1 == h3 {
		t.Error("changed cntr_value should produce different hash")
	}
}
