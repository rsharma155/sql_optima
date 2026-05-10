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

// CollectBackupDR calls the database-side collector function.
func (r *PostgresBackupRepository) CollectBackupDR(ctx context.Context, instanceID string) error {
	_, err := r.pool.Exec(ctx, "SELECT monitor.pg_collect_backup_dr($1)", instanceID)
	return err
}

func (r *PostgresBackupRepository) GetKPIData(ctx context.Context, instance string, from, to string) (map[string]interface{}, error) {
	var walRate float64
	err := r.pool.QueryRow(ctx, `
		SELECT
		  COALESCE((max(wal_bytes_total)-min(wal_bytes_total))/1024.0/1024.0, 0)
		FROM snapshot.pg_backup_dr_timeseries
		WHERE instance_id = $1 AND collected_at BETWEEN $2 AND $3`, instance, from, to).Scan(&walRate)
	if err != nil {
		return nil, err
	}

	var archiveSuccessRate float64
	err = r.pool.QueryRow(ctx, `
		SELECT 
		  CASE WHEN (sum(archived_count) + sum(archive_failed_count)) > 0 
		       THEN (sum(archived_count)::float / (sum(archived_count) + sum(archive_failed_count))) * 100
		       ELSE 100 END
		FROM snapshot.pg_backup_dr_timeseries
		WHERE instance_id = $1 AND collected_at BETWEEN $2 AND $3`, instance, from, to).Scan(&archiveSuccessRate)
	if err != nil {
		return nil, err
	}

	var maxReplicaLag float64
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(max(extract(epoch from replay_lag)), 0)
		FROM snapshot.pg_replication_timeseries
		WHERE instance_id = $1 AND collected_at BETWEEN $2 AND $3`, instance, from, to).Scan(&maxReplicaLag)
	if err != nil {
		return nil, err
	}

	var slotsRiskGB float64
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(sum(retained_bytes)/1024.0/1024.0/1024.0, 0)
		FROM snapshot.pg_replication_timeseries
		WHERE instance_id = $1 AND collected_at = (SELECT max(collected_at) FROM snapshot.pg_replication_timeseries WHERE instance_id = $1)`, instance).Scan(&slotsRiskGB)
	if err != nil {
		return nil, err
	}

	var lastArchiveAge string
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE((now() - max(last_archived_time))::text, 'N/A')
		FROM snapshot.pg_backup_dr_timeseries
		WHERE instance_id = $1`, instance).Scan(&lastArchiveAge)
	if err != nil {
		return nil, err
	}

	var avgCheckpointWrite float64
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(avg(checkpoint_write_time_ms), 0)
		FROM snapshot.pg_backup_dr_timeseries
		WHERE instance_id = $1 AND collected_at BETWEEN $2 AND $3`, instance, from, to).Scan(&avgCheckpointWrite)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"wal_generation_rate_mb_min": walRate,
		"archive_success_percent":    archiveSuccessRate,
		"replica_max_lag_seconds":    maxReplicaLag,
		"replication_slots_risk_gb":  slotsRiskGB,
		"last_archive_age":           lastArchiveAge,
		"checkpoint_avg_write_time":  avgCheckpointWrite,
	}, nil
}

func (r *PostgresBackupRepository) GetWALTrend(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT collected_at,
		       wal_bytes_total - lag(wal_bytes_total) OVER (ORDER BY collected_at) AS wal_bytes_delta
		FROM snapshot.pg_backup_dr_timeseries
		WHERE instance_id = $1 AND collected_at BETWEEN $2 AND $3
		ORDER BY collected_at ASC`
	
	rows, err := r.pool.Query(ctx, query, instance, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var t time.Time
		var delta sql.NullInt64
		if err := rows.Scan(&t, &delta); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"collected_at":    t,
			"wal_bytes_delta": delta.Int64,
		})
	}
	return results, nil
}

func (r *PostgresBackupRepository) GetReplicationLagTrend(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT collected_at, application_name,
		       extract(epoch from replay_lag) AS lag_seconds
		FROM snapshot.pg_replication_timeseries
		WHERE instance_id = $1 AND collected_at BETWEEN $2 AND $3
		ORDER BY collected_at ASC`
	
	rows, err := r.pool.Query(ctx, query, instance, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var t time.Time
		var appName string
		var lag float64
		if err := rows.Scan(&t, &appName, &lag); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"collected_at":     t,
			"application_name": appName,
			"lag_seconds":      lag,
		})
	}
	return results, nil
}

func (r *PostgresBackupRepository) GetArchiveHealth(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT collected_at,
		       archived_count,
		       archive_failed_count
		FROM snapshot.pg_backup_dr_timeseries
		WHERE instance_id = $1 AND collected_at BETWEEN $2 AND $3
		ORDER BY collected_at ASC`
	
	rows, err := r.pool.Query(ctx, query, instance, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var t time.Time
		var archived, failed int64
		if err := rows.Scan(&t, &archived, &failed); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"collected_at":   t,
			"archived_count": archived,
			"failed_count":   failed,
		})
	}
	return results, nil
}

func (r *PostgresBackupRepository) GetCheckpointTrend(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT collected_at,
		       checkpoint_write_time_ms,
		       checkpoint_sync_time_ms
		FROM snapshot.pg_backup_dr_timeseries
		WHERE instance_id = $1 AND collected_at BETWEEN $2 AND $3
		ORDER BY collected_at ASC`
	
	rows, err := r.pool.Query(ctx, query, instance, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var t time.Time
		var write, sync float64
		if err := rows.Scan(&t, &write, &sync); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"collected_at":             t,
			"checkpoint_write_time_ms": write,
			"checkpoint_sync_time_ms":  sync,
		})
	}
	return results, nil
}

func (r *PostgresBackupRepository) GetReplicationDetails(ctx context.Context, instance string) ([]map[string]interface{}, error) {
	query := `
		SELECT DISTINCT ON (application_name)
		 application_name,
		 state,
		 sync_state,
		 write_lag::text,
		 flush_lag::text,
		 replay_lag::text,
		 retained_bytes/1024/1024 AS retained_mb
		FROM snapshot.pg_replication_timeseries
		WHERE instance_id = $1
		ORDER BY application_name, collected_at DESC`
	
	rows, err := r.pool.Query(ctx, query, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var appName, state, syncState, wLag, fLag, rLag string
		var retainedMB int64
		if err := rows.Scan(&appName, &state, &syncState, &wLag, &fLag, &rLag, &retainedMB); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"application_name": appName,
			"state":            state,
			"sync_state":       syncState,
			"write_lag":        wLag,
			"flush_lag":        fLag,
			"replay_lag":       rLag,
			"retained_mb":      retainedMB,
		})
	}
	return results, nil
}

func (r *PostgresBackupRepository) GetArchiverFailures(ctx context.Context, instance string) ([]map[string]interface{}, error) {
	query := `
		SELECT collected_at, archive_failed_count, last_failed_time
		FROM snapshot.pg_backup_dr_timeseries
		WHERE instance_id = $1 AND archive_failed_count > 0
		ORDER BY collected_at DESC
		LIMIT 20`
	
	rows, err := r.pool.Query(ctx, query, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var t time.Time
		var failedCount int64
		var lastFailed time.Time
		if err := rows.Scan(&t, &failedCount, &lastFailed); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"collected_at":         t,
			"archive_failed_count": failedCount,
			"last_failed_time":     lastFailed,
		})
	}
	return results, nil
}

// SaveArchiverStats is kept for compatibility during migration and currently performs no-op persistence.
func (r *PostgresBackupRepository) SaveArchiverStats(ctx context.Context, s entities.BackupArchiverStats) error {
	return nil
}

func (r *PostgresBackupRepository) SaveWALRate(ctx context.Context, s entities.WALRate) error {
	return nil
}

func (r *PostgresBackupRepository) SaveBaseBackupHistory(ctx context.Context, s entities.BaseBackupHistory) error {
	return nil
}
