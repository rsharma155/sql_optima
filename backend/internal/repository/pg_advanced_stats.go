// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Advanced PostgreSQL statistics for vacuum, autovacuum, and background writer.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
)

type PgWaitEventCount struct {
	WaitEventType string `json:"wait_event_type"`
	WaitEvent     string `json:"wait_event"`
	SessionsCount int    `json:"sessions_count"`
}

type PgDbIOStat struct {
	DatabaseName string `json:"database_name"`
	BlksRead     int64  `json:"blks_read"`
	BlksHit      int64  `json:"blks_hit"`
	TempFiles    int64  `json:"temp_files"`
	TempBytes    int64  `json:"temp_bytes"`
}

type PgSettingSnapRow struct {
	Name   string `json:"name"`
	Setting string `json:"setting"`
	Unit   string `json:"unit"`
	Source string `json:"source"`
}

func (c *PgRepository) GetWaitEventCounts(instanceName string) ([]PgWaitEventCount, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	q := `
		SELECT COALESCE(wait_event_type, '') AS wait_event_type,
		       COALESCE(wait_event, '') AS wait_event,
		       COUNT(*)::int AS sessions_count
		FROM pg_stat_activity
		WHERE wait_event_type IS NOT NULL
		GROUP BY 1,2
		ORDER BY sessions_count DESC
	`
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PgWaitEventCount
	for rows.Next() {
		var r PgWaitEventCount
		if err := rows.Scan(&r.WaitEventType, &r.WaitEvent, &r.SessionsCount); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (c *PgRepository) GetDbIOStats(instanceName string) ([]PgDbIOStat, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	q := `
		SELECT datname,
		       blks_read,
		       blks_hit,
		       temp_files,
		       temp_bytes
		FROM pg_stat_database
		WHERE datname IS NOT NULL
		  AND datname NOT IN ('template0','template1')
	`
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PgDbIOStat
	for rows.Next() {
		var r PgDbIOStat
		if err := rows.Scan(&r.DatabaseName, &r.BlksRead, &r.BlksHit, &r.TempFiles, &r.TempBytes); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetSettingsSnapshot returns a curated set of settings for drift tracking.
func (c *PgRepository) GetSettingsSnapshot(instanceName string) ([]PgSettingSnapRow, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// Curated “drift” set: enough to explain most changes without dumping all pg_settings.
	names := []string{
		"shared_buffers",
		"effective_cache_size",
		"work_mem",
		"maintenance_work_mem",
		"max_connections",
		"max_worker_processes",
		"max_parallel_workers",
		"max_parallel_workers_per_gather",
		"autovacuum",
		"autovacuum_max_workers",
		"autovacuum_naptime",
		"autovacuum_vacuum_scale_factor",
		"autovacuum_analyze_scale_factor",
		"checkpoint_timeout",
		"max_wal_size",
		"min_wal_size",
		"wal_keep_size",
		"synchronous_commit",
		"fsync",
		"full_page_writes",
		"wal_level",
	}
	sort.Strings(names)

	// Build IN list safely using sql placeholders.
	placeholders := ""
	args := make([]interface{}, 0, len(names))
	for i, n := range names {
		if i > 0 {
			placeholders += ","
		}
		placeholders += fmt.Sprintf("$%d", i+1)
		args = append(args, n)
	}

	q := fmt.Sprintf(`
		SELECT name,
		       COALESCE(setting,'') AS setting,
		       COALESCE(unit,'') AS unit,
		       COALESCE(source,'') AS source
		FROM pg_settings
		WHERE name IN (%s)
	`, placeholders)

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PgSettingSnapRow
	for rows.Next() {
		var r PgSettingSnapRow
		if err := rows.Scan(&r.Name, &r.Setting, &r.Unit, &r.Source); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		log.Printf("[POSTGRES] GetSettingsSnapshot returned 0 rows for %s", instanceName)
	}
	return out, nil
}

// Compile-time check: ensure we still import sql when needed in this file.
var _ = sql.ErrNoRows

