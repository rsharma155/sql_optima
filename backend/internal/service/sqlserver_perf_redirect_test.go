// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Tests that perf counter reads are redirected to TimescaleDB cache.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// TestReadPerfCounterRatesSignatureAcceptsLogger verifies at compile time that
// readPerfCounterRates accepts a *hot.TimescaleLogger so callers can supply the
// TimescaleDB cache instead of hitting the monitored server's DMV.
func TestReadPerfCounterRatesSignatureAcceptsLogger(t *testing.T) {
	// Compile-time check: if signature changes, this fails to compile.
	type funcType func(context.Context, uuid.UUID, *hot.TimescaleLogger) (float64, float64, float64, float64, error)
	_ = funcType(readPerfCounterRatesFromCache)
}

// TestReadPerfCounterRatesNilLogger verifies that when tsLogger is nil the
// function returns zero rates and a nil error (graceful no-op; caller uses DMV).
func TestReadPerfCounterRatesNilLogger(t *testing.T) {
	pr, br, sc, ls, err := readPerfCounterRatesFromCache(context.Background(), uuid.New(), nil)
	if err != nil {
		t.Errorf("expected nil error with nil logger, got %v", err)
	}
	if pr != 0 || br != 0 || sc != 0 || ls != 0 {
		t.Errorf("expected zero rates with nil logger, got pr=%v br=%v sc=%v ls=%v", pr, br, sc, ls)
	}
}
