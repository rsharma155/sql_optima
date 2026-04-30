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
	"database/sql"
	"fmt"
	"log"
)

// BGWriterStats represents PostgreSQL background writer metrics from pg_stat_bgwriter.
type BGWriterStats struct {
	CheckpointsTimed    int64   // Number of timed checkpoints
	CheckpointsReq      int64   // Number of requested checkpoints
	CheckpointWriteTime float64 // Time spent writing checkpoint files (ms)
	CheckpointSyncTime  float64 // Time spent syncing checkpoint files (ms)
	BuffersCheckpoint   int64   // Buffers written by checkpoints
	BuffersClean        int64   // Buffers written by background writer
	MaxwrittenClean     int64   // Times bgwriter stopped due to full buffers
	BuffersBackend      int64   // Buffers written directly by backends
	BuffersAlloc        int64   // Buffers allocated by backends
}

// FetchBGWriterStats retrieves background writer and checkpointer statistics from pg_stat_bgwriter.
// These metrics are collected and stored in TimescaleDB for historical analysis.
func (c *PgRepository) FetchBGWriterStats(instanceName string) (*BGWriterStats, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("[POSTGRES] FetchBGWriterStats: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return nil, fmt.Errorf("connection not found after reconnect")
			}
		} else {
			return nil, fmt.Errorf("connection not found")
		}
	}

	stats := &BGWriterStats{}
	query := `
		/* SQL_OPTIMA */ SELECT   
			checkpoints_timed,
			checkpoints_req,
			checkpoint_write_time,
			checkpoint_sync_time,
			buffers_checkpoint,
			buffers_clean,
			maxwritten_clean,
			buffers_backend,
			buffers_alloc
		FROM pg_stat_bgwriter
	`

	err := db.QueryRow(query).Scan(
		&stats.CheckpointsTimed,
		&stats.CheckpointsReq,
		&stats.CheckpointWriteTime,
		&stats.CheckpointSyncTime,
		&stats.BuffersCheckpoint,
		&stats.BuffersClean,
		&stats.MaxwrittenClean,
		&stats.BuffersBackend,
		&stats.BuffersAlloc,
	)

	if err != nil {
		return nil, err
	}

	return stats, nil
}

// ArchiverStats represents PostgreSQL WAL archiver metrics from pg_stat_archiver.
type ArchiverStats struct {
	ArchivedCount   int64          // Number of WAL files successfully archived
	FailedCount     int64          // Number of WAL file archive failures
	LastArchivedWal sql.NullString // Name of last successfully archived WAL
	LastFailedWal   sql.NullString // Name of last failed WAL file
}

// FetchArchiverStats retrieves WAL archiver statistics from pg_stat_archiver.
func (c *PgRepository) FetchArchiverStats(instanceName string) (*ArchiverStats, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("[POSTGRES] FetchArchiverStats: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
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
			last_failed_wal
		FROM pg_stat_archiver
	`

	err := db.QueryRow(query).Scan(
		&stats.ArchivedCount,
		&stats.FailedCount,
		&stats.LastArchivedWal,
		&stats.LastFailedWal,
	)

	if err != nil {
		return nil, err
	}

	return stats, nil
}
