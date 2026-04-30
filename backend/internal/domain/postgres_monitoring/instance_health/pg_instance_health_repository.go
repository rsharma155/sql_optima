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

	"github.com/jackc/pgx/v5/pgxpool"
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
			instance_id, collected_at, tps, active_sessions, idle_sessions, 
			idle_in_tx_sessions, blocked_sessions, cpu_usage, shared_buffers_used_pct, 
			cache_hit_ratio, wal_mb_per_min, checkpoints_timed, checkpoints_req, 
			checkpoint_write_time, max_xid_age, oldest_tx_age_sec, database_size_gb, 
			temp_bytes_mb, autovacuum_workers, dead_tuple_pct, replica_lag_sec, 
			replication_slots, health_score, version, uptime, checkpoint_req_ratio
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)
		ON CONFLICT (instance_id) DO UPDATE SET
			collected_at = EXCLUDED.collected_at,
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
		s.InstanceID, s.CollectedAt, s.TPS, s.ActiveSessions, s.IdleSessions,
		s.IdleInTxSessions, s.BlockedSessions, s.CPUUsage, s.SharedBuffersUsedPct,
		s.CacheHitRatio, s.WALMBPerMin, s.CheckpointsTimed, s.CheckpointsReq,
		s.CheckpointWriteTime, s.MaxXIDAge, s.OldestTxAgeSec, s.DatabaseSizeGB,
		s.TempBytesMB, s.AutovacuumWorkers, s.DeadTuplePct, s.ReplicaLagSec,
		s.ReplicationSlots, s.HealthScore, s.Version, s.Uptime, s.CheckpointReqRatio,
	)
	return err
}

func (r *InstanceHealthRepository) LogMetric(ctx context.Context, instanceID string, metric string, value float64) error {
	q := `INSERT INTO pg_ts_metrics (time, instance_id, metric, value) VALUES (now(), $1, $2, $3)`
	_, err := r.pool.Exec(ctx, q, instanceID, metric, value)
	return err
}

type MetricDataPoint struct {
	Time   time.Time `json:"time"`
	Metric string    `json:"metric"`
	Value  float64   `json:"value"`
}

func (r *InstanceHealthRepository) GetMetricHistory(ctx context.Context, instanceID, metric string, from, to time.Time, limit int) ([]MetricDataPoint, error) {
	var q string
	var args []interface{}
	args = append(args, instanceID, metric)

	if !from.IsZero() && !to.IsZero() {
		q = `
			SELECT time_bucket('1 min', time) AS bucket, metric, avg(value) AS value
			FROM pg_ts_metrics
			WHERE instance_id = $1 AND metric = $2
			  AND time >= $3 AND time <= $4
			GROUP BY bucket, metric
			ORDER BY bucket DESC
			LIMIT 2000
		`
		args = append(args, from, to)
	} else {
		q = `
			SELECT time_bucket('1 min', time) AS bucket, metric, avg(value) AS value
			FROM pg_ts_metrics
			WHERE instance_id = $1 AND metric = $2
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

func (r *InstanceHealthRepository) GetLatestSnapshot(ctx context.Context, instanceID string) (*PgInstanceSnapshot, error) {
	q := `SELECT instance_id, collected_at, tps, active_sessions, idle_sessions, 
	             idle_in_tx_sessions, blocked_sessions, cpu_usage, shared_buffers_used_pct, 
	             cache_hit_ratio, wal_mb_per_min, checkpoints_timed, checkpoints_req, 
	             checkpoint_write_time, max_xid_age, oldest_tx_age_sec, database_size_gb, 
	             temp_bytes_mb, autovacuum_workers, dead_tuple_pct, replica_lag_sec, 
	             replication_slots, health_score, version, uptime, checkpoint_req_ratio
	      FROM pg_instance_snapshot WHERE instance_id = $1`
	var s PgInstanceSnapshot
	err := r.pool.QueryRow(ctx, q, instanceID).Scan(
		&s.InstanceID, &s.CollectedAt, &s.TPS, &s.ActiveSessions, &s.IdleSessions,
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
