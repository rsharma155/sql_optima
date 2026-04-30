// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Control Center metrics logger.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5"
)

type PostgresControlCenterRow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerInstanceName string    `json:"server_instance_name"`
	WALMBPerMin        float64   `json:"wal_mb_per_min"`
	WALSizeMB          float64   `json:"wal_size_mb"`
	ReplicaLagMB       float64   `json:"max_replication_lag_mb"`
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
}

type PostgresReplicationLagDetailRow struct {
	CaptureTimestamp   time.Time
	ServerInstanceName string
	ReplicaName        string
	LagMB              float64
	State              string
	SyncState          string
}

func pgControlCenterHash(r PostgresControlCenterRow) uint64 {
	h := fnv.New64a()
	// exclude timestamp
	_, _ = fmt.Fprintf(h, "%s|%g|%g|%g|%g|%g|%d|%g|%g|%d|%d|%d|%d|%d|%g|%d|%s",
		r.ServerInstanceName,
		r.WALMBPerMin,
		r.WALSizeMB,
		r.ReplicaLagMB,
		r.ReplicaLagSec,
		r.CheckpointReqRatio,
		r.XIDAge,
		r.XIDWraparoundPct,
		r.TPS,
		r.ActiveSessions,
		r.WaitingSessions,
		r.SlowQueriesCount,
		r.BlockingSessions,
		r.AutovacuumWorkers,
		r.DeadTuplePct,
		r.HealthScore,
		r.HealthStatus,
	)
	return h.Sum64()
}

// LogPostgresControlCenterStats inserts a new snapshot only when it differs from last snapshot for that instance.
func (tl *TimescaleLogger) LogPostgresControlCenterStats(ctx context.Context, row PostgresControlCenterRow) error {
	sig := pgControlCenterHash(row)
	tl.mu.Lock()
	if prev, ok := tl.prevPgControlCenterHash[row.ServerInstanceName]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgControlCenterHash[row.ServerInstanceName] = sig
	tl.mu.Unlock()

	q := `
		INSERT INTO postgres_control_center_stats (
			capture_timestamp, server_instance_name,
			wal_mb_per_min, wal_size_mb,
			max_replication_lag_mb, replica_lag_sec,
			checkpoint_req_ratio,
			xid_age, xid_wraparound_pct,
			tps, active_sessions, waiting_sessions, slow_queries_count,
			blocking_sessions, autovacuum_workers, dead_tuple_pct,
			health_score, health_status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
	`
	_, err := tl.pool.Exec(ctx, q,
		row.CaptureTimestamp, row.ServerInstanceName,
		row.WALMBPerMin, row.WALSizeMB,
		row.ReplicaLagMB, row.ReplicaLagSec,
		row.CheckpointReqRatio,
		row.XIDAge, row.XIDWraparoundPct,
		row.TPS, row.ActiveSessions, row.WaitingSessions, row.SlowQueriesCount,
		row.BlockingSessions, row.AutovacuumWorkers, row.DeadTuplePct,
		row.HealthScore, row.HealthStatus,
	)
	return err
}

func (tl *TimescaleLogger) GetLatestPostgresControlCenterStats(ctx context.Context, instanceName string) (*PostgresControlCenterRow, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := `
		SELECT capture_timestamp, server_instance_name,
		       wal_mb_per_min, wal_size_mb,
		       max_replication_lag_mb, replica_lag_sec,
		       checkpoint_req_ratio,
		       xid_age, xid_wraparound_pct,
		       tps, active_sessions, waiting_sessions, slow_queries_count,
		       blocking_sessions, autovacuum_workers, dead_tuple_pct,
		       health_score, COALESCE(health_status,'')
		FROM postgres_control_center_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`
	var r PostgresControlCenterRow
	err := tl.pool.QueryRow(ctx, q, instanceName).Scan(
		&r.CaptureTimestamp, &r.ServerInstanceName,
		&r.WALMBPerMin, &r.WALSizeMB,
		&r.ReplicaLagMB, &r.ReplicaLagSec,
		&r.CheckpointReqRatio,
		&r.XIDAge, &r.XIDWraparoundPct,
		&r.TPS, &r.ActiveSessions, &r.WaitingSessions, &r.SlowQueriesCount,
		&r.BlockingSessions, &r.AutovacuumWorkers, &r.DeadTuplePct,
		&r.HealthScore, &r.HealthStatus,
	)
	if err != nil {
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
			capture_timestamp, server_instance_name, replica_name,
			lag_mb, state, sync_state
		) VALUES ($1,$2,$3,$4,$5,$6)
	`
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, r.CaptureTimestamp, r.ServerInstanceName, r.ReplicaName, r.LagMB, r.State, r.SyncState)
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

type PostgresControlCenterHistory struct {
	Labels             []string  `json:"labels"`
	TPS                []float64 `json:"tps"`
	WALMBPerMin        []float64 `json:"wal_mb_per_min"`
	ReplLagSec         []float64 `json:"replica_lag_sec"`
	CheckpointReqRatio []float64 `json:"checkpoint_req_ratio"`
	Autovacuum         []int     `json:"autovacuum_workers"`
	DeadTuplePct       []float64 `json:"dead_tuple_pct"`
	BlockingSessions   []int     `json:"blocking_sessions"`
	HealthScore        []int     `json:"health_score"`
}

func (tl *TimescaleLogger) GetPostgresControlCenterHistory(ctx context.Context, instanceName string, from, to string, limit int) (*PostgresControlCenterHistory, error) {
	if limit <= 0 {
		limit = 180
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	where := `server_instance_name = $1`
	args := []interface{}{instanceName}
	if from != "" && to != "" {
		where += ` AND capture_timestamp >= $2 AND capture_timestamp <= $3`
		args = append(args, from, to)
	}

	query := fmt.Sprintf(`
		SELECT capture_timestamp,
		       tps,
		       wal_mb_per_min,
		       replica_lag_sec,
		       checkpoint_req_ratio,
		       autovacuum_workers,
		       dead_tuple_pct,
		       blocking_sessions,
		       health_score
		FROM postgres_control_center_stats
		WHERE %s
		ORDER BY capture_timestamp DESC
		LIMIT $%d
	`, where, len(args)+1)

	if from != "" && to != "" {
		query = fmt.Sprintf(`
			SELECT capture_timestamp,
				tps,
				wal_mb_per_min,
				replica_lag_sec,
				checkpoint_req_ratio,
				autovacuum_workers,
				dead_tuple_pct,
				blocking_sessions,
				health_score
			FROM postgres_control_center_stats
			WHERE %s
			ORDER BY capture_timestamp DESC
		`, where)
	} else {
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
	}
	var tmp []r0
	for rows.Next() {
		var r r0
		if err := rows.Scan(&r.ts, &r.tps, &r.wal, &r.lagS, &r.cp, &r.auto, &r.dead, &r.block, &r.score); err != nil {
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
	}
	return out, nil
}

type PostgresReplicationLagSeries struct {
	ReplicaName string    `json:"replica_name"`
	Labels      []string  `json:"labels"`
	LagMB       []float64 `json:"lag_mb"`
}

func (tl *TimescaleLogger) GetPostgresReplicationLagDetail(ctx context.Context, instanceName string, from, to string, limit int) (map[string]PostgresReplicationLagSeries, error) {
	if limit <= 0 {
		limit = 180
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	where := `server_instance_name = $1`
	args := []interface{}{instanceName}
	if from != "" && to != "" {
		where += ` AND capture_timestamp >= $2 AND capture_timestamp <= $3`
		args = append(args, from, to)
	}

	query := fmt.Sprintf(`
		SELECT capture_timestamp, replica_name, lag_mb
		FROM postgres_replication_lag_detail
		WHERE %s
		ORDER BY capture_timestamp DESC
		LIMIT $%d
	`, where, len(args)+1)

	if from != "" && to != "" {
		query = fmt.Sprintf(`
			SELECT capture_timestamp, replica_name, lag_mb
			FROM postgres_replication_lag_detail
			WHERE %s
			ORDER BY capture_timestamp DESC
		`, where)
	} else {
		args = append(args, limit)
	}

	rows, err := tl.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rr struct {
		ts   time.Time
		name string
		mb   float64
	}
	var tmp []rr
	for rows.Next() {
		var r rr
		if err := rows.Scan(&r.ts, &r.name, &r.mb); err != nil {
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
		s.Labels = append(s.Labels, r.ts.UTC().Format(time.RFC3339))
		s.LagMB = append(s.LagMB, r.mb)
		out[r.name] = s
	}
	return out, nil
}

// ComputeWalRateMBPerMin updates internal WAL bytes state and returns MB/min.
// ok=false on first observation (no prior baseline).
func (tl *TimescaleLogger) ComputeWalRateMBPerMin(instanceName string, walBytesTotal uint64, intervalSec float64) (rate float64, ok bool) {
	if intervalSec <= 0 {
		intervalSec = 60
	}
	tl.mu.Lock()
	defer tl.mu.Unlock()
	prev, seen := tl.prevPgWalBytesTotal[instanceName]
	tl.prevPgWalBytesTotal[instanceName] = walBytesTotal
	if !seen {
		return 0, false
	}
	deltaBytes := int64(walBytesTotal) - int64(prev)
	if deltaBytes < 0 {
		deltaBytes = 0
	}
	mb := float64(deltaBytes) / 1024.0 / 1024.0
	return mb * (60.0 / intervalSec), true
}

// ComputePgTps updates internal transaction state and returns average TPS for the interval.
func (tl *TimescaleLogger) ComputePgTps(instanceName string, xactTotal uint64, intervalSec float64) (tps float64, ok bool) {
	if intervalSec <= 0 {
		intervalSec = 60
	}
	tl.mu.Lock()
	defer tl.mu.Unlock()
	prev, seen := tl.prevPgXactTotal[instanceName]
	tl.prevPgXactTotal[instanceName] = xactTotal
	if !seen {
		return 0, false
	}
	delta := int64(xactTotal) - int64(prev)
	if delta < 0 {
		delta = 0
	}
	return float64(delta) / intervalSec, true
}
