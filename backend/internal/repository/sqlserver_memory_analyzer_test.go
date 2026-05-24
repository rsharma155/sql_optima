// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for sqlserver_memory_analyzer repository methods.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestFetchMemoryAnalyzerSnapshot_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := &SqlServerRepository{
		conns: map[string]*sql.DB{"TEST_INSTANCE": db},
	}

	serverID := uuid.New()
	instance := "test_instance"

	// 1. SQL Memory vs Target
	mock.ExpectQuery("sys.dm_os_performance_counters").
		WillReturnRows(sqlmock.NewRows([]string{"total_mb", "target_mb"}).AddRow(8192, 16384))

	// 2. OS memory
	mock.ExpectQuery("sys.dm_os_sys_memory").
		WillReturnRows(sqlmock.NewRows([]string{"total_os_mb", "available_os_mb", "memory_state"}).AddRow(32768, 16384, "Available"))

	// 3. Process memory (including new columns)
	mock.ExpectQuery("sys.dm_os_process_memory").
		WillReturnRows(sqlmock.NewRows([]string{
			"process_physical_memory_low", "process_virtual_memory_low",
			"sql_physical_memory_in_use_mb", "sql_memory_utilization_pct",
			"sql_page_fault_count", "sql_locked_page_alloc_mb", "sql_large_page_alloc_mb",
		}).AddRow(false, false, 8000, 95, 100, 10, 20))

	// 4. Memory Grants Pending
	mock.ExpectQuery("Memory Grants Pending").
		WillReturnRows(sqlmock.NewRows([]string{"cntr_value"}).AddRow(0))

	// 5. Active grants
	mock.ExpectQuery("sys.dm_exec_query_memory_grants").
		WillReturnRows(sqlmock.NewRows([]string{"active_grants", "granted_mb", "requested_mb"}).AddRow(5, 1024, 1024))

	// 6. Waiting grants
	mock.ExpectQuery("sys.dm_exec_query_memory_grants").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// 7. PLE
	mock.ExpectQuery("Page life expectancy").
		WillReturnRows(sqlmock.NewRows([]string{"cntr_value"}).AddRow(3600))

	// 8. Plan cache
	mock.ExpectQuery("sys.dm_exec_cached_plans").
		WillReturnRows(sqlmock.NewRows([]string{"mb"}).AddRow(512))

	// 9. Sort/Hash Warnings
	mock.ExpectQuery("Sort Warnings").
		WillReturnRows(sqlmock.NewRows([]string{"sort_warn", "hash_warn"}).AddRow(10, 5))

	out, err := repo.FetchMemoryAnalyzerSnapshot(context.Background(), serverID, instance)
	assert.NoError(t, err)
	assert.NotNil(t, out)

	assert.Equal(t, int64(8192), out["sql_memory_used_mb"])
	assert.Equal(t, int64(16384), out["sql_memory_target_mb"])
	assert.Equal(t, int64(8000), out["sql_physical_memory_in_use_mb"])
	assert.Equal(t, int64(95), out["sql_memory_utilization_pct"])
	assert.Equal(t, int64(100), out["sql_page_fault_count"])
	assert.Equal(t, int64(10), out["sql_locked_page_alloc_mb"])
	assert.Equal(t, int64(20), out["sql_large_page_alloc_mb"])
}
