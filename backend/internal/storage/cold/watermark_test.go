// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/watermark_test.go
// Purpose: Unit tests for the watermark store, verifying persistence and upsert logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package cold

import (
	"context"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
)

func TestWatermarkStore_Get(t *testing.T) {
	mock, err := pgxmock.NewPool()
	assert.NoError(t, err)
	defer mock.Close()

	store := NewWatermarkStore(mock)
	ctx := context.Background()
	tableName := "test_table"
	serverID := "550e8400-e29b-41d4-a716-446655440000"
	lastExported := time.Now().Truncate(time.Second)

	// Case 1: Watermark exists
	mock.ExpectQuery("SELECT last_exported_at FROM coldstorage.watermarks").
		WithArgs(tableName, serverID).
		WillReturnRows(pgxmock.NewRows([]string{"last_exported_at"}).AddRow(lastExported))

	got, err := store.Get(ctx, tableName, serverID)
	assert.NoError(t, err)
	assert.True(t, lastExported.Equal(got))

	// Case 2: Watermark does not exist
	mock.ExpectQuery("SELECT last_exported_at FROM coldstorage.watermarks").
		WithArgs(tableName, serverID).
		WillReturnRows(pgxmock.NewRows([]string{"last_exported_at"})) // Empty row set triggers Scan error

	got, err = store.Get(ctx, tableName, serverID)
	assert.NoError(t, err)
	assert.True(t, got.IsZero())

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWatermarkStore_Set(t *testing.T) {
	mock, err := pgxmock.NewPool()
	assert.NoError(t, err)
	defer mock.Close()

	store := NewWatermarkStore(mock)
	ctx := context.Background()
	tableName := "test_table"
	serverID := "550e8400-e29b-41d4-a716-446655440000"
	exportedAt := time.Now().Truncate(time.Second)

	mock.ExpectExec("INSERT INTO coldstorage.watermarks").
		WithArgs(tableName, serverID, exportedAt).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = store.Set(ctx, tableName, serverID, exportedAt)
	assert.NoError(t, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}
