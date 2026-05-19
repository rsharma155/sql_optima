// Package repository provides data access layer for database operations.
// It handles connections and queries for both PostgreSQL and SQL Server databases.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL background process monitoring (BGWriter and Archiver).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// BGWriterStats represents PostgreSQL background writer metrics from pg_stat_bgwriter.
type BGWriterStats struct {
	CheckpointsTimed    int64     // Number of timed checkpoints
	CheckpointsReq      int64     // Number of requested checkpoints
	CheckpointWriteTime float64   // Time spent writing checkpoint files (ms)
	CheckpointSyncTime  float64   // Time spent syncing checkpoint files (ms)
	BuffersCheckpoint   int64     // Buffers written by checkpoints
	BuffersClean        int64     // Buffers written by background writer
	MaxwrittenClean     int64     // Times bgwriter stopped due to full buffers
	BuffersBackend      int64     // Buffers written directly by backends
	BuffersBackendFsync int64     // Buffers fsynced by backends
	BuffersAlloc        int64     // Buffers allocated by backends
	StatsReset          time.Time // Time of last statistics reset
}

// FetchBGWriterStats retrieves background writer and checkpointer statistics from pg_stat_bgwriter.
// These metrics are collected and stored in TimescaleDB for historical analysis.
func (c *PgRepository) FetchBGWriterStats(ctx context.Context, instanceName string) (*BGWriterStats, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		slog.Info("[POSTGRES] FetchBGWriterStats: connection not found for %s, attempting reconnect", "val", instanceName)
		if c.reconnectInstance(ctx, instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return nil, fmt.Errorf("connection not found after reconnect")
			}
		} else {
			return nil, fmt.Errorf("connection not found")
		}
	}

	stats := &BGWriterStats{}

	versionNum := c.GetPgVersion(ctx, instanceName)

	var query string
	if versionNum >= 170000 {
		query = `
			/* SQL_OPTIMA */ SELECT   
				(SELECT COALESCE(checkpoints_timed,0) FROM pg_stat_checkpointer),
				(SELECT COALESCE(checkpoints_req,0) FROM pg_stat_checkpointer),
				(SELECT COALESCE(checkpoint_write_time,0) FROM pg_stat_checkpointer),
				(SELECT COALESCE(checkpoint_sync_time,0) FROM pg_stat_checkpointer),
				(SELECT COALESCE(buffers_written,0) FROM pg_stat_checkpointer),
				buffers_clean,
				maxwritten_clean,
				0, -- buffers_backend moved to pg_stat_io
				0, -- buffers_backend_fsync moved to pg_stat_io
				0, -- buffers_alloc moved to pg_stat_io
				stats_reset
			FROM pg_stat_bgwriter
		`
	} else {
		query = `
			/* SQL_OPTIMA */ SELECT   
				checkpoints_timed,
				checkpoints_req,
				checkpoint_write_time,
				checkpoint_sync_time,
				buffers_checkpoint,
				buffers_clean,
				maxwritten_clean,
				buffers_backend,
				buffers_backend_fsync,
				buffers_alloc,
				stats_reset
			FROM pg_stat_bgwriter
		`
	}

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, query).Scan(
		&stats.CheckpointsTimed,
		&stats.CheckpointsReq,
		&stats.CheckpointWriteTime,
		&stats.CheckpointSyncTime,
		&stats.BuffersCheckpoint,
		&stats.BuffersClean,
		&stats.MaxwrittenClean,
		&stats.BuffersBackend,
		&stats.BuffersBackendFsync,
		&stats.BuffersAlloc,
		&stats.StatsReset,
	)

	if err != nil {
		return nil, err
	}

	return stats, nil
}

// ArchiverStats represents PostgreSQL WAL archiver metrics from pg_stat_archiver.
type ArchiverStats struct {
	ArchivedCount    int64          // Number of WAL files successfully archived
	FailedCount      int64          // Number of WAL file archive failures
	LastArchivedWal  sql.NullString // Name of last successfully archived WAL
	LastArchivedTime *time.Time     // Time of last successfully archived WAL
	LastFailedWal    sql.NullString // Name of last failed WAL file
	LastFailedTime   *time.Time     // Time of last failed WAL file
	StatsReset       time.Time      // Time of last statistics reset
}

// FetchArchiverStats retrieves WAL archiver statistics from pg_stat_archiver.
func (c *PgRepository) FetchArchiverStats(ctx context.Context, instanceName string) (*ArchiverStats, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		slog.Info("[POSTGRES] FetchArchiverStats: connection not found for %s, attempting reconnect", "val", instanceName)
		if c.reconnectInstance(ctx, instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return nil, fmt.Errorf("connection not found after reconnect")
			}
		} else {
			return nil, fmt.Errorf("connection not found")
		}
	}

	stats := &ArchiverStats{}
	query := `
		SELECT /* SQL_OPTIMA */   
			archived_count,
			failed_count,
			last_archived_wal,
			last_archived_time,
			last_failed_wal,
			last_failed_time,
			stats_reset
		FROM pg_stat_archiver
	`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, query).Scan(
		&stats.ArchivedCount,
		&stats.FailedCount,
		&stats.LastArchivedWal,
		&stats.LastArchivedTime,
		&stats.LastFailedWal,
		&stats.LastFailedTime,
		&stats.StatsReset,
	)

	if err != nil {
		return nil, err
	}

	return stats, nil
}

type PgWALStats struct {
	WalBytes   int64
	WalRecords int64
	WalFpi     int64
}

func (c *PgRepository) FetchPgWALStats(ctx context.Context, instanceName string) (*PgWALStats, error) {
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	stats := &PgWALStats{}
	err := db.QueryRowContext(ctx, "SELECT wal_bytes, wal_records, wal_fpi FROM pg_stat_wal").Scan(
		&stats.WalBytes, &stats.WalRecords, &stats.WalFpi,
	)
	if err != nil {
		// Fallback for PG < 14
		return &PgWALStats{}, nil
	}
	return stats, nil
}
