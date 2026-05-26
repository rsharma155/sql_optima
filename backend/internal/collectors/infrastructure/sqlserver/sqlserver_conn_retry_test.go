// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for SQL Server connection retry on snapshot queries.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package sqlserver

import (
	"context"
	"database/sql"
	"io"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchSessionEnrichment_retriesAfterEOF(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLServerSnapshotRepository(db)

	rows := sqlmock.NewRows([]string{
		"plan_handle", "query_hash", "login_name", "application_name", "database_name", "is_user_workload",
	}).AddRow(
		[]byte("plan_handle"), []byte{0, 0, 0, 0, 0, 0, 0, 1}, "test_user", "test_app", "testdb", 1,
	)

	mock.ExpectQuery("SELECT").WillReturnError(io.EOF)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	enrichments, err := repo.FetchSessionEnrichment(context.Background())

	assert.NoError(t, err)
	assert.Len(t, enrichments, 1)
	assert.Equal(t, "test_user", enrichments[0].LoginName)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchSessionEnrichment_reconnectBeforeRetry(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSQLServerSnapshotRepository(db)
	reconnected := false
	repo.SetConnRecovery(func(ctx context.Context) (*sql.DB, bool) {
		reconnected = true
		return db, true
	})

	rows := sqlmock.NewRows([]string{
		"plan_handle", "query_hash", "login_name", "application_name", "database_name", "is_user_workload",
	}).AddRow(
		[]byte("plan_handle"), []byte{0, 0, 0, 0, 0, 0, 0, 1}, "test_user", "test_app", "testdb", 1,
	)

	mock.ExpectQuery("SELECT").WillReturnError(io.EOF)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	enrichments, err := repo.FetchSessionEnrichment(context.Background())

	assert.NoError(t, err)
	assert.True(t, reconnected)
	assert.Len(t, enrichments, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}
