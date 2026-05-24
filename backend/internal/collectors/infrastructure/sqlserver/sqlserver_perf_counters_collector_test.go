// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for the unified sys.dm_os_performance_counters collector.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package sqlserver

import (
	"testing"
)

// TestPerfCounterRowFields is a compile-time check that PerfCounterRow has
// all required fields used by the logger and service layer.
func TestPerfCounterRowFields(t *testing.T) {
	row := PerfCounterRow{
		CounterName:  "Batch Requests/sec",
		InstanceName: "",
		CntrValue:    12345,
		CntrType:     272696576,
		ObjectName:   "SQLServer:SQL Statistics",
	}
	if row.CounterName == "" {
		t.Error("CounterName must not be empty")
	}
	if row.CntrType == 0 {
		t.Error("CntrType must be set")
	}
}

// TestPerfCountersCollectorIsStateless verifies that PerfCountersCollector
// can be zero-value constructed (stateless — state is held by the service).
func TestPerfCountersCollectorIsStateless(t *testing.T) {
	c := &PerfCountersCollector{}
	if c == nil {
		t.Error("PerfCountersCollector should be constructible as zero value")
	}
}

// TestAllRequiredCountersPresent checks that all 15 target counter names are
// included in the collector's query constant. This guards against accidental
// removal during refactors.
func TestAllRequiredCountersPresent(t *testing.T) {
	required := []string{
		"Page life expectancy",
		"Buffer Pool Size (KB)",
		"Total Server Memory (KB)",
		"Target Server Memory (KB)",
		"Memory Grants Pending",
		"Batch Requests/sec",
		"Page Reads/sec",
		"SQL Compilations/sec",
		"Logins/sec",
		"User Connections",
		"Transactions/sec",
		"Buffer cache hit ratio",
		"Buffer cache hit ratio base",
		"Sort Warnings/sec",
		"Hash Warnings/sec",
	}
	for _, name := range required {
		found := false
		for _, target := range perfCounterNames {
			if target == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing required counter in perfCounterNames: %q", name)
		}
	}
}

// TestComputeRatePerSec verifies rate calculation for type-272696576 counters
// (cumulative counters that need delta / interval).
func TestComputeRatePerSec(t *testing.T) {
	tests := []struct {
		curr     int64
		prev     int64
		secs     float64
		expected float64
	}{
		{curr: 2000, prev: 1000, secs: 10, expected: 100},
		{curr: 500, prev: 500, secs: 30, expected: 0},    // no change
		{curr: 500, prev: 1000, secs: 10, expected: 0},   // counter reset → non-negative
		{curr: 1000, prev: 0, secs: 5, expected: 200},
	}

	for _, tc := range tests {
		got := computeRatePerSec(tc.curr, tc.prev, tc.secs)
		if got != tc.expected {
			t.Errorf("computeRatePerSec(%d, %d, %v) = %v; want %v",
				tc.curr, tc.prev, tc.secs, got, tc.expected)
		}
	}
}
