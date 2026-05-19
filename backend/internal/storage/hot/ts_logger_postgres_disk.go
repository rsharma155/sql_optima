// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL host disk utilization logger.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PostgresDiskRow struct {
	MountName  string  `json:"mount_name"`
	Path       string  `json:"path"`
	TotalBytes int64   `json:"total_bytes"`
	FreeBytes  int64   `json:"free_bytes"`
	AvailBytes int64   `json:"avail_bytes"`
	UsedPct    float64 `json:"used_pct"`
}

type PostgresDiskStatResponse struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	MountName        string    `json:"mount_name"`
	Path             string    `json:"path"`
	TotalBytes       int64     `json:"total_bytes"`
	FreeBytes        int64     `json:"free_bytes"`
	AvailBytes       int64     `json:"avail_bytes"`
	UsedPct          float64   `json:"used_pct"`
}

func (tl *TimescaleLogger) LogPostgresDiskStats(ctx context.Context, serverID uuid.UUID, rows []PostgresDiskRow) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()

	sig := pgFnv64(serverID, len(rows))
	for _, r := range rows {
		sig = pgFnv64(sig, r.MountName, r.Path, r.TotalBytes, r.FreeBytes, r.AvailBytes, r.UsedPct)
	}

	key := "pg_disk|" + serverID.String()
	tl.mu.Lock()
	if prev, ok := tl.prevPgDbIOHash[serverID]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgDbIOHash[serverID] = sig
	tl.mu.Unlock()

	q := `
		INSERT INTO postgres_disk_stats (
			capture_timestamp, server_id, mount_name, path,
			total_bytes, free_bytes, avail_bytes, used_pct
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, now, serverID, r.MountName, r.Path, r.TotalBytes, r.FreeBytes, r.AvailBytes, r.UsedPct)
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	_ = key // key was used for a different hash map in older logic, keeping it for potential future composite keys
	return nil
}

func (tl *TimescaleLogger) GetPostgresDiskHistory(ctx context.Context, serverID uuid.UUID, limit int) ([]PostgresDiskStatResponse, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `
		SELECT capture_timestamp, server_id, mount_name, path,
		       total_bytes, free_bytes, avail_bytes, used_pct
		FROM postgres_disk_stats
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresDiskStatResponse
	for rows.Next() {
		var r PostgresDiskStatResponse
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerID, &r.MountName, &r.Path,
			&r.TotalBytes, &r.FreeBytes, &r.AvailBytes, &r.UsedPct); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
