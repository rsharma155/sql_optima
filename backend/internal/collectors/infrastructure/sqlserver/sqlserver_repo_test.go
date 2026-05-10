// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for SQL Server repository methods.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package sqlserver

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestFetchSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	repo := NewSQLServerSnapshotRepository(db)
	lastWatermark := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{
		"db_id", "database_name", "total_executions", "last_execution_time",
		"total_cpu_ms", "total_elapsed_ms", "total_logical_reads", "total_logical_writes",
		"total_physical_reads", "total_rows", "total_grant_kb", "max_worker_time",
		"max_logical_reads", "max_dop", "max_grant_kb", "max_rows", "statement_text",
		"query_text_raw", "query_plan_hash", "query_hash", "plan_handle",
	}).AddRow(
		5, "testdb", 100, time.Now(),
		1000, 2000, 500, 100,
		50, 1000, 1024, 100,
		50, 4, 256, 100, "SELECT * FROM test",
		"SELECT * FROM test", []byte("plan_hash"), []byte("query_hash"), []byte("plan_handle"),
	)

	mock.ExpectQuery("SELECT").WithArgs(sqlmock.AnyArg()).WillReturnRows(rows)

	snapshots, err := repo.FetchSnapshot(context.Background(), lastWatermark)

	assert.NoError(t, err)
	assert.Len(t, snapshots, 1)
	assert.Equal(t, "testdb", snapshots[0].DatabaseName)
	assert.Equal(t, int64(100), snapshots[0].TotalExecutions)
}

func TestFetchSessionEnrichment(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	repo := NewSQLServerSnapshotRepository(db)

	rows := sqlmock.NewRows([]string{
		"plan_handle", "login_name", "application_name", "database_name", "is_user_workload",
	}).AddRow(
		[]byte("plan_handle"), "test_user", "test_app", "testdb", 1,
	)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	enrichments, err := repo.FetchSessionEnrichment(context.Background())

	assert.NoError(t, err)
	assert.Len(t, enrichments, 1)
	assert.Equal(t, "test_user", enrichments[0].LoginName)
	assert.Equal(t, 1, enrichments[0].IsUserWorkload)
}

func TestGetSqlServerStartTime(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	repo := NewSQLServerSnapshotRepository(db)
	startTime := time.Now()

	rows := sqlmock.NewRows([]string{"sqlserver_start_time"}).AddRow(startTime)
	mock.ExpectQuery("SELECT sqlserver_start_time").WillReturnRows(rows)

	result, err := repo.GetSqlServerStartTime(context.Background())

	assert.NoError(t, err)
	assert.True(t, result.Equal(startTime))
}
