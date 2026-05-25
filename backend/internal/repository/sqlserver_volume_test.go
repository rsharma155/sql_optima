// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for sqlserver_volume repository methods.
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
	"github.com/stretchr/testify/assert"
)

func TestFetchVolumeStats_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := &SqlServerRepository{
		conns: map[string]*sql.DB{"TEST_INSTANCE": db},
	}

	instance := "test_instance"

	mock.ExpectQuery("sys.dm_os_volume_stats").
		WillReturnRows(sqlmock.NewRows([]string{
			"database_id", "database_name", "logical_file_name", "physical_name",
			"file_type", "file_size_mb", "volume_mount_point", "volume_label",
			"volume_total_gb", "volume_available_gb", "volume_free_pct",
		}).AddRow(5, "UserDB", "UserDB_Data", "C:\\Data\\UserDB.mdf", "ROWS", 1024.0, "C:\\", "OS", 100.0, 20.0, 20.0))

	out, err := repo.FetchVolumeStats(context.Background(), instance)
	assert.NoError(t, err)
	assert.Len(t, out, 1)
	assert.Equal(t, "UserDB", out[0].DatabaseName)
	assert.Equal(t, 20.0, out[0].VolumeFreePct)
}
