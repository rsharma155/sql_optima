// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository for PostgreSQL Instance Health.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package instance_health

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type InstanceHealthRepository struct {
	pool *pgxpool.Pool
}

func NewInstanceHealthRepository(pool *pgxpool.Pool) *InstanceHealthRepository {
	return &InstanceHealthRepository{pool: pool}
}

func (r *InstanceHealthRepository) UpsertSnapshot(ctx context.Context, s *PgInstanceSnapshot) error {
	q := `
		INSERT INTO pg_instance_snapshot (
			server_id, capture_timestamp, tps, active_sessions, idle_sessions, 
			idle_in_tx_sessions, blocked_sessions, cpu_usage, shared_buffers_used_pct, 
			cache_hit_ratio, wal_mb_per_min, checkpoints_timed, checkpoints_req, 
			checkpoint_write_time, max_xid_age, oldest_tx_age_sec, database_size_gb, 
			temp_bytes_mb, autovacuum_workers, dead_tuple_pct, replica_lag_sec, 
			replication_slots, health_score, version, uptime, checkpoint_req_ratio
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)
		ON CONFLICT (server_id) DO UPDATE SET
			capture_timestamp = EXCLUDED.capture_timestamp,
			tps = EXCLUDED.tps,
			active_sessions = EXCLUDED.active_sessions,
			idle_sessions = EXCLUDED.idle_sessions,
			idle_in_tx_sessions = EXCLUDED.idle_in_tx_sessions,
			blocked_sessions = EXCLUDED.blocked_sessions,
			cpu_usage = EXCLUDED.cpu_usage,
			shared_buffers_used_pct = EXCLUDED.shared_buffers_used_pct,
			cache_hit_ratio = EXCLUDED.cache_hit_ratio,
			wal_mb_per_min = EXCLUDED.wal_mb_per_min,
			checkpoints_timed = EXCLUDED.checkpoints_timed,
			checkpoints_req = EXCLUDED.checkpoints_req,
			checkpoint_write_time = EXCLUDED.checkpoint_write_time,
			max_xid_age = EXCLUDED.max_xid_age,
			oldest_tx_age_sec = EXCLUDED.oldest_tx_age_sec,
			database_size_gb = EXCLUDED.database_size_gb,
			temp_bytes_mb = EXCLUDED.temp_bytes_mb,
			autovacuum_workers = EXCLUDED.autovacuum_workers,
			dead_tuple_pct = EXCLUDED.dead_tuple_pct,
			replica_lag_sec = EXCLUDED.replica_lag_sec,
			replication_slots = EXCLUDED.replication_slots,
			health_score = EXCLUDED.health_score,
			version = EXCLUDED.version,
			uptime = EXCLUDED.uptime,
			checkpoint_req_ratio = EXCLUDED.checkpoint_req_ratio
	`
	_, err := r.pool.Exec(ctx, q,
		s.ServerID, s.CollectedAt, s.TPS, s.ActiveSessions, s.IdleSessions,
		s.IdleInTxSessions, s.BlockedSessions, s.CPUUsage, s.SharedBuffersUsedPct,
		s.CacheHitRatio, s.WALMBPerMin, s.CheckpointsTimed, s.CheckpointsReq,
		s.CheckpointWriteTime, s.MaxXIDAge, s.OldestTxAgeSec, s.DatabaseSizeGB,
		s.TempBytesMB, s.AutovacuumWorkers, s.DeadTuplePct, s.ReplicaLagSec,
		s.ReplicationSlots, s.HealthScore, s.Version, s.Uptime, s.CheckpointReqRatio,
	)
	return err
}

func (r *InstanceHealthRepository) LogMetric(ctx context.Context, serverID uuid.UUID, metric string, value float64) error {
	q := `INSERT INTO pg_ts_metrics (capture_timestamp, server_id, metric, value) VALUES (now(), $1, $2, $3)`
	_, err := r.pool.Exec(ctx, q, serverID, metric, value)
	return err
}

func (r *InstanceHealthRepository) LogSnapshotMetrics(ctx context.Context, s *PgInstanceSnapshot) error {
	q := `INSERT INTO postgres_snapshot_metrics (
		capture_timestamp, server_id, tps, wal_mb_per_min, dead_tuple_pct,
		replica_lag_sec, cache_hit_ratio, checkpoint_req_ratio, database_size_gb,
		temp_bytes_mb, cpu_usage_pct, active_sessions, idle_sessions,
		idle_in_txn_sessions, waiting_sessions, health_score
	) VALUES (NOW(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`

	_, err := r.pool.Exec(ctx, q,
		s.ServerID, s.TPS, s.WALMBPerMin, s.DeadTuplePct,
		s.ReplicaLagSec, s.CacheHitRatio, s.CheckpointReqRatio, s.DatabaseSizeGB,
		s.TempBytesMB, s.CPUUsage, s.ActiveSessions, s.IdleSessions,
		s.IdleInTxSessions, s.BlockedSessions, s.HealthScore,
	)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "42P01" {
			// Table not yet created — schema migration pending; skip silently.
			return nil
		}
	}
	return err
}

type MetricDataPoint struct {
	Time   time.Time `json:"timestamp"`
	Metric string    `json:"metric"`
	Value  float64   `json:"value"`
}

func (r *InstanceHealthRepository) GetMetricHistory(ctx context.Context, serverID uuid.UUID, metric string, from, to time.Time, limit int) ([]MetricDataPoint, error) {
	var q string
	var args []interface{}
	args = append(args, serverID, metric)

	if !from.IsZero() && !to.IsZero() {
		q = `
			SELECT time_bucket('1 min', capture_timestamp) AS bucket, metric, avg(value) AS value
			FROM pg_ts_metrics
			WHERE server_id = $1 AND metric = $2
			  AND capture_timestamp >= $3 AND capture_timestamp <= $4
			GROUP BY bucket, metric
			ORDER BY bucket DESC
			LIMIT 2000
		`
		args = append(args, from, to)
	} else {
		q = `
			SELECT time_bucket('1 min', capture_timestamp) AS bucket, metric, avg(value) AS value
			FROM pg_ts_metrics
			WHERE server_id = $1 AND metric = $2
			GROUP BY bucket, metric
			ORDER BY bucket DESC
			LIMIT $3
		`
		args = append(args, limit)
	}

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []MetricDataPoint
	for rows.Next() {
		var dp MetricDataPoint
		if err := rows.Scan(&dp.Time, &dp.Metric, &dp.Value); err != nil {
			return nil, err
		}
		results = append(results, dp)
	}
	return results, nil
}

// SnapshotMetricPoint is a single time-bucketed row from postgres_snapshot_metrics.
type SnapshotMetricPoint struct {
	Bucket             time.Time `json:"timestamp"`
	TPS                float64   `json:"tps"`
	WALMBPerMin        float64   `json:"wal_mb_per_min"`
	DeadTuplePct       float64   `json:"dead_tuple_pct"`
	ReplicaLagSec      float64   `json:"replica_lag_sec"`
	CacheHitRatio      float64   `json:"cache_hit_ratio"`
	CheckpointReqRatio float64   `json:"checkpoint_req_ratio"`
	DatabaseSizeGB     float64   `json:"database_size_gb"`
	TempBytesMB        float64   `json:"temp_bytes_mb"`
	CPUUsagePct        float64   `json:"cpu_usage_pct"`
	ActiveSessions     float64   `json:"active_sessions"`
	HealthScore        float64   `json:"health_score"`
}

// GetSnapshotMetricsHistory returns time-bucketed history from the wide-row
// postgres_snapshot_metrics hypertable, replacing the legacy EAV pg_ts_metrics queries.
func (r *InstanceHealthRepository) GetSnapshotMetricsHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]SnapshotMetricPoint, error) {
	q := `
		SELECT
			time_bucket('1 min', capture_timestamp) AS bucket,
			avg(tps), avg(wal_mb_per_min), avg(dead_tuple_pct),
			avg(replica_lag_sec), avg(cache_hit_ratio), avg(checkpoint_req_ratio),
			avg(database_size_gb), avg(temp_bytes_mb), avg(cpu_usage_pct),
			avg(active_sessions), avg(health_score)
		FROM postgres_snapshot_metrics
		WHERE server_id = $1
		  AND capture_timestamp >= $2 AND capture_timestamp <= $3
		GROUP BY bucket
		ORDER BY bucket DESC
		LIMIT 2000
	`
	rows, err := r.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SnapshotMetricPoint
	for rows.Next() {
		var p SnapshotMetricPoint
		if err := rows.Scan(
			&p.Bucket, &p.TPS, &p.WALMBPerMin, &p.DeadTuplePct,
			&p.ReplicaLagSec, &p.CacheHitRatio, &p.CheckpointReqRatio,
			&p.DatabaseSizeGB, &p.TempBytesMB, &p.CPUUsagePct,
			&p.ActiveSessions, &p.HealthScore,
		); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

func (r *InstanceHealthRepository) GetLatestSnapshot(ctx context.Context, serverID uuid.UUID) (*PgInstanceSnapshot, error) {
	q := `SELECT server_id, capture_timestamp, tps, active_sessions, idle_sessions, 
	             idle_in_tx_sessions, blocked_sessions, cpu_usage, shared_buffers_used_pct, 
	             cache_hit_ratio, wal_mb_per_min, checkpoints_timed, checkpoints_req, 
	             checkpoint_write_time, max_xid_age, oldest_tx_age_sec, database_size_gb, 
	             temp_bytes_mb, autovacuum_workers, dead_tuple_pct, replica_lag_sec, 
	             replication_slots, health_score, version, uptime, checkpoint_req_ratio
	      FROM pg_instance_snapshot WHERE server_id = $1`
	var s PgInstanceSnapshot
	err := r.pool.QueryRow(ctx, q, serverID).Scan(
		&s.ServerID, &s.CollectedAt, &s.TPS, &s.ActiveSessions, &s.IdleSessions,
		&s.IdleInTxSessions, &s.BlockedSessions, &s.CPUUsage, &s.SharedBuffersUsedPct,
		&s.CacheHitRatio, &s.WALMBPerMin, &s.CheckpointsTimed, &s.CheckpointsReq,
		&s.CheckpointWriteTime, &s.MaxXIDAge, &s.OldestTxAgeSec, &s.DatabaseSizeGB,
		&s.TempBytesMB, &s.AutovacuumWorkers, &s.DeadTuplePct, &s.ReplicaLagSec,
		&s.ReplicationSlots, &s.HealthScore, &s.Version, &s.Uptime, &s.CheckpointReqRatio,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *InstanceHealthRepository) GetQueryStatsRange(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]hot.PostgresQueryStatsDelta, error) {
	q := `
		SELECT capture_timestamp, server_id, query_id, calls, total_exec_time
		FROM pgss_delta_1m
		WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3
		ORDER BY capture_timestamp ASC
	`
	rows, err := r.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []hot.PostgresQueryStatsDelta
	for rows.Next() {
		var d hot.PostgresQueryStatsDelta
		if err := rows.Scan(&d.CaptureTimestamp, &d.ServerID, &d.QueryID, &d.CallsDelta, &d.TotalTimeDeltaMs); err != nil {
			continue
		}
		results = append(results, d)
	}
	return results, rows.Err()
}
