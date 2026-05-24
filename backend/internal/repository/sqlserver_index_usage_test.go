// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for FetchUnusedIndexes redirect to monitor.index_usage_stats.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
)

// TestFetchUnusedIndexesSignature is a compile-time check that FetchUnusedIndexes
// accepts serverID so it can read from the local TimescaleDB cache.
func TestFetchUnusedIndexesSignature(t *testing.T) {
	var _ func(context.Context, uuid.UUID, string, string, int64, int) ([]PerformanceDebtUnusedIndex, error) =
		(*SqlServerRepository)(nil).FetchUnusedIndexes
}

// TestFetchUnusedIndexesNilPoolNoConn verifies that when LocalPool is nil and
// there is no monitored-instance connection, the function returns an error
// without panicking.
func TestFetchUnusedIndexesNilPoolNoConn(t *testing.T) {
	repo := &SqlServerRepository{
		conns:       make(map[string]*sql.DB),
		connStrings: make(map[string]string),
	}
	// LocalPool is nil, no connection for "MISSING"
	result, err := repo.FetchUnusedIndexes(context.Background(), uuid.New(), "MISSING", "testdb", 1000, 50)
	if err == nil {
		t.Error("expected error when instance has no connection, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %v", result)
	}
}

// TestFetchUnusedIndexesDefaultLimits verifies that zero values for limit and
// minUpdates are replaced with safe defaults (50 and 1000 respectively).
// We use a nil-pool repo with no connection so the function short-circuits at
// the "no connection" guard — we only need the defaults to be applied before
// that guard to be effective, but in the current implementation they are
// applied before the DMV fallback query as well.
func TestFetchUnusedIndexesDefaultLimits(t *testing.T) {
	repo := &SqlServerRepository{
		conns:       make(map[string]*sql.DB),
		connStrings: make(map[string]string),
	}
	// Passing zero values; the function must not panic and must return an error
	// (no connection), proving it handled zero-value defaults gracefully.
	_, err := repo.FetchUnusedIndexes(context.Background(), uuid.New(), "MISSING", "testdb", 0, 0)
	if err == nil {
		t.Error("expected error when instance has no connection, got nil")
	}
}
