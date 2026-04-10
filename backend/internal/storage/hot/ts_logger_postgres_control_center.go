package hot

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5"
)

type PostgresControlCenterRow struct {
	CaptureTimestamp        time.Time `json:"capture_timestamp"`
	ServerInstanceName      string    `json:"server_instance_name"`
	WALRateMBPerMin         float64   `json:"wal_rate_mb_per_min"`
	WALSizeMB               float64   `json:"wal_size_mb"`
	MaxReplicationLagMB     float64   `json:"max_replication_lag_mb"`
	MaxReplicationLagSecond float64   `json:"max_replication_lag_seconds"`
	CheckpointReqRatio      float64   `json:"checkpoint_req_ratio"`
	XIDAge                  int64     `json:"xid_age"`
	XIDWraparoundPct        float64   `json:"xid_wraparound_pct"`
	TPS                     float64   `json:"tps"`
	ActiveSessions          int       `json:"active_sessions"`
	WaitingSessions         int       `json:"waiting_sessions"`
	SlowQueriesCount        int       `json:"slow_queries_count"`
	BlockingSessions        int       `json:"blocking_sessions"`
	AutovacuumWorkers       int       `json:"autovacuum_workers"`
	DeadTupleRatioPct       float64   `json:"dead_tuple_ratio_pct"`
	HealthScore             int       `json:"health_score"`
	HealthStatus            string    `json:"health_status"`
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
		r.WALRateMBPerMin,
		r.WALSizeMB,
		r.MaxReplicationLagMB,
		r.MaxReplicationLagSecond,
		r.CheckpointReqRatio,
		r.XIDAge,
		r.XIDWraparoundPct,
		r.TPS,
		r.ActiveSessions,
		r.WaitingSessions,
		r.SlowQueriesCount,
		r.BlockingSessions,
		r.AutovacuumWorkers,
		r.DeadTupleRatioPct,
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
			wal_rate_mb_per_min, wal_size_mb,
			max_replication_lag_mb, max_replication_lag_seconds,
			checkpoint_req_ratio,
			xid_age, xid_wraparound_pct,
			tps, active_sessions, waiting_sessions, slow_queries_count,
			blocking_sessions, autovacuum_workers, dead_tuple_ratio_pct,
			health_score, health_status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
	`
	_, err := tl.pool.Exec(ctx, q,
		row.CaptureTimestamp, row.ServerInstanceName,
		row.WALRateMBPerMin, row.WALSizeMB,
		row.MaxReplicationLagMB, row.MaxReplicationLagSecond,
		row.CheckpointReqRatio,
		row.XIDAge, row.XIDWraparoundPct,
		row.TPS, row.ActiveSessions, row.WaitingSessions, row.SlowQueriesCount,
		row.BlockingSessions, row.AutovacuumWorkers, row.DeadTupleRatioPct,
		row.HealthScore, row.HealthStatus,
	)
	return err
}

func (tl *TimescaleLogger) GetLatestPostgresControlCenterStats(ctx context.Context, instanceName string) (*PostgresControlCenterRow, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := `
		SELECT capture_timestamp, server_instance_name,
		       wal_rate_mb_per_min, wal_size_mb,
		       max_replication_lag_mb, max_replication_lag_seconds,
		       checkpoint_req_ratio,
		       xid_age, xid_wraparound_pct,
		       tps, active_sessions, waiting_sessions, slow_queries_count,
		       blocking_sessions, autovacuum_workers, dead_tuple_ratio_pct,
		       health_score, COALESCE(health_status,'')
		FROM postgres_control_center_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`
	var r PostgresControlCenterRow
	err := tl.pool.QueryRow(ctx, q, instanceName).Scan(
		&r.CaptureTimestamp, &r.ServerInstanceName,
		&r.WALRateMBPerMin, &r.WALSizeMB,
		&r.MaxReplicationLagMB, &r.MaxReplicationLagSecond,
		&r.CheckpointReqRatio,
		&r.XIDAge, &r.XIDWraparoundPct,
		&r.TPS, &r.ActiveSessions, &r.WaitingSessions, &r.SlowQueriesCount,
		&r.BlockingSessions, &r.AutovacuumWorkers, &r.DeadTupleRatioPct,
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
	Labels           []string  `json:"labels"`
	WALRateMBPerMin  []float64 `json:"wal_rate_mb_per_min"`
	ReplLagSeconds   []float64 `json:"replication_lag_seconds"`
	CheckpointReqRatio []float64 `json:"checkpoint_req_ratio"`
	Autovacuum       []int     `json:"autovacuum_workers"`
	DeadTupleRatio   []float64 `json:"dead_tuple_ratio_pct"`
	BlockingSessions []int     `json:"blocking_sessions"`
	HealthScore      []int     `json:"health_score"`
}

func (tl *TimescaleLogger) GetPostgresControlCenterHistory(ctx context.Context, instanceName string, limit int) (*PostgresControlCenterHistory, error) {
	if limit <= 0 {
		limit = 60
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	q := `
		SELECT capture_timestamp,
		       wal_rate_mb_per_min,
		       max_replication_lag_seconds,
		       checkpoint_req_ratio,
		       autovacuum_workers,
		       dead_tuple_ratio_pct,
		       blocking_sessions,
		       health_score
		FROM postgres_control_center_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// reverse at end (desc -> asc)
	type r0 struct {
		ts    time.Time
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
		if err := rows.Scan(&r.ts, &r.wal, &r.lagS, &r.cp, &r.auto, &r.dead, &r.block, &r.score); err != nil {
			continue
		}
		tmp = append(tmp, r)
	}
	out := &PostgresControlCenterHistory{}
	for i := len(tmp) - 1; i >= 0; i-- {
		r := tmp[i]
		out.Labels = append(out.Labels, r.ts.Format("15:04"))
		out.WALRateMBPerMin = append(out.WALRateMBPerMin, r.wal)
		out.ReplLagSeconds = append(out.ReplLagSeconds, r.lagS)
		out.CheckpointReqRatio = append(out.CheckpointReqRatio, r.cp)
		out.Autovacuum = append(out.Autovacuum, r.auto)
		out.DeadTupleRatio = append(out.DeadTupleRatio, r.dead)
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

func (tl *TimescaleLogger) GetPostgresReplicationLagDetail(ctx context.Context, instanceName string, limit int) (map[string]PostgresReplicationLagSeries, error) {
	if limit <= 0 {
		limit = 60
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	q := `
		SELECT capture_timestamp, replica_name, lag_mb
		FROM postgres_replication_lag_detail
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
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
		s.Labels = append(s.Labels, r.ts.Format("15:04"))
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

