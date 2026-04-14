// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Vacuum progress tracking with live vacuum operation status.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
	"log"
	"time"
)

type PgVacuumProgressRow struct {
	CaptureTimestamp  time.Time `json:"capture_timestamp"`
	PID               int64     `json:"pid"`
	DatabaseName      string    `json:"database_name,omitempty"`
	UserName          string    `json:"user_name,omitempty"`
	RelationName      string    `json:"relation_name,omitempty"`
	Phase             string    `json:"phase,omitempty"`
	HeapBlksTotal     int64     `json:"heap_blks_total"`
	HeapBlksScanned   int64     `json:"heap_blks_scanned"`
	HeapBlksVacuumed  int64     `json:"heap_blks_vacuumed"`
	IndexVacuumCount  int64     `json:"index_vacuum_count"`
	MaxDeadTuples     int64     `json:"max_dead_tuples"`
	NumDeadTuples     int64     `json:"num_dead_tuples"`
	ProgressPct       float64   `json:"progress_pct"`
}

func vacuumProgressPct(total, scanned int64) float64 {
	if total <= 0 || scanned <= 0 {
		return 0
	}
	p := (float64(scanned) / float64(total)) * 100.0
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func (c *PgRepository) GetVacuumProgress(instanceName string) ([]PgVacuumProgressRow, error) {
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

	// pg_stat_progress_vacuum requires privileges; use left joins for names.
	q := `
		SELECT
			now() AT TIME ZONE 'UTC' AS capture_timestamp,
			p.pid,
			COALESCE(a.datname,'') AS database_name,
			COALESCE(a.usename,'') AS user_name,
			COALESCE(p.relid::regclass::text,'') AS relation_name,
			COALESCE(p.phase,'') AS phase,
			COALESCE(p.heap_blks_total,0) AS heap_blks_total,
			COALESCE(p.heap_blks_scanned,0) AS heap_blks_scanned,
			COALESCE(p.heap_blks_vacuumed,0) AS heap_blks_vacuumed,
			COALESCE(p.index_vacuum_count,0) AS index_vacuum_count,
			COALESCE(p.max_dead_tuples,0) AS max_dead_tuples,
			COALESCE(p.num_dead_tuples,0) AS num_dead_tuples
		FROM pg_stat_progress_vacuum p
		LEFT JOIN pg_stat_activity a ON a.pid = p.pid
		ORDER BY p.heap_blks_scanned DESC, p.pid
	`

	rows, err := db.Query(q)
	if err != nil {
		log.Printf("[POSTGRES] GetVacuumProgress query error for %s: %v", instanceName, err)
		return nil, err
	}
	defer rows.Close()

	var out []PgVacuumProgressRow
	for rows.Next() {
		var r PgVacuumProgressRow
		if err := rows.Scan(
			&r.CaptureTimestamp,
			&r.PID,
			&r.DatabaseName,
			&r.UserName,
			&r.RelationName,
			&r.Phase,
			&r.HeapBlksTotal,
			&r.HeapBlksScanned,
			&r.HeapBlksVacuumed,
			&r.IndexVacuumCount,
			&r.MaxDeadTuples,
			&r.NumDeadTuples,
		); err != nil {
			continue
		}
		r.ProgressPct = vacuumProgressPct(r.HeapBlksTotal, r.HeapBlksScanned)
		out = append(out, r)
	}
	return out, rows.Err()
}

