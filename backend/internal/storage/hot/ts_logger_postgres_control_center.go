/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: TimescaleDB storage for PostgreSQL Control Center metrics.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */
package hot

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/cespare/xxhash/v2"
)

type PostgresReplicationLagDetailRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	ReplicaName      string    `json:"replica_name"`
	LagMB            float64   `json:"lag_mb"`
	State            string    `json:"state"`
	SyncState        string    `json:"sync_state"`
	WriteLagSec      float64   `json:"write_lag_sec"`
	FlushLagSec      float64   `json:"flush_lag_sec"`
	ReplayLagSec     float64   `json:"replay_lag_sec"`
}

type PostgresControlCenterRow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerID           uuid.UUID `json:"server_id"`
	WALMBPerMin        float64   `json:"wal_mb_per_min"`
	WALSizeMB          float64   `json:"wal_size_mb"`
	ReplicaLagMB       float64   `json:"replica_lag_mb"`
	ReplicaLagSec      float64   `json:"replica_lag_sec"`
	CheckpointReqRatio float64   `json:"checkpoint_req_ratio"`
	XIDAge             int64     `json:"xid_age"`
	XIDWraparoundPct   float64   `json:"xid_wraparound_pct"`
	TPS                float64   `json:"tps"`
	ActiveSessions     int       `json:"active_sessions"`
	WaitingSessions    int       `json:"waiting_sessions"`
	SlowQueriesCount   int       `json:"slow_queries_count"`
	BlockingSessions   int       `json:"blocking_sessions"`
	AutovacuumWorkers  int       `json:"autovacuum_workers"`
	DeadTuplePct       float64   `json:"dead_tuple_pct"`
	HealthScore        int       `json:"health_score"`
	HealthStatus       string    `json:"health_status"`
	IdleSessions       int       `json:"idle_sessions"`
	IdleInTxnSessions  int       `json:"idle_in_txn_sessions"`
	ConnectionsMax     int       `json:"connections_max"`
	ConnectionsUsed    int       `json:"connections_used"`
	ConnectionsUsagePct float64  `json:"connections_usage_pct"`
	CacheHitRatioPct   float64   `json:"cache_hit_ratio_pct"`
	DeadlocksPerMin    float64   `json:"deadlocks_per_min"`
}

func pgControlCenterHash(r PostgresControlCenterRow) uint64 {
	h := xxhash.New()
	fmt.Fprintf(h, "%v|%f|%f|%f|%f|%f|%d|%f|%f|%d|%d|%d|%d|%d|%f|%d|%s|%d|%d|%d|%d|%f|%f|%f",
		r.ServerID, r.WALMBPerMin, r.WALSizeMB, r.ReplicaLagMB, r.ReplicaLagSec,
		r.CheckpointReqRatio, r.XIDAge, r.XIDWraparoundPct,
		r.TPS, r.ActiveSessions, r.WaitingSessions, r.SlowQueriesCount,
		r.BlockingSessions, r.AutovacuumWorkers, r.DeadTuplePct,
		r.HealthScore, r.HealthStatus,
		r.IdleSessions, r.IdleInTxnSessions, r.ConnectionsMax,
		r.ConnectionsUsed, r.ConnectionsUsagePct, r.CacheHitRatioPct,
		r.DeadlocksPerMin,
	)
	return h.Sum64()
}

// LogPostgresControlCenterStats inserts a new snapshot only when it differs from last snapshot for that serverID.
func (tl *TimescaleLogger) LogPostgresControlCenterStats(ctx context.Context, row PostgresControlCenterRow) error {
	sig := pgControlCenterHash(row)
	tl.mu.Lock()
	if prev, ok := tl.prevPgControlCenterHash[row.ServerID]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgControlCenterHash[row.ServerID] = sig
	tl.mu.Unlock()

	q := `
		INSERT INTO postgres_control_center_stats (
			capture_timestamp, server_id,
			wal_mb_per_min, wal_size_mb,
			max_replication_lag_mb, replica_lag_sec,
			checkpoint_req_ratio,
			xid_age, xid_wraparound_pct,
			tps, active_sessions, waiting_sessions, slow_queries_count,
			blocking_sessions, autovacuum_workers, dead_tuple_pct,
			health_score, health_status,
			idle_sessions, idle_in_txn_sessions, connections_max,
			connections_used, connections_usage_pct, cache_hit_ratio_pct,
			deadlocks_per_min
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
	`
	_, err := tl.pool.Exec(ctx, q,
		row.CaptureTimestamp, row.ServerID,
		row.WALMBPerMin, row.WALSizeMB,
		row.ReplicaLagMB, row.ReplicaLagSec,
		row.CheckpointReqRatio,
		row.XIDAge, row.XIDWraparoundPct,
		row.TPS, row.ActiveSessions, row.WaitingSessions, row.SlowQueriesCount,
		row.BlockingSessions, row.AutovacuumWorkers, row.DeadTuplePct,
		row.HealthScore, row.HealthStatus,
		row.IdleSessions, row.IdleInTxnSessions, row.ConnectionsMax,
		row.ConnectionsUsed, row.ConnectionsUsagePct, row.CacheHitRatioPct,
		row.DeadlocksPerMin,
	)
	return err
}

func (tl *TimescaleLogger) GetLatestPostgresControlCenterStats(ctx context.Context, serverID uuid.UUID) (*PostgresControlCenterRow, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := `
		SELECT capture_timestamp, server_id,
		       wal_mb_per_min, wal_size_mb,
		       max_replication_lag_mb, replica_lag_sec,
		       checkpoint_req_ratio,
		       xid_age, xid_wraparound_pct,
		       tps, active_sessions, waiting_sessions, slow_queries_count,
		       blocking_sessions, autovacuum_workers, dead_tuple_pct,
		       health_score, COALESCE(health_status,''),
		       COALESCE(idle_sessions, 0), COALESCE(idle_in_txn_sessions, 0), COALESCE(connections_max, 0),
		       COALESCE(connections_used, 0), COALESCE(connections_usage_pct, 0), COALESCE(cache_hit_ratio_pct, 0),
		       COALESCE(deadlocks_per_min, 0)
		FROM postgres_control_center_stats
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`
	var r PostgresControlCenterRow
	err := tl.pool.QueryRow(ctx, q, serverID).Scan(
		&r.CaptureTimestamp, &r.ServerID,
		&r.WALMBPerMin, &r.WALSizeMB,
		&r.ReplicaLagMB, &r.ReplicaLagSec,
		&r.CheckpointReqRatio,
		&r.XIDAge, &r.XIDWraparoundPct,
		&r.TPS, &r.ActiveSessions, &r.WaitingSessions, &r.SlowQueriesCount,
		&r.BlockingSessions, &r.AutovacuumWorkers, &r.DeadTuplePct,
		&r.HealthScore, &r.HealthStatus,
		&r.IdleSessions, &r.IdleInTxnSessions, &r.ConnectionsMax,
		&r.ConnectionsUsed, &r.ConnectionsUsagePct, &r.CacheHitRatioPct,
		&r.DeadlocksPerMin,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (tl *TimescaleLogger) LogPostgresReplicationLagDetail(ctx context.Context, rows []PostgresReplicationLagDetailRow) error {
	if len(rows) == 0 {
		return nil
	}
	q := `
		INSERT INTO postgres_replication_lag_detail (
			capture_timestamp, server_id, replica_name,
			lag_mb, state, sync_state,
			write_lag_sec, flush_lag_sec, replay_lag_sec
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, r.CaptureTimestamp, r.ServerID, r.ReplicaName, r.LagMB, r.State, r.SyncState,
			r.WriteLagSec, r.FlushLagSec, r.ReplayLagSec)
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// ComputePgTps calculates Transactions Per Second based on the delta of xact_total.
func (tl *TimescaleLogger) ComputePgTps(serverID uuid.UUID, xactTotal uint64, intervalSeconds float64) (float64, bool) {
	if intervalSeconds <= 0 {
		intervalSeconds = 15.0
	}
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if tl.prevPgXactTotal == nil {
		tl.prevPgXactTotal = make(map[uuid.UUID]uint64)
	}

	prev, ok := tl.prevPgXactTotal[serverID]
	tl.prevPgXactTotal[serverID] = xactTotal

	if !ok {
		return 0, false
	}

	if xactTotal < prev {
		// Counter reset
		return 0, true
	}

	delta := float64(xactTotal - prev)
	return delta / intervalSeconds, true
}

// ComputeWalRateMBPerMin calculates WAL generation rate in MB/minute based on pg_wal_bytes_total delta.
func (tl *TimescaleLogger) ComputeWalRateMBPerMin(serverID uuid.UUID, walBytesTotal uint64, intervalSeconds float64) (float64, bool) {
	if intervalSeconds <= 0 {
		intervalSeconds = 15.0
	}
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if tl.prevPgWalBytesTotal == nil {
		tl.prevPgWalBytesTotal = make(map[uuid.UUID]uint64)
	}

	prev, ok := tl.prevPgWalBytesTotal[serverID]
	tl.prevPgWalBytesTotal[serverID] = walBytesTotal

	if !ok {
		return 0, false
	}

	if walBytesTotal < prev {
		// Counter reset or overflow
		return 0, true
	}

	deltaBytes := float64(walBytesTotal - prev)
	deltaMB := deltaBytes / (1024 * 1024)
	rateMBPerMin := deltaMB / (intervalSeconds / 60.0)

	return rateMBPerMin, true
}

type PostgresControlCenterHistory struct {
	Labels              []string  `json:"labels"`
	TPS                 []float64 `json:"tps"`
	WALMBPerMin         []float64 `json:"wal_mb_per_min"`
	ReplLagSec          []float64 `json:"replica_lag_sec"`
	CheckpointReqRatio  []float64 `json:"checkpoint_req_ratio"`
	Autovacuum          []int     `json:"autovacuum_workers"`
	DeadTuplePct        []float64 `json:"dead_tuple_pct"`
	BlockingSessions    []int     `json:"blocking_sessions"`
	HealthScore         []int     `json:"health_score"`
	CacheHitRatioPct    []float64 `json:"cache_hit_ratio_pct"`
	ConnectionsUsagePct []float64 `json:"connections_usage_pct"`
}

func (tl *TimescaleLogger) GetPostgresControlCenterHistory(ctx context.Context, serverID uuid.UUID, from, to string, limit int) (*PostgresControlCenterHistory, error) {
	if limit <= 0 {
		limit = 180
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	where := `server_id = $1`
	args := []interface{}{serverID}
	if from != "" && to != "" {
		where += ` AND capture_timestamp >= $2 AND capture_timestamp <= $3`
		args = append(args, from, to)
	}

	const histSelect = `
		SELECT capture_timestamp,
		       COALESCE(tps, 0),
		       COALESCE(wal_mb_per_min, 0),
		       COALESCE(replica_lag_sec, 0),
		       COALESCE(checkpoint_req_ratio, 0),
		       COALESCE(autovacuum_workers, 0),
		       COALESCE(dead_tuple_pct, 0),
		       COALESCE(blocking_sessions, 0),
		       COALESCE(health_score, 0),
		       COALESCE(cache_hit_ratio_pct, 0),
		       COALESCE(connections_usage_pct, 0)
		FROM postgres_control_center_stats
		WHERE %s
		ORDER BY capture_timestamp DESC
	`
	var query string
	if from != "" && to != "" {
		query = fmt.Sprintf(histSelect, where)
	} else {
		query = fmt.Sprintf(histSelect+` LIMIT $%d`, where, len(args)+1)
		args = append(args, limit)
	}

	rows, err := tl.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// reverse at end (desc -> asc)
	type r0 struct {
		ts    time.Time
		tps   float64
		wal   float64
		lagS  float64
		cp    float64
		auto  int
		dead  float64
		block int
		score int
		cache float64
		conn  float64
	}
	var tmp []r0
	for rows.Next() {
		var r r0
		if err := rows.Scan(&r.ts, &r.tps, &r.wal, &r.lagS, &r.cp, &r.auto, &r.dead, &r.block, &r.score, &r.cache, &r.conn); err != nil {
			continue
		}
		tmp = append(tmp, r)
	}
	out := &PostgresControlCenterHistory{}
	for i := len(tmp) - 1; i >= 0; i-- {
		r := tmp[i]
		out.Labels = append(out.Labels, r.ts.UTC().Format(time.RFC3339))
		out.TPS = append(out.TPS, r.tps)
		out.WALMBPerMin = append(out.WALMBPerMin, r.wal)
		out.ReplLagSec = append(out.ReplLagSec, r.lagS)
		out.CheckpointReqRatio = append(out.CheckpointReqRatio, r.cp)
		out.Autovacuum = append(out.Autovacuum, r.auto)
		out.DeadTuplePct = append(out.DeadTuplePct, r.dead)
		out.BlockingSessions = append(out.BlockingSessions, r.block)
		out.HealthScore = append(out.HealthScore, r.score)
		out.CacheHitRatioPct = append(out.CacheHitRatioPct, r.cache)
		out.ConnectionsUsagePct = append(out.ConnectionsUsagePct, r.conn)
	}
	return out, nil
}

type PostgresReplicationLagSeries struct {
	ReplicaName   string    `json:"replica_name"`
	Labels        []string  `json:"labels"`
	LagMB         []float64 `json:"lag_mb"`
	WriteLagSec   []float64 `json:"write_lag_sec"`
	FlushLagSec   []float64 `json:"flush_lag_sec"`
	ReplayLagSec  []float64 `json:"replay_lag_sec"`
	State         string    `json:"state"`
	SyncState     string    `json:"sync_state"`
}

func (tl *TimescaleLogger) GetPostgresReplicationLagHistory(ctx context.Context, serverID uuid.UUID, from, to string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 180
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	where := `server_id = $1`
	args := []interface{}{serverID}
	if from != "" && to != "" {
		where += ` AND capture_timestamp >= $2 AND capture_timestamp <= $3`
		args = append(args, from, to)
	}

	query := fmt.Sprintf(`
		SELECT capture_timestamp as time, MAX(lag_mb) as value
		FROM postgres_replication_lag_detail
		WHERE %s
		GROUP BY capture_timestamp
		ORDER BY capture_timestamp DESC
		LIMIT $%d
	`, where, len(args)+1)
	args = append(args, limit)

	rows, err := tl.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var val float64
		if err := rows.Scan(&ts, &val); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"timestamp": ts.UTC().Format(time.RFC3339),
			"value":     val,
		})
	}

	// Reverse to ascending for Chart.js
	for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
		results[i], results[j] = results[j], results[i]
	}

	return results, nil
}

func (tl *TimescaleLogger) GetPostgresReplicationLagDetail(ctx context.Context, serverID uuid.UUID, from, to string, limit int) (map[string]PostgresReplicationLagSeries, error) {
	if limit <= 0 {
		limit = 180
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	where := `server_id = $1`
	args := []interface{}{serverID}
	if from != "" && to != "" {
		where += ` AND capture_timestamp >= $2 AND capture_timestamp <= $3`
		args = append(args, from, to)
	}

	baseQuery := `
		SELECT capture_timestamp, replica_name, lag_mb,
		       COALESCE(write_lag_sec, 0), COALESCE(flush_lag_sec, 0), COALESCE(replay_lag_sec, 0),
		       COALESCE(state, ''), COALESCE(sync_state, '')
		FROM postgres_replication_lag_detail
		WHERE %s
		ORDER BY capture_timestamp DESC
	`
	var query string
	if from == "" || to == "" {
		query = fmt.Sprintf(baseQuery+` LIMIT $%d`, where, len(args)+1)
		args = append(args, limit)
	} else {
		query = fmt.Sprintf(baseQuery, where)
	}

	rows, err := tl.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rr struct {
		ts        time.Time
		name      string
		mb        float64
		writeSec  float64
		flushSec  float64
		replaySec float64
		state     string
		syncState string
	}
	var tmp []rr
	for rows.Next() {
		var r rr
		if err := rows.Scan(&r.ts, &r.name, &r.mb, &r.writeSec, &r.flushSec, &r.replaySec, &r.state, &r.syncState); err != nil {
			continue
		}
		tmp = append(tmp, r)
	}
	// Build in ascending time
	out := make(map[string]PostgresReplicationLagSeries)
	for i := len(tmp) - 1; i >= 0; i-- {
		r := tmp[i]
		s := out[r.name]
		s.ReplicaName = r.name
		s.State = r.state
		s.SyncState = r.syncState
		s.Labels = append(s.Labels, r.ts.UTC().Format(time.RFC3339))
		s.LagMB = append(s.LagMB, r.mb)
		s.WriteLagSec = append(s.WriteLagSec, r.writeSec)
		s.FlushLagSec = append(s.FlushLagSec, r.flushSec)
		s.ReplayLagSec = append(s.ReplayLagSec, r.replaySec)
		out[r.name] = s
	}
	return out, nil
}
