// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL replication slot management and monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

type PgReplicationSlotStat struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	SlotName         string    `json:"slot_name"`
	SlotType         string    `json:"slot_type"`
	Active           bool      `json:"active"`
	Temporary        bool      `json:"temporary"`
	RetainedWalMB    float64   `json:"retained_wal_mb"`
	RestartLSN       string    `json:"restart_lsn"`
	ConfirmedFlushLSN string   `json:"confirmed_flush_lsn"`
	Xmin             *int64    `json:"xmin,omitempty"`
	CatalogXmin      *int64    `json:"catalog_xmin,omitempty"`
}
// GetReplicationSlotStats returns replication slot stats for an instance.
// It uses pg_wal_lsn_diff to estimate WAL retention for the slot.
func (c *PgRepository) GetReplicationSlotStats(instanceName string) ([]PgReplicationSlotStat, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// retained bytes:
	// - physical slot: current_wal_lsn - restart_lsn
	// - logical slot: current_wal_lsn - confirmed_flush_lsn (if present), else restart_lsn
	query := `
		SELECT
			now() AT TIME ZONE 'UTC' AS capture_timestamp,
			slot_name,
			slot_type,
			active,
			temporary,
			COALESCE(
				pg_wal_lsn_diff(pg_current_wal_lsn(), COALESCE(NULLIF(confirmed_flush_lsn::text,''), restart_lsn::text)::pg_lsn),
				0
			) AS retained_bytes,
			COALESCE(restart_lsn::text,'') AS restart_lsn,
			COALESCE(confirmed_flush_lsn::text,'') AS confirmed_flush_lsn,
			xmin,
			catalog_xmin
		FROM pg_replication_slots
		ORDER BY retained_bytes DESC, slot_name
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[POSTGRES] GetReplicationSlotStats query error for %s: %v", instanceName, err)
		return nil, err
	}
	defer rows.Close()

	var out []PgReplicationSlotStat
	for rows.Next() {
		var r PgReplicationSlotStat
		var retainedBytes sql.NullInt64
		var restartLSN, confirmedLSN sql.NullString
		var xmin, catalogXmin sql.NullInt64
		if err := rows.Scan(
			&r.CaptureTimestamp,
			&r.SlotName,
			&r.SlotType,
			&r.Active,
			&r.Temporary,
			&retainedBytes,
			&restartLSN,
			&confirmedLSN,
			&xmin,
			&catalogXmin,
		); err != nil {
			continue
		}
		if retainedBytes.Valid {
			r.RetainedWalMB = retainedWalMBFromBytes(retainedBytes.Int64)
		}
		if restartLSN.Valid {
			r.RestartLSN = restartLSN.String
		}
		if confirmedLSN.Valid {
			r.ConfirmedFlushLSN = confirmedLSN.String
		}
		if xmin.Valid {
			v := xmin.Int64
			r.Xmin = &v
		}
		if catalogXmin.Valid {
			v := catalogXmin.Int64
			r.CatalogXmin = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

