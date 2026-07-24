// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/exporter.go
// Purpose: Core orchestrator for the cold storage archival pipeline. Handles batching, atomic writes, and safety purges.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package cold

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/parquet-go/parquet-go"
)

// workerPoolSize is the number of concurrent table/server export goroutines.
// Each goroutine is I/O-bound (DB read + S3 upload) so 8 is a safe default.
const workerPoolSize = 8

// TableExportConfig describes one table to export.
type TableExportConfig struct {
	TableName       string
	Engine          string
	TimestampColumn string
	ServerIDColumn  string
	// SkipCompressionCheck disables the TimescaleDB chunk-compression gate for this
	// table. Set to true for non-hypertables and event tables where compression
	// does not apply (e.g. pg_failed_login_events, pg_basebackup_history).
	SkipCompressionCheck bool
	QueryFn              func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error)
	WriteParquetFn       func(localPath string, rows []any) (int, error)
}

// Exporter orchestrates cold storage export for all registered tables.
type Exporter struct {
	db        DB
	uploader  *S3Uploader
	watermark *WatermarkStore
	iceberg   *IcebergRegistrar
	cfg       *Config
	tables    []TableExportConfig
}

// NewExporter creates a new Exporter.
func NewExporter(db DB, uploader *S3Uploader, wm *WatermarkStore, cfg *Config) *Exporter {
	var iceberg *IcebergRegistrar
	if cfg.CatalogURL != "" {
		iceberg = NewIcebergRegistrar(cfg.CatalogURL, "default")
	}

	return &Exporter{
		db:        db,
		uploader:  uploader,
		watermark: wm,
		iceberg:   iceberg,
		cfg:       cfg,
	}
}

// RegisterTable adds a table to the export pipeline.
func (e *Exporter) RegisterTable(t TableExportConfig) {
	e.tables = append(e.tables, t)
}

// Run executes one full export cycle for all registered tables.
func (e *Exporter) Run(ctx context.Context) error {
	if !e.cfg.Enabled {
		return nil
	}

	cutoff := e.cfg.ExportCutoff()
	slog.Info("cold export cycle starting", "cutoff", cutoff.Format(time.RFC3339))

	// Start audit log
	var runID int64
	err := e.db.QueryRow(ctx, `INSERT INTO coldstorage.runs (run_started, status) VALUES (NOW(), 'running') RETURNING cold_export_run_id`).Scan(&runID)
	if err != nil {
		slog.Warn("cold export audit start failed", "err", err)
	}

	serverIDs, err := e.fetchServerIDs(ctx)
	if err != nil {
		e.finishRun(ctx, runID, "failed", 0, 0, 0, 0, fmt.Sprintf("fetch server IDs: %v", err))
		return fmt.Errorf("cold/exporter: fetch server IDs: %w", err)
	}

	if err := os.MkdirAll(e.cfg.LocalStagingDir, 0750); err != nil {
		e.finishRun(ctx, runID, "failed", 0, 0, 0, 0, fmt.Sprintf("create staging dir: %v", err))
		return fmt.Errorf("cold/exporter: create staging dir: %w", err)
	}

	// Remove any leftover .tmp files from a previous crashed run.
	e.cleanStagingDir()

	type exportResult struct {
		tableName string
		rows      int64
		bytes     int64
		err       error
	}

	resultCh := make(chan exportResult, len(e.tables)*len(serverIDs))
	sem := make(chan struct{}, workerPoolSize)
	var wg sync.WaitGroup

	for _, table := range e.tables {
		for _, serverID := range serverIDs {
			wg.Add(1)
			sem <- struct{}{}
			go func(t TableExportConfig, sid string) {
				defer wg.Done()
				defer func() { <-sem }()
				r, b, err := e.exportTableForServerWithMetrics(ctx, t, sid, cutoff)
				resultCh <- exportResult{tableName: t.TableName, rows: r, bytes: b, err: err}
			}(table, serverID)
		}
	}

	wg.Wait()
	close(resultCh)

	var totalRows int64
	var totalBytes int64
	failedTables := make(map[string]struct{})
	for res := range resultCh {
		if res.err != nil {
			slog.Error("cold export table failed", "table", res.tableName, "err", res.err)
			failedTables[res.tableName] = struct{}{}
		}
		totalRows += res.rows
		totalBytes += res.bytes
	}

	tablesOk := len(e.tables) - len(failedTables)
	tablesFailed := len(failedTables)

	status := "success"
	if tablesFailed > 0 {
		status = "partial"
	}

	e.finishRun(ctx, runID, status, tablesOk, tablesFailed, totalRows, totalBytes, "")

	slog.Info("cold export cycle complete", "status", status, "rows", totalRows, "bytes", totalBytes)

	// Perform safety purge if enabled
	if e.cfg.PurgeRetentionDays > 0 {
		e.Purge(ctx)
	}

	return nil
}

// Purge drops old chunks from TimescaleDB for all registered tables,
// but only if the data has been successfully archived to cold storage.
func (e *Exporter) Purge(ctx context.Context) {
	slog.Info("cold purge starting", "retention_days", e.cfg.PurgeRetentionDays)

	purgeCutoff := time.Now().UTC().AddDate(0, 0, -e.cfg.PurgeRetentionDays)

	for _, table := range e.tables {
		// 1. Find the oldest 'last_exported_at' across all servers for this table.
		// If any server is behind, we don't purge that window.
		const qWatermark = `
			SELECT MIN(last_exported_at)
			FROM coldstorage.watermarks
			WHERE table_name = $1`

		var minExportedAt time.Time
		err := e.db.QueryRow(ctx, qWatermark, table.TableName).Scan(&minExportedAt)
		if err != nil || minExportedAt.IsZero() {
			// No exports yet or error, skip purge for safety
			continue
		}

		// 2. The effective purge cutoff is the MIN of our desired retention and the actual archive coverage.
		effectiveCutoff := purgeCutoff
		if minExportedAt.Before(effectiveCutoff) {
			effectiveCutoff = minExportedAt
		}

		// 3. Drop chunks
		// TimescaleDB's drop_chunks is hypertable-specific.
		// We handle schema prefixes.
		parts := strings.Split(table.TableName, ".")
		simpleName := parts[len(parts)-1]

		slog.Info("cold purge table", "table", table.TableName, "older_than", effectiveCutoff.Format(time.RFC3339))

		const qDrop = `SELECT drop_chunks($1, older_than => $2::TIMESTAMPTZ)`
		_, err = e.db.Exec(ctx, qDrop, simpleName, effectiveCutoff)
		if err != nil {
			slog.Error("cold purge failed", "table", table.TableName, "err", err)
		}
	}

	slog.Info("cold purge complete")
}

func (e *Exporter) finishRun(ctx context.Context, runID int64, status string, ok, failed int, rows, bytes int64, errDetail string) {
	if runID == 0 {
		return
	}
	const q = `
		UPDATE coldstorage.runs
		SET run_finished = NOW(),
		    status = $1,
		    tables_ok = $2,
		    tables_failed = $3,
		    total_rows = $4,
		    total_bytes = $5,
		    error_detail = $6
		WHERE cold_export_run_id = $7`
	_, err := e.db.Exec(ctx, q, status, ok, failed, rows, bytes, errDetail, runID)
	if err != nil {
		slog.Warn("cold export audit finish failed", "err", err)
	}
}

func (e *Exporter) exportTableForServerWithMetrics(ctx context.Context, table TableExportConfig, serverID string, cutoff time.Time) (int64, int64, error) {
	from, err := e.watermark.Get(ctx, table.TableName, serverID)
	if err != nil {
		return 0, 0, err
	}
	if from.IsZero() {
		from = cutoff.AddDate(0, 0, -90)
	}
	if !from.Before(cutoff) {
		return 0, 0, nil
	}

	current := from.Truncate(24 * time.Hour)
	partNum := 0
	var totalRows int64
	var totalBytes int64

	for current.Before(cutoff) {
		nextDay := current.Add(24 * time.Hour)

		// Optimization: only export if the window is fully compressed in TimescaleDB.
		// Non-hypertables and event tables set SkipCompressionCheck=true to bypass this.
		if !table.SkipCompressionCheck {
			compressed, err := e.isCompressed(ctx, table.TableName, current, nextDay)
			if err != nil {
				slog.Warn("cold compression check failed", "table", table.TableName, "err", err)
				compressed = true // safe fallback: allow export on check error
			}
			if !compressed {
				slog.Info("cold export skip uncompressed", "table", table.TableName, "day", current.Format("2006-01-02"))
				break // stop for this table/server to preserve order
			}
		}

		partNum++
		localPath, uploaded, rowCount, byteCount, err := e.exportDayWithMetrics(ctx, table, serverID, current, nextDay, partNum)
		if localPath != "" {
			_ = os.Remove(localPath) // clean final path; .tmp already renamed or cleaned up
		}
		if err != nil {
			return totalRows, totalBytes, err
		}

		if uploaded {
			totalRows += rowCount
			totalBytes += byteCount
			if wmErr := e.watermark.Set(ctx, table.TableName, serverID, nextDay); wmErr != nil {
				return totalRows, totalBytes, wmErr
			}
		}
		current = nextDay
	}

	return totalRows, totalBytes, nil
}

func (e *Exporter) isCompressed(ctx context.Context, tableName string, from, to time.Time) (bool, error) {
	// TimescaleDB stores hypertable info in chunks view.
	// We check if any chunks in this range are UNCOMPRESSED.
	parts := strings.Split(tableName, ".")
	simpleName := parts[len(parts)-1]

	const q = `
		SELECT COUNT(*) = 0
		FROM timescaledb_information.chunks
		WHERE hypertable_name = $1
		  AND range_start >= $2
		  AND range_end   <= $3
		  AND is_compressed = FALSE`

	var allCompressed bool
	err := e.db.QueryRow(ctx, q, simpleName, from, to).Scan(&allCompressed)
	if err != nil {
		// If the table is not a hypertable or view doesn't exist, this might fail.
		// In that case, we default to TRUE to allow the export.
		return true, nil
	}
	return allCompressed, nil
}

func (e *Exporter) exportDayWithMetrics(ctx context.Context, table TableExportConfig, serverID string, from, to time.Time, partNum int) (string, bool, int64, int64, error) {
	allRows := make([]any, 0, e.cfg.ExportBatchSize)
	offset := 0

	for {
		batch, fetchErr := table.QueryFn(ctx, e.db, serverID, from, to, offset, e.cfg.ExportBatchSize)
		if fetchErr != nil {
			return "", false, 0, 0, fmt.Errorf("query %s: %w", table.TableName, fetchErr)
		}
		if len(batch) == 0 {
			break
		}
		allRows = append(allRows, batch...)
		if len(batch) < e.cfg.ExportBatchSize {
			break
		}
		offset += e.cfg.ExportBatchSize
	}

	if len(allRows) == 0 {
		return "", false, 0, 0, nil
	}

	// Write to *.tmp first, then rename for atomic delivery.
	// A killed process leaves a .tmp file that is cleaned up on next run.
	tmpPath := filepath.Join(e.cfg.LocalStagingDir, fmt.Sprintf("%s_%s_%s_part%d.parquet.tmp", table.TableName, serverID, from.Format("20060102"), partNum))
	finalPath := tmpPath[:len(tmpPath)-4] // strip .tmp suffix

	rowCount, writeErr := table.WriteParquetFn(tmpPath, allRows)
	if writeErr != nil {
		return tmpPath, false, 0, 0, fmt.Errorf("write parquet %s: %w", table.TableName, writeErr)
	}

	if renameErr := os.Rename(tmpPath, finalPath); renameErr != nil {
		return tmpPath, false, 0, 0, fmt.Errorf("rename parquet %s: %w", table.TableName, renameErr)
	}

	var byteCount int64
	if fInfo, statErr := os.Stat(finalPath); statErr == nil {
		byteCount = fInfo.Size()
	}

	objectKey := e.uploader.ObjectKey(table.Engine, table.TableName, serverID, from.Format("2006"), from.Format("01"), from.Format("02"), partNum)
	if uploadErr := e.uploader.UploadFile(ctx, finalPath, objectKey); uploadErr != nil {
		return finalPath, false, 0, 0, fmt.Errorf("upload %s: %w", objectKey, uploadErr)
	}

	slog.Info("cold export uploaded", "table", table.TableName, "rows", rowCount, "bucket", e.cfg.Bucket, "key", objectKey)

	// Phase 2: Register with Iceberg catalog if enabled
	if e.iceberg != nil {
		s3Path := fmt.Sprintf("s3://%s/%s", e.cfg.Bucket, objectKey)
		if err := e.iceberg.AppendDataFile(ctx, table.TableName, s3Path, int64(rowCount)); err != nil {
			slog.Warn("cold iceberg register failed", "table", table.TableName, "err", err)
		}
	}

	return tmpPath, true, int64(rowCount), byteCount, nil
}

// cleanStagingDir removes orphaned .tmp files left by a previously crashed export.
func (e *Exporter) cleanStagingDir() {
	entries, err := os.ReadDir(e.cfg.LocalStagingDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".parquet.tmp") {
			path := filepath.Join(e.cfg.LocalStagingDir, entry.Name())
			if rmErr := os.Remove(path); rmErr == nil {
				slog.Info("cold staging orphan removed", "file", entry.Name())
			}
		}
	}
}

func (e *Exporter) fetchServerIDs(ctx context.Context) ([]string, error) {
	rows, err := e.db.Query(ctx, `SELECT id::text FROM optima_servers WHERE is_active = true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// pageBufferSize controls how much data is buffered before a Parquet page is flushed.
// 8 MB produces row groups large enough for DuckDB/Trino predicate pushdown to be
// effective, while staying well within typical memory budgets.
const pageBufferSize = 8 * 1024 * 1024

// WriteTypedParquet is a generic helper for writing a slice of typed rows to Parquet.
func WriteTypedParquet[T any](path string, rows []T) (int, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	writer := parquet.NewGenericWriter[T](f,
		parquet.Compression(&parquet.Snappy),
		parquet.PageBufferSize(pageBufferSize),
	)
	n, err := writer.Write(rows)
	if err != nil {
		_ = writer.Close()
		return 0, err
	}
	if err := writer.Close(); err != nil {
		return 0, err
	}
	return n, nil
}
