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
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/parquet-go/parquet-go"
)

// TableExportConfig describes one table to export.
type TableExportConfig struct {
	TableName       string
	Engine          string
	TimestampColumn string
	ServerIDColumn  string
	QueryFn         func(ctx context.Context, db DB, serverID string, from, to time.Time, offset, limit int) ([]any, error)
	WriteParquetFn  func(localPath string, rows []any) (int, error)
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
	log.Printf("[ColdExporter] Starting export cycle, cutoff=%s", cutoff.Format(time.RFC3339))

	// Start audit log
	var runID int64
	err := e.db.QueryRow(ctx, `INSERT INTO coldstorage.runs (run_started, status) VALUES (NOW(), 'running') RETURNING cold_export_run_id`).Scan(&runID)
	if err != nil {
		log.Printf("[ColdExporter] Warning: failed to start audit log: %v", err)
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

	var totalRows int64
	var totalBytes int64
	var tablesOk int
	var tablesFailed int

	for _, table := range e.tables {
		tableOk := true
		for _, serverID := range serverIDs {
			rows, bytes, err := e.exportTableForServerWithMetrics(ctx, table, serverID, cutoff)
			if err != nil {
				log.Printf("[ColdExporter] ERROR exporting %s / %s: %v", table.TableName, serverID, err)
				tableOk = false
			}
			totalRows += rows
			totalBytes += bytes
		}
		if tableOk {
			tablesOk++
		} else {
			tablesFailed++
		}
	}

	status := "success"
	if tablesFailed > 0 {
		status = "partial"
	}

	e.finishRun(ctx, runID, status, tablesOk, tablesFailed, totalRows, totalBytes, "")

	log.Printf("[ColdExporter] Export cycle complete. Status: %s, Rows: %d, Bytes: %d", status, totalRows, totalBytes)

	// Perform safety purge if enabled
	if e.cfg.PurgeRetentionDays > 0 {
		e.Purge(ctx)
	}

	return nil
}

// Purge drops old chunks from TimescaleDB for all registered tables,
// but only if the data has been successfully archived to cold storage.
func (e *Exporter) Purge(ctx context.Context) {
	log.Printf("[ColdExporter] Starting safety purge, retention=%d days", e.cfg.PurgeRetentionDays)

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

		log.Printf("[ColdExporter] Purging %s older than %s", table.TableName, effectiveCutoff.Format(time.RFC3339))

		const qDrop = `SELECT drop_chunks($1, older_than => $2::TIMESTAMPTZ)`
		_, err = e.db.Exec(ctx, qDrop, simpleName, effectiveCutoff)
		if err != nil {
			log.Printf("[ColdExporter] ERROR purging %s: %v", table.TableName, err)
		}
	}

	log.Println("[ColdExporter] Safety purge complete")
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
		log.Printf("[ColdExporter] Warning: failed to finish audit log: %v", err)
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
		// This ensures we don't archive data that is still mutable or being updated.
		compressed, err := e.isCompressed(ctx, table.TableName, current, nextDay)
		if err != nil {
			log.Printf("[ColdExporter] Warning: compression check failed for %s: %v", table.TableName, err)
			// Proceed anyway if check fails, as fallback.
			compressed = true
		}

		if !compressed {
			log.Printf("[ColdExporter] Skipping %s for %s (chunks not yet compressed)", table.TableName, current.Format("2006-01-02"))
			break // Stop for this table/server to maintain order
		}

		partNum++
		localPath, uploaded, rowCount, byteCount, err := e.exportDayWithMetrics(ctx, table, serverID, current, nextDay, partNum)
		if localPath != "" {
			_ = os.Remove(localPath)
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

	tmpPath := filepath.Join(e.cfg.LocalStagingDir, fmt.Sprintf("%s_%s_%s_part%d.parquet", table.TableName, serverID, from.Format("20060102"), partNum))
	rowCount, writeErr := table.WriteParquetFn(tmpPath, allRows)
	if writeErr != nil {
		return tmpPath, false, 0, 0, fmt.Errorf("write parquet %s: %w", table.TableName, writeErr)
	}

	fInfo, _ := os.Stat(tmpPath)
	byteCount := fInfo.Size()

	objectKey := e.uploader.ObjectKey(table.Engine, table.TableName, serverID, from.Format("2006"), from.Format("01"), from.Format("02"), partNum)
	if uploadErr := e.uploader.UploadFile(ctx, tmpPath, objectKey); uploadErr != nil {
		return tmpPath, false, 0, 0, fmt.Errorf("upload %s: %w", objectKey, uploadErr)
	}

	log.Printf("[ColdExporter] Uploaded %s (%d rows) → s3://%s/%s", table.TableName, rowCount, e.cfg.Bucket, objectKey)

	// Phase 2: Register with Iceberg catalog if enabled
	if e.iceberg != nil {
		s3Path := fmt.Sprintf("s3://%s/%s", e.cfg.Bucket, objectKey)
		if err := e.iceberg.AppendDataFile(ctx, table.TableName, s3Path, int64(rowCount)); err != nil {
			log.Printf("[ColdExporter] Warning: failed to register %s with Iceberg: %v", table.TableName, err)
		}
	}

	return tmpPath, true, int64(rowCount), byteCount, nil
}

func (e *Exporter) fetchServerIDs(ctx context.Context) ([]string, error) {
	rows, err := e.db.Query(ctx, `SELECT id::text FROM monitored_servers WHERE is_active = true`)
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

// WriteTypedParquet is a generic helper for writing a slice of typed rows to Parquet.
func WriteTypedParquet[T any](path string, rows []T) (int, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	writer := parquet.NewGenericWriter[T](f, parquet.Compression(&parquet.Snappy))
	n, err := writer.Write(rows)
	if err != nil {
		writer.Close()
		return 0, err
	}
	if err := writer.Close(); err != nil {
		return 0, err
	}
	return n, nil
}
