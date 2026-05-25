// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for sqlserver_phase2 repository methods.
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

// TestFetchWaitStatsCumulativeSignature is a compile-time check that the
// function accepts serverID so it can read from the local TimescaleDB.
func TestFetchWaitStatsCumulativeSignature(t *testing.T) {
	// Compile-time: verify the method exists with the expected types.
	var _ = (*SqlServerRepository)(nil).FetchWaitStatsCumulative
}

// TestFetchWaitStatsCumulativeNilPool verifies that when LocalPool is nil
// (no TimescaleDB available), the function returns an error without panicking.
func TestFetchWaitStatsCumulativeNilPool(t *testing.T) {
	repo := &SqlServerRepository{
		conns:       make(map[string]*sql.DB),
		connStrings: make(map[string]string),
	}
	// LocalPool is nil — must not panic; should return error (no connection)
	result, err := repo.FetchWaitStatsCumulative(context.Background(), uuid.New(), "MISSING_INSTANCE")
	if err == nil {
		t.Error("expected error when instance has no connection, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}
