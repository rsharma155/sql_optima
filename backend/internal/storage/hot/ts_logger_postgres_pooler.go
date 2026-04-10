package hot

import (
	"context"
	"fmt"
	"time"
)

type PostgresPoolerStatRow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerInstanceName string    `json:"server_instance_name"`
	PoolerType         string    `json:"pooler_type"`
	ClActive           int       `json:"cl_active"`
	ClWaiting          int       `json:"cl_waiting"`
	SvActive           int       `json:"sv_active"`
	SvIdle             int       `json:"sv_idle"`
	SvUsed             int       `json:"sv_used"`
	MaxwaitSeconds     float64   `json:"maxwait_seconds"`
	TotalPools         int       `json:"total_pools"`
}

func (tl *TimescaleLogger) LogPostgresPoolerStats(ctx context.Context, instanceName string, row PostgresPoolerStatRow) error {
	sig := pgFnv64(instanceName, row.PoolerType, row.ClActive, row.ClWaiting, row.SvActive, row.SvIdle, row.SvUsed, fmt.Sprintf("%.3f", row.MaxwaitSeconds), row.TotalPools)
	tl.mu.Lock()
	if tl.prevEnterpriseBatchHash == nil {
		tl.prevEnterpriseBatchHash = make(map[string]uint64)
	}
	key := "pg_pooler|" + instanceName
	if prev, ok := tl.prevEnterpriseBatchHash[key]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevEnterpriseBatchHash[key] = sig
	tl.mu.Unlock()

	q := `
		INSERT INTO postgres_pooler_stats (
			capture_timestamp, server_instance_name, pooler_type,
			cl_active, cl_waiting, sv_active, sv_idle, sv_used,
			maxwait_seconds, total_pools
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`
	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, q,
		now, instanceName, row.PoolerType,
		row.ClActive, row.ClWaiting, row.SvActive, row.SvIdle, row.SvUsed,
		row.MaxwaitSeconds, row.TotalPools,
	)
	return err
}

func (tl *TimescaleLogger) GetLatestPostgresPoolerStats(ctx context.Context, instanceName string) (*PostgresPoolerStatRow, error) {
	q := `
		SELECT capture_timestamp, server_instance_name, COALESCE(pooler_type,'pgbouncer'),
		       cl_active, cl_waiting, sv_active, sv_idle, sv_used,
		       maxwait_seconds, total_pools
		FROM postgres_pooler_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`
	var r PostgresPoolerStatRow
	if err := tl.pool.QueryRow(ctx, q, instanceName).Scan(
		&r.CaptureTimestamp, &r.ServerInstanceName, &r.PoolerType,
		&r.ClActive, &r.ClWaiting, &r.SvActive, &r.SvIdle, &r.SvUsed,
		&r.MaxwaitSeconds, &r.TotalPools,
	); err != nil {
		return nil, err
	}
	return &r, nil
}

func (tl *TimescaleLogger) GetPostgresPoolerStatsHistory(ctx context.Context, instanceName string, limit int) ([]PostgresPoolerStatRow, error) {
	if limit <= 0 {
		limit = 180
	}
	q := `
		SELECT capture_timestamp, server_instance_name, COALESCE(pooler_type,'pgbouncer'),
		       cl_active, cl_waiting, sv_active, sv_idle, sv_used,
		       maxwait_seconds, total_pools
		FROM postgres_pooler_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PostgresPoolerStatRow
	for rows.Next() {
		var r PostgresPoolerStatRow
		if err := rows.Scan(
			&r.CaptureTimestamp, &r.ServerInstanceName, &r.PoolerType,
			&r.ClActive, &r.ClWaiting, &r.SvActive, &r.SvIdle, &r.SvUsed,
			&r.MaxwaitSeconds, &r.TotalPools,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

