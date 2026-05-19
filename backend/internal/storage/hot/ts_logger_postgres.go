// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL metrics logger for TimescaleDB including throughput and connections.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func pgFnv64(parts ...any) uint64 {
	h := fnv.New64a()
	for i, p := range parts {
		if i > 0 {
			_, _ = h.Write([]byte("|"))
		}
		_, _ = fmt.Fprintf(h, "%v", p)
	}
	return h.Sum64()
}

func (tl *TimescaleLogger) logPostgresReplicationSlots(ctx context.Context, serverID uuid.UUID, rows []PostgresReplicationSlotRow, retried bool) error {
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].SlotName < rows[j].SlotName
	})
	sig := pgFnv64(serverID, len(rows))
	for _, r := range rows {
		sig = pgFnv64(sig,
			r.SlotName, r.SlotType, r.Active, r.Temporary,
			fmt.Sprintf("%.3f", r.RetainedWalMB),
			r.RestartLSN, r.ConfirmedFlushLSN,
			func() any {
				if r.Xmin == nil {
					return ""
				}
				return *r.Xmin
			}(),
			func() any {
				if r.CatalogXmin == nil {
					return ""
				}
				return *r.CatalogXmin
			}(),
		)
	}
	tl.mu.Lock()
	if prev, ok := tl.prevPgReplicationSlotsHash[serverID]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgReplicationSlotsHash[serverID] = sig
	tl.mu.Unlock()

	if len(rows) == 0 {
		return nil
	}

	q := `
		INSERT INTO postgres_replication_slot_stats (
			capture_timestamp, server_id,
			slot_name, slot_type, active, temporary,
			retained_wal_mb, restart_lsn, confirmed_flush_lsn,
			xmin_txid, catalog_xmin_txid
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`
	now := time.Now().UTC()
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q,
			now, serverID,
			r.SlotName, r.SlotType, r.Active, r.Temporary,
			r.RetainedWalMB, r.RestartLSN, r.ConfirmedFlushLSN,
			r.Xmin, r.CatalogXmin,
		)
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

func (tl *TimescaleLogger) LogPostgresReplicationSlots(ctx context.Context, serverID uuid.UUID, rows []PostgresReplicationSlotRow) error {
	return tl.logPostgresReplicationSlots(ctx, serverID, rows, false)
}

func (tl *TimescaleLogger) GetPostgresReplicationSlotHistory(ctx context.Context, serverID uuid.UUID, limit int) ([]PostgresReplicationSlotRow, error) {
	if limit <= 0 {
		limit = 200
	}
	q := `
		WITH latest AS (
			SELECT DISTINCT ON (slot_name)
			       capture_timestamp, server_id,
			       slot_name, COALESCE(slot_type,''), active, temporary,
			       retained_wal_mb, COALESCE(restart_lsn,''), COALESCE(confirmed_flush_lsn,''),
			       xmin_txid, catalog_xmin_txid
			FROM postgres_replication_slot_stats
			WHERE server_id = $1
			ORDER BY slot_name, capture_timestamp DESC
		)
		SELECT *
		FROM latest
		ORDER BY retained_wal_mb DESC, slot_name
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresReplicationSlotRow
	for rows.Next() {
		var r PostgresReplicationSlotRow
		if err := rows.Scan(
			&r.CaptureTimestamp, &r.ServerID,
			&r.SlotName, &r.SlotType, &r.Active, &r.Temporary,
			&r.RetainedWalMB, &r.RestartLSN, &r.ConfirmedFlushLSN,
			&r.Xmin, &r.CatalogXmin,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
