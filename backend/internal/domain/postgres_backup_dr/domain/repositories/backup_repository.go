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
	"fmt"
	"time"

	"github.com/google/uuid"
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
func (r *PostgresBackupRepository) CollectBackupDR(ctx context.Context, serverID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "SELECT monitor.pg_collect_backup_dr($1)", serverID)
	return err
}

// SaveBackupDR saves backup and DR metrics to TimescaleDB.
func (r *PostgresBackupRepository) SaveBackupDR(ctx context.Context, serverID uuid.UUID, data entities.PGBackupDRSnapshot) error {
	const q = `
		INSERT INTO snapshot.pg_backup_dr_timeseries (
			capture_timestamp, server_id, wal_bytes_total, wal_records_total, wal_fpi_total,
			archived_count, archive_failed_count, last_archived_time, last_failed_time,
			checkpoints_timed, checkpoints_req, checkpoint_write_time_ms, checkpoint_sync_time_ms,
			is_in_recovery
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`
	
	_, err := r.pool.Exec(ctx, q,
		data.CollectedAt, serverID, data.WALBytesTotal, data.WALRecordsTotal, data.WALFPITotal,
		data.ArchivedCount, data.ArchiveFailedCount, data.LastArchivedTime, data.LastFailedTime,
		data.CheckpointsTimed, data.CheckpointsReq, data.CheckpointWriteTimeMs, data.CheckpointSyncTimeMs,
		data.IsInRecovery,
	)
	return err
}

func (r *PostgresBackupRepository) GetKPIData(ctx context.Context, serverID uuid.UUID, from, to string) (map[string]interface{}, error) {
	var walRate float64
	err := r.pool.QueryRow(ctx, `
		SELECT
		  COALESCE((max(wal_bytes_total)-min(wal_bytes_total))/1024.0/1024.0, 0)
		FROM snapshot.pg_backup_dr_timeseries
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3`, serverID, from, to).Scan(&walRate)
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
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3`, serverID, from, to).Scan(&archiveSuccessRate)
	if err != nil {
		return nil, err
	}

	var maxReplicaLag float64
	_ = r.pool.QueryRow(ctx, `
		SELECT COALESCE(max(replay_lag_sec), 0)
		FROM postgres_replication_lag_detail
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3`, serverID, from, to).Scan(&maxReplicaLag)

	var slotsRiskGB float64
	_ = r.pool.QueryRow(ctx, `
		SELECT COALESCE(sum(lag_mb)/1024.0, 0)
		FROM postgres_replication_lag_detail
		WHERE server_id = $1 AND capture_timestamp = (SELECT max(capture_timestamp) FROM postgres_replication_lag_detail WHERE server_id = $1)`, serverID).Scan(&slotsRiskGB)

	var isInRecovery bool
	_ = r.pool.QueryRow(ctx, `
		SELECT COALESCE(is_in_recovery, false)
		FROM snapshot.pg_backup_dr_timeseries
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC LIMIT 1`, serverID).Scan(&isInRecovery)

	var lastArchiveAge string
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE((now() - max(last_archived_time))::text, 'N/A')
		FROM snapshot.pg_backup_dr_timeseries
		WHERE server_id = $1`, serverID).Scan(&lastArchiveAge)
	if err != nil {
		return nil, err
	}

	var avgCheckpointWrite float64
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(avg(checkpoint_write_time_ms), 0)
		FROM snapshot.pg_backup_dr_timeseries
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3`, serverID, from, to).Scan(&avgCheckpointWrite)
	if err != nil {
		return nil, err
	}

	var totalFailed int64
	_ = r.pool.QueryRow(ctx, `
		SELECT COALESCE(sum(archive_failed_count), 0)
		FROM snapshot.pg_backup_dr_timeseries
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3`, serverID, from, to).Scan(&totalFailed)

	nodeRole := "standalone"
	if maxReplicaLag > 0 {
		nodeRole = "primary"
	} else if isInRecovery {
		nodeRole = "replica"
	} else if !isInRecovery {
		nodeRole = "primary"
	}

	return map[string]interface{}{
		"wal_generation_rate_mb_min": walRate,
		"archive_success_percent":    archiveSuccessRate,
		"replica_max_lag_seconds":    maxReplicaLag,
		"replication_slots_risk_gb":  slotsRiskGB,
		"last_archive_age":           lastArchiveAge,
		"checkpoint_avg_write_time":  avgCheckpointWrite,
		"failed_count":               totalFailed,
		"is_in_recovery":             isInRecovery,
		"node_role":                  nodeRole,
	}, nil
}

func (r *PostgresBackupRepository) GetWALTrend(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT capture_timestamp,
		       wal_bytes_total - lag(wal_bytes_total) OVER (ORDER BY capture_timestamp) AS wal_bytes_delta
		FROM snapshot.pg_backup_dr_timeseries
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3
		ORDER BY capture_timestamp ASC`

	rows, err := r.pool.Query(ctx, query, serverID, from, to)
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
			"collected_at": t,
			"wal_bytes_delta": delta.Int64,
		})
	}
	return results, nil
}

func (r *PostgresBackupRepository) GetReplicationLagTrend(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT capture_timestamp, replica_name, replay_lag_sec
		FROM postgres_replication_lag_detail
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3
		ORDER BY capture_timestamp ASC`

	rows, err := r.pool.Query(ctx, query, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var t time.Time
		var replicaName string
		var lag float64
		if err := rows.Scan(&t, &replicaName, &lag); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"capture_timestamp": t,
			"application_name":  replicaName,
			"lag_seconds":       lag,
		})
	}
	return results, nil
}

func (r *PostgresBackupRepository) GetArchiveHealth(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT capture_timestamp,
		       archived_count,
		       archive_failed_count
		FROM snapshot.pg_backup_dr_timeseries
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3
		ORDER BY capture_timestamp ASC`

	rows, err := r.pool.Query(ctx, query, serverID, from, to)
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
			"collected_at": t,
			"archived_count": archived,
			"archive_failed_count": failed,
		})
	}
	return results, nil
}

func (r *PostgresBackupRepository) GetCheckpointTrend(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT capture_timestamp,
		       checkpoint_write_time_ms,
		       checkpoint_sync_time_ms
		FROM snapshot.pg_backup_dr_timeseries
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3
		ORDER BY capture_timestamp ASC`

	rows, err := r.pool.Query(ctx, query, serverID, from, to)
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

func (r *PostgresBackupRepository) GetReplicationDetails(ctx context.Context, serverID uuid.UUID) ([]map[string]interface{}, error) {
	query := `
		SELECT DISTINCT ON (replica_name)
		 replica_name,
		 COALESCE(state, 'unknown'),
		 COALESCE(sync_state, 'unknown'),
		 write_lag_sec,
		 flush_lag_sec,
		 replay_lag_sec,
		 lag_mb
		FROM postgres_replication_lag_detail
		WHERE server_id = $1
		ORDER BY replica_name, capture_timestamp DESC`

	rows, err := r.pool.Query(ctx, query, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var appName, state, syncState string
		var writeLag, flushLag, replayLag, lagMB float64
		if err := rows.Scan(&appName, &state, &syncState, &writeLag, &flushLag, &replayLag, &lagMB); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"application_name": appName,
			"state":            state,
			"sync_state":       syncState,
			"write_lag":        fmt.Sprintf("%.1fs", writeLag),
			"flush_lag":        fmt.Sprintf("%.1fs", flushLag),
			"replay_lag":       fmt.Sprintf("%.1fs", replayLag),
			"retained_mb":      lagMB,
		})
	}
	return results, nil
}

func (r *PostgresBackupRepository) GetArchiverFailures(ctx context.Context, serverID uuid.UUID) ([]map[string]interface{}, error) {
	query := `
		SELECT capture_timestamp, archive_failed_count, last_failed_time
		FROM snapshot.pg_backup_dr_timeseries
		WHERE server_id = $1 AND archive_failed_count > 0
		ORDER BY capture_timestamp DESC
		LIMIT 20`

	rows, err := r.pool.Query(ctx, query, serverID)
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
