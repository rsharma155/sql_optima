// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Backup & DR data repositories.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repositories

import (
	"context"
	"database/sql"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/domain/entities"
)

type PostgresBackupRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresBackupRepository(pool *pgxpool.Pool) *PostgresBackupRepository {
	return &PostgresBackupRepository{pool: pool}
}

func (r *PostgresBackupRepository) SaveArchiverStats(ctx context.Context, s entities.BackupArchiverStats) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO monitor.pg_backup_archiver_ts (ts, instance_id, archived_count, failed_count, last_archived_time, last_failed_time)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		s.TS, s.InstanceID, s.ArchivedCount, s.FailedCount, s.LastArchivedTime, s.LastFailedTime)
	return err
}

func (r *PostgresBackupRepository) SaveWALRate(ctx context.Context, s entities.WALRate) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO monitor.pg_wal_rate_ts (ts, instance_id, wal_bytes)
		VALUES ($1, $2, $3)`,
		s.TS, s.InstanceID, s.WALBytes)
	return err
}

func (r *PostgresBackupRepository) SaveBaseBackupHistory(ctx context.Context, s entities.BaseBackupHistory) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO monitor.pg_basebackup_history (ts, instance_id, checkpoint_time, checkpoint_write_time)
		VALUES ($1, $2, $3, $4)`,
		s.TS, s.InstanceID, s.CheckpointTime, s.CheckpointWriteTime)
	return err
}

func (r *PostgresBackupRepository) GetKPIData(ctx context.Context, instance string, from, to string) (map[string]interface{}, error) {
	// Note: The JOIN might be tricky if timestamps don't align exactly. 
	// In production, we'd use separate queries or better aggregation.
	// For simplicity, let's use separate queries.
	
	var lastArchiveAge sql.NullString
	r.pool.QueryRow(ctx, "SELECT COALESCE((now() - max(last_archived_time))::text, 'N/A') FROM monitor.pg_backup_archiver_ts WHERE instance_id = $1", instance).Scan(&lastArchiveAge)
	
	var walBytes sql.NullFloat64
	r.pool.QueryRow(ctx, "SELECT COALESCE(sum(wal_bytes), 0) FROM monitor.pg_wal_rate_ts WHERE ts BETWEEN $1 AND $2 AND instance_id = $3", from, to, instance).Scan(&walBytes)
	
	var failedCount sql.NullInt64
	r.pool.QueryRow(ctx, "SELECT COALESCE(sum(failed_count), 0) FROM monitor.pg_backup_archiver_ts WHERE ts BETWEEN $1 AND $2 AND instance_id = $3", from, to, instance).Scan(&failedCount)
	
	var avgCheckpoint sql.NullFloat64
	r.pool.QueryRow(ctx, "SELECT COALESCE(avg(checkpoint_write_time), 0) FROM monitor.pg_basebackup_history WHERE ts BETWEEN $1 AND $2 AND instance_id = $3", from, to, instance).Scan(&avgCheckpoint)

	return map[string]interface{}{
		"last_archive_age":    lastArchiveAge.String,
		"wal_mb":              walBytes.Float64 / 1024.0 / 1024.0,
		"failed_count":        failedCount.Int64,
		"avg_checkpoint_time": avgCheckpoint.Float64,
	}, nil
}

func (r *PostgresBackupRepository) GetWALTrend(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT time_bucket('5 min', ts) AS bucket, sum(wal_bytes) as wal_bytes
		FROM monitor.pg_wal_rate_ts
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3
		GROUP BY bucket ORDER BY bucket ASC`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var bytes float64
		if err := rows.Scan(&bucket, &bytes); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"bucket":    bucket,
			"wal_bytes": bytes,
		})
	}
	return results, nil
}

func (r *PostgresBackupRepository) GetArchiveHealth(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT time_bucket('10 min', ts) AS bucket,
		       sum(archived_count) as archived,
		       sum(failed_count) as failed
		FROM monitor.pg_backup_archiver_ts
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3
		GROUP BY bucket ORDER BY bucket ASC`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var archived, failed float64
		if err := rows.Scan(&bucket, &archived, &failed); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"bucket":   bucket,
			"archived": archived,
			"failed":   failed,
		})
	}
	return results, nil
}

func (r *PostgresBackupRepository) GetFailedArchiveEvents(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT ts, failed_count, last_failed_time
		FROM monitor.pg_backup_archiver_ts
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3 AND failed_count > 0
		ORDER BY ts DESC LIMIT 50`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts, lastFailed time.Time
		var failed int64
		if err := rows.Scan(&ts, &failed, &lastFailed); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"ts":                ts,
			"failed_count":      failed,
			"last_failed_time": lastFailed,
		})
	}
	return results, nil
}

func (r *PostgresBackupRepository) GetCheckpointTrend(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT time_bucket('10 min', ts) AS bucket,
		       avg(checkpoint_write_time) as write_time
		FROM monitor.pg_basebackup_history
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3
		GROUP BY bucket ORDER BY bucket ASC`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var writeTime float64
		if err := rows.Scan(&bucket, &writeTime); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"bucket":     bucket,
			"write_time": writeTime,
		})
	}
	return results, nil
}

