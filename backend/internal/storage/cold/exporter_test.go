// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/exporter_test.go
// Purpose: Integration-style tests for the cold storage exporter, verifying orchestration and purge logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package cold

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestExporter_Run(t *testing.T) {
	mockDB, err := pgxmock.NewPool()
	assert.NoError(t, err)
	defer mockDB.Close()

	mockUploader := new(MockS3Client)
	cfg := &Config{
		Enabled:         true,
		ExportLagDays:   2,
		ExportBatchSize: 100,
		LocalStagingDir: "/tmp/sql-optima-test",
	}
	uploader := &S3Uploader{client: mockUploader, cfg: cfg}
	watermark := NewWatermarkStore(mockDB)

	exporter := NewExporter(mockDB, uploader, watermark, cfg)

	ctx := context.Background()
	serverID := "550e8400-e29b-41d4-a716-446655440000"
	runID := int64(123)

	// 1. Audit log start
	mockDB.ExpectQuery("INSERT INTO coldstorage.runs").
		WillReturnRows(pgxmock.NewRows([]string{"cold_export_run_id"}).AddRow(runID))

	// 2. Fetch active servers
	mockDB.ExpectQuery("SELECT id::text FROM monitored_servers").
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(serverID))

	// 3. Register a dummy table
	exporter.RegisterTable(TableExportConfig{
		TableName:       "test_metrics",
		Engine:          "sqlserver",
		TimestampColumn: "capture_timestamp",
		ServerIDColumn:  "server_id",
		QueryFn: func(ctx context.Context, db DB, sid string, from, to time.Time, offset, limit int) ([]any, error) {
			if offset > 0 {
				return nil, nil // no more data
			}
			return []any{"dummy_row"}, nil
		},
		WriteParquetFn: func(path string, rows []any) (int, error) {
			err := os.WriteFile(path, []byte("dummy parquet"), 0644)
			return 1, err
		},
	})

	// 4. Watermark check
	mockDB.ExpectQuery("SELECT last_exported_at FROM coldstorage.watermarks").
		WithArgs("test_metrics", serverID).
		WillReturnRows(pgxmock.NewRows([]string{"last_exported_at"}).AddRow(time.Now().AddDate(0, 0, -5)))

	// 5, 6, 7. Compression, Upload, Watermark update (per day)
	for i := 0; i < 3; i++ {
		mockDB.ExpectQuery("SELECT COUNT").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(true))

		mockUploader.On("PutObject", mock.Anything, mock.Anything).Return(&s3.PutObjectOutput{}, nil).Once()

		mockDB.ExpectExec("INSERT INTO coldstorage.watermarks").
			WithArgs("test_metrics", serverID, pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
	}

	// 8. Audit log finish
	mockDB.ExpectExec("UPDATE coldstorage.runs").
		WithArgs("success", 1, 0, int64(3), pgxmock.AnyArg(), "", runID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err = exporter.Run(ctx)
	assert.NoError(t, err)

	mockDB.ExpectationsWereMet()
	mockUploader.AssertExpectations(t)
}

func TestExporter_Purge(t *testing.T) {
	mockDB, err := pgxmock.NewPool()
	assert.NoError(t, err)
	defer mockDB.Close()

	cfg := &Config{
		PurgeRetentionDays: 30,
	}
	exporter := NewExporter(mockDB, nil, nil, cfg)

	tableName := "test_metrics"
	exporter.RegisterTable(TableExportConfig{TableName: tableName})

	// 1. Min Watermark check
	minExported := time.Now().AddDate(0, 0, -40)
	mockDB.ExpectQuery("SELECT MIN").
		WithArgs(tableName).
		WillReturnRows(pgxmock.NewRows([]string{"min"}).AddRow(minExported))

	// 2. Drop chunks
	mockDB.ExpectExec("SELECT drop_chunks").
		WithArgs(tableName, pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("SELECT", 1))

	exporter.Purge(context.Background())

	assert.NoError(t, mockDB.ExpectationsWereMet())
}
