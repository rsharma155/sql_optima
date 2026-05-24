// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for the extended perf-counters TimescaleDB logger.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestLogSqlServerPerfCountersV2Signature is a compile-time check that the
// extended logger function exists with the expected signature.
func TestLogSqlServerPerfCountersV2Signature(t *testing.T) {
	var _ func(context.Context, uuid.UUID, time.Time, []PerfCounterWriteRow) error =
		(*TimescaleLogger)(nil).LogSqlServerPerfCountersV2
}

// TestGetLatestPerfCounterSignature verifies the read helper compiles.
func TestGetLatestPerfCounterSignature(t *testing.T) {
	var _ func(context.Context, uuid.UUID, string, string) (float64, bool, error) =
		(*TimescaleLogger)(nil).GetLatestPerfCounter
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
