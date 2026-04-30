// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Backup & DR data collectors.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"context"
	"database/sql"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/domain/entities"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/domain/repositories"
	"time"
)

type PostgresBackupCollector struct {
	repo       *repositories.PostgresBackupRepository
	instanceID string
}

func NewPostgresBackupCollector(repo *repositories.PostgresBackupRepository, instanceID string) *PostgresBackupCollector {
	return &PostgresBackupCollector{
		repo:       repo,
		instanceID: instanceID,
	}
}

func (c *PostgresBackupCollector) CollectArchiverStats(ctx context.Context, db *sql.DB) error {
	var s entities.BackupArchiverStats
	s.TS = time.Now().UTC()
	s.InstanceID = c.instanceID

	var lastArchived, lastFailed sql.NullTime
	err := db.QueryRowContext(ctx, `
		SELECT archived_count, failed_count, last_archived_time, last_failed_time
		FROM pg_stat_archiver`).Scan(&s.ArchivedCount, &s.FailedCount, &lastArchived, &lastFailed)
	if err != nil {
		return err
	}

	if lastArchived.Valid {
		s.LastArchivedTime = &lastArchived.Time
	}
	if lastFailed.Valid {
		s.LastFailedTime = &lastFailed.Time
	}

	return c.repo.SaveArchiverStats(ctx, s)
}

func (c *PostgresBackupCollector) CollectWALRate(ctx context.Context, db *sql.DB) error {
	var s entities.WALRate
	s.TS = time.Now().UTC()
	s.InstanceID = c.instanceID

	// pg_stat_wal is available in PG 14+
	var exists bool
	err := db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_views WHERE viewname = 'pg_stat_wal')").Scan(&exists)
	if err != nil || !exists {
		return nil
	}

	err = db.QueryRowContext(ctx, "SELECT wal_bytes FROM pg_stat_wal").Scan(&s.WALBytes)
	if err != nil {
		return err
	}

	return c.repo.SaveWALRate(ctx, s)
}

func (c *PostgresBackupCollector) CollectBaseBackupHistory(ctx context.Context, db *sql.DB) error {
	var s entities.BaseBackupHistory
	s.TS = time.Now().UTC()
	s.InstanceID = c.instanceID

	var lastArchived sql.NullTime
	var writeTime sql.NullFloat64
	err := db.QueryRowContext(ctx, `
		SELECT last_archived_time, CAST(extract(epoch from (now() - last_archived_time)) AS DOUBLE PRECISION)
		FROM pg_stat_archiver`).Scan(&lastArchived, &writeTime)
	if err != nil {
		return nil
	}

	if lastArchived.Valid {
		s.CheckpointTime = &lastArchived.Time
	}
	if writeTime.Valid {
		s.CheckpointWriteTime = writeTime.Float64
	}

	return c.repo.SaveBaseBackupHistory(ctx, s)
}
