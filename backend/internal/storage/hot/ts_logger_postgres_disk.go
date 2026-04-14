// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL disk usage and disk I/O logger.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type PostgresDiskStatRow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerInstanceName string    `json:"server_instance_name"`
	MountName          string    `json:"mount_name"`
	Path              string    `json:"path"`
	TotalBytes         int64     `json:"total_bytes"`
	FreeBytes          int64     `json:"free_bytes"`
	AvailBytes         int64     `json:"avail_bytes"`
	UsedPct            float64   `json:"used_pct"`
}

func (tl *TimescaleLogger) LogPostgresDiskStats(ctx context.Context, instanceName string, rows []PostgresDiskStatRow) error {
	if len(rows) == 0 {
		return nil
	}

	// Dedup per instance+mount set
	sig := pgFnv64(instanceName, len(rows))
	for _, r := range rows {
		sig = pgFnv64(sig, r.MountName, r.Path, r.TotalBytes, r.FreeBytes, r.AvailBytes, fmt.Sprintf("%.2f", r.UsedPct))
	}
	tl.mu.Lock()
	if tl.prevEnterpriseBatchHash == nil {
		tl.prevEnterpriseBatchHash = make(map[string]uint64)
	}
	key := "pg_disk|" + instanceName
	if prev, ok := tl.prevEnterpriseBatchHash[key]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevEnterpriseBatchHash[key] = sig
	tl.mu.Unlock()

	q := `
		INSERT INTO postgres_disk_stats (
			capture_timestamp, server_instance_name, mount_name, path,
			total_bytes, free_bytes, avail_bytes, used_pct
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`
	now := time.Now().UTC()
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, now, instanceName, r.MountName, r.Path, r.TotalBytes, r.FreeBytes, r.AvailBytes, r.UsedPct)
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

func (tl *TimescaleLogger) GetPostgresDiskStats(ctx context.Context, instanceName string, limit int) ([]PostgresDiskStatRow, error) {
	if limit <= 0 {
		limit = 200
	}
	q := `
		SELECT capture_timestamp, server_instance_name, mount_name, path,
		       total_bytes, free_bytes, avail_bytes, used_pct
		FROM postgres_disk_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC, mount_name
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresDiskStatRow
	for rows.Next() {
		var r PostgresDiskStatRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerInstanceName, &r.MountName, &r.Path, &r.TotalBytes, &r.FreeBytes, &r.AvailBytes, &r.UsedPct); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

