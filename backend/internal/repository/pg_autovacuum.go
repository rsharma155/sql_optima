// Package repository provides data access functions for SQL Optima.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL autovacuum, bloat, idle-in-transaction, XID wraparound risk, and WAL archiver risk queries.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
	"time"
)

// PgBloatEstimate holds heuristic table bloat signals for one table.
type PgBloatEstimate struct {
	SchemaName       string     `json:"schema"`
	TableName        string     `json:"table"`
	TotalBytes       int64      `json:"total_bytes"`
	TotalSizePretty  string     `json:"total_size"`
	LiveTuples       int64      `json:"live_tuples"`
	DeadTuples       int64      `json:"dead_tuples"`
	DeadPct          float64    `json:"dead_pct"`
	EstimatedWasteMB float64    `json:"estimated_waste_mb"`
	SeqScans         int64      `json:"seq_scans"`
	LastAutovacuum   *time.Time `json:"last_autovacuum,omitempty"`
	LastVacuum       *time.Time `json:"last_vacuum,omitempty"`
	TableFreezeAge   int64      `json:"table_freeze_age"`
	VacuumLagSeconds float64    `json:"vacuum_lag_seconds"`
	Recommendation   string     `json:"recommendation"`
}

// PgIdleInTransactionSession holds one idle-in-transaction session.
type PgIdleInTransactionSession struct {
	PID           int64      `json:"pid"`
	UserName      string     `json:"user_name"`
	Database      string     `json:"database"`
	ClientAddr    string     `json:"client_addr"`
	State         string     `json:"state"`
	Query         string     `json:"query"`
	QueryStart    *time.Time `json:"query_start,omitempty"`
	IdleSeconds   float64    `json:"idle_seconds"`
	WaitEventType string     `json:"wait_event_type"`
	WaitEvent     string     `json:"wait_event"`
}

// PgXIDWraparoundRisk holds XID freeze risk for one database.
type PgXIDWraparoundRisk struct {
	DatabaseName    string  `json:"database_name"`
	FreezeAge       int64   `json:"freeze_age"`
	WraparoundLimit int64   `json:"wraparound_limit"`
	UsedPct         float64 `json:"used_pct"`
	RiskLevel       string  `json:"risk_level"` // "low", "medium", "high", "critical"
}

// PgWALArchiverRisk summarises WAL archiver health and retention risk.
type PgWALArchiverRisk struct {
	ArchivedCount     int64     `json:"archived_count"`
	FailedCount       int64     `json:"failed_count"`
	LastArchivedWal   string    `json:"last_archived_wal"`
	LastArchivedAge   float64   `json:"last_archived_age_seconds"`
	LastFailedWal     string    `json:"last_failed_wal"`
	FailureRatePct    float64   `json:"failure_rate_pct"`
	MaxRetainedSlotMB float64   `json:"max_retained_slot_mb"`
	HighRetentionSlot string    `json:"high_retention_slot"`
	ArchiverEnabled   bool      `json:"archiver_enabled"`
	RiskLevel         string    `json:"risk_level"`
	CaptureTimestamp  time.Time `json:"capture_timestamp"`
}

// GetBloatEstimates returns per-table bloat signals ordered by dead tuple count.
func (c *PgRepository) GetBloatEstimates(instanceName string, limit int) ([]PgBloatEstimate, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
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
		return nil, fmt.Errorf("connection not found for %s", instanceName)
	}

	q := `
		SELECT
			s.schemaname,
			s.relname,
			pg_total_relation_size(c.oid)                                        AS total_bytes,
			pg_size_pretty(pg_total_relation_size(c.oid))                        AS total_pretty,
			COALESCE(s.n_live_tup, 0)                                            AS live_tuples,
			COALESCE(s.n_dead_tup, 0)                                            AS dead_tuples,
			CASE
				WHEN COALESCE(s.n_live_tup,0) + COALESCE(s.n_dead_tup,0) > 0
				THEN ROUND(100.0 * COALESCE(s.n_dead_tup,0)::numeric /
				           (COALESCE(s.n_live_tup,0) + COALESCE(s.n_dead_tup,0)), 2)::float8
				ELSE 0
			END                                                                   AS dead_pct,
			GREATEST(0,
				ROUND((COALESCE(s.n_dead_tup,0)::numeric / NULLIF(c.reltuples, 0))
				      * pg_relation_size(c.oid) / 1048576.0, 2)
			)::float8                                                             AS estimated_waste_mb,
			COALESCE(s.seq_scan, 0)                                              AS seq_scans,
			s.last_autovacuum,
			s.last_vacuum,
			age(c.relfrozenxid)::bigint                                          AS table_freeze_age,
			COALESCE(EXTRACT(EPOCH FROM (now() - s.last_autovacuum))::float8, -1) AS vacuum_lag_seconds
		FROM pg_stat_user_tables s
		JOIN pg_class c
			ON c.relname = s.relname
			AND c.relnamespace = (SELECT oid FROM pg_namespace WHERE nspname = s.schemaname)
		WHERE c.relkind = 'r'
		  AND c.reltuples > 0
		  AND COALESCE(s.n_dead_tup, 0) > 0
		ORDER BY COALESCE(s.n_dead_tup, 0) DESC
		LIMIT $1`

	rows, err := db.Query(q, limit)
	if err != nil {
		return nil, fmt.Errorf("GetBloatEstimates query: %w", err)
	}
	defer rows.Close()

	var out []PgBloatEstimate
	for rows.Next() {
		var r PgBloatEstimate
		if err := rows.Scan(
			&r.SchemaName, &r.TableName, &r.TotalBytes, &r.TotalSizePretty,
			&r.LiveTuples, &r.DeadTuples, &r.DeadPct, &r.EstimatedWasteMB,
			&r.SeqScans, &r.LastAutovacuum, &r.LastVacuum,
			&r.TableFreezeAge, &r.VacuumLagSeconds,
		); err != nil {
			continue
		}
		// Derive recommendation based on dead_pct and freeze age thresholds.
		const maxXIDFreeze int64 = 2100000000
		switch {
		case r.TableFreezeAge > maxXIDFreeze*3/4:
			r.Recommendation = "VACUUM FREEZE urgently — freeze age >75% of XID limit"
		case r.TableFreezeAge > maxXIDFreeze/2:
			r.Recommendation = "Consider VACUUM FREEZE — freeze age >50% of XID limit"
		case r.DeadPct >= 30:
			r.Recommendation = "VACUUM required immediately — >30% dead tuples"
		case r.DeadPct >= 10:
			r.Recommendation = "Schedule VACUUM — 10-30% dead tuples"
		default:
			r.Recommendation = "Monitor — autovacuum may be delayed"
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetIdleInTransactionSessions returns sessions currently idle in a transaction.
func (c *PgRepository) GetIdleInTransactionSessions(instanceName string) ([]PgIdleInTransactionSession, error) {
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
		return nil, fmt.Errorf("connection not found for %s", instanceName)
	}

	q := `
		SELECT
			pid,
			COALESCE(usename, '')         AS user_name,
			COALESCE(datname, '')         AS database,
			COALESCE(client_addr::text, '') AS client_addr,
			state,
			COALESCE(query, '')           AS query,
			query_start,
			EXTRACT(EPOCH FROM (now() - query_start))::float8 AS idle_seconds,
			COALESCE(wait_event_type, '') AS wait_event_type,
			COALESCE(wait_event, '')      AS wait_event
		FROM pg_stat_activity
		WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')
		  AND pid <> pg_backend_pid()
		ORDER BY idle_seconds DESC NULLS LAST
		LIMIT 200`

	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("GetIdleInTransactionSessions query: %w", err)
	}
	defer rows.Close()

	var out []PgIdleInTransactionSession
	for rows.Next() {
		var r PgIdleInTransactionSession
		if err := rows.Scan(
			&r.PID, &r.UserName, &r.Database, &r.ClientAddr,
			&r.State, &r.Query, &r.QueryStart, &r.IdleSeconds,
			&r.WaitEventType, &r.WaitEvent,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetXIDWraparoundRisk returns per-database XID freeze risk derived from age(datfrozenxid).
func (c *PgRepository) GetXIDWraparoundRisk(instanceName string) ([]PgXIDWraparoundRisk, error) {
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
		return nil, fmt.Errorf("connection not found for %s", instanceName)
	}

	// wraparound_limit is autovacuum_freeze_max_age (default 200M); we derive it from pg_settings.
	q := `
		WITH limit_val AS (
			SELECT COALESCE(setting::bigint, 200000000) AS max_age
			FROM pg_settings WHERE name = 'autovacuum_freeze_max_age'
		)
		SELECT
			datname,
			age(datfrozenxid)::bigint AS freeze_age,
			l.max_age
		FROM pg_database
		CROSS JOIN limit_val l
		WHERE datname NOT IN ('template0', 'template1')
		  AND datallowconn
		ORDER BY freeze_age DESC
		LIMIT 100`

	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("GetXIDWraparoundRisk query: %w", err)
	}
	defer rows.Close()

	const maxXID int64 = 2100000000
	var out []PgXIDWraparoundRisk
	for rows.Next() {
		var r PgXIDWraparoundRisk
		if err := rows.Scan(&r.DatabaseName, &r.FreezeAge, &r.WraparoundLimit); err != nil {
			continue
		}
		if maxXID > 0 {
			r.UsedPct = float64(r.FreezeAge) / float64(maxXID) * 100.0
		}
		switch {
		case r.UsedPct >= 75:
			r.RiskLevel = "critical"
		case r.UsedPct >= 50:
			r.RiskLevel = "high"
		case r.UsedPct >= 25:
			r.RiskLevel = "medium"
		default:
			r.RiskLevel = "low"
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PgLongRunningTransaction holds one long-running active transaction.
type PgLongRunningTransaction struct {
	PID                int64      `json:"pid"`
	UserName           string     `json:"user_name"`
	Database           string     `json:"database"`
	ClientAddr         string     `json:"client_addr"`
	State              string     `json:"state"`
	Query              string     `json:"query"`
	XactStart          *time.Time `json:"xact_start,omitempty"`
	TxnDurationSeconds float64    `json:"txn_duration_seconds"`
	WaitEventType      string     `json:"wait_event_type"`
	WaitEvent          string     `json:"wait_event"`
}

// GetLongRunningTransactions returns active transactions running longer than 1 minute.
func (c *PgRepository) GetLongRunningTransactions(instanceName string) ([]PgLongRunningTransaction, error) {
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
		return nil, fmt.Errorf("connection not found for %s", instanceName)
	}

	q := `
		SELECT
			pid,
			COALESCE(usename, '')           AS user_name,
			COALESCE(datname, '')           AS database,
			COALESCE(client_addr::text, '') AS client_addr,
			state,
			COALESCE(query, '')             AS query,
			xact_start,
			EXTRACT(EPOCH FROM (now() - xact_start))::float8 AS txn_duration_seconds,
			COALESCE(wait_event_type, '')   AS wait_event_type,
			COALESCE(wait_event, '')        AS wait_event
		FROM pg_stat_activity
		WHERE state = 'active'
		  AND xact_start IS NOT NULL
		  AND now() - xact_start > interval '1 minute'
		  AND pid <> pg_backend_pid()
		ORDER BY txn_duration_seconds DESC NULLS LAST
		LIMIT 100`

	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("GetLongRunningTransactions query: %w", err)
	}
	defer rows.Close()

	var out []PgLongRunningTransaction
	for rows.Next() {
		var r PgLongRunningTransaction
		if err := rows.Scan(
			&r.PID, &r.UserName, &r.Database, &r.ClientAddr,
			&r.State, &r.Query, &r.XactStart, &r.TxnDurationSeconds,
			&r.WaitEventType, &r.WaitEvent,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PgIndexBloat holds index size and usage signals for one index.
type PgIndexBloat struct {
	SchemaName      string `json:"schema"`
	TableName       string `json:"table"`
	IndexName       string `json:"index_name"`
	IndexSizeBytes  int64  `json:"index_size_bytes"`
	IndexSizePretty string `json:"index_size"`
	IdxScans        int64  `json:"idx_scans"`
	IsUnique        bool   `json:"is_unique"`
	IsPrimary       bool   `json:"is_primary"`
	Recommendation  string `json:"recommendation"`
}

// GetIndexBloat returns index size and usage signals ordered by size desc.
func (c *PgRepository) GetIndexBloat(instanceName string, limit int) ([]PgIndexBloat, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
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
		return nil, fmt.Errorf("connection not found for %s", instanceName)
	}

	q := `
		SELECT
			i.schemaname,
			i.relname                                            AS table_name,
			i.indexrelname                                       AS index_name,
			pg_relation_size(i.indexrelid)                       AS index_bytes,
			pg_size_pretty(pg_relation_size(i.indexrelid))       AS index_size,
			COALESCE(i.idx_scan, 0)                              AS idx_scans,
			ix.indisunique                                       AS is_unique,
			ix.indisprimary                                      AS is_primary
		FROM pg_stat_user_indexes i
		JOIN pg_index ix ON ix.indexrelid = i.indexrelid
		WHERE pg_relation_size(i.indexrelid) > 0
		ORDER BY pg_relation_size(i.indexrelid) DESC
		LIMIT $1`

	rows, err := db.Query(q, limit)
	if err != nil {
		return nil, fmt.Errorf("GetIndexBloat query: %w", err)
	}
	defer rows.Close()

	var out []PgIndexBloat
	for rows.Next() {
		var r PgIndexBloat
		if err := rows.Scan(
			&r.SchemaName, &r.TableName, &r.IndexName,
			&r.IndexSizeBytes, &r.IndexSizePretty,
			&r.IdxScans, &r.IsUnique, &r.IsPrimary,
		); err != nil {
			continue
		}
		switch {
		case r.IsPrimary:
			r.Recommendation = "Primary key — keep"
		case r.IsUnique:
			r.Recommendation = "Unique constraint — keep"
		case r.IdxScans == 0:
			r.Recommendation = "Unused index — consider dropping to reduce write overhead"
		case r.IndexSizeBytes >= 1073741824: // 1 GB
			r.Recommendation = "Large index — consider REINDEX CONCURRENTLY if fragmented"
		default:
			r.Recommendation = "Active — monitor for fragmentation"
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetWALArchiverRisk returns a combined WAL archiver health and slot retention summary.
func (c *PgRepository) GetWALArchiverRisk(instanceName string) (*PgWALArchiverRisk, error) {
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
		return nil, fmt.Errorf("connection not found for %s", instanceName)
	}

	result := &PgWALArchiverRisk{CaptureTimestamp: time.Now().UTC()}

	// Archiver stats.
	archQ := `
		SELECT
			archived_count,
			failed_count,
			COALESCE(last_archived_wal, '')    AS last_archived_wal,
			COALESCE(EXTRACT(EPOCH FROM (now() - last_archived_time))::float8, -1) AS last_age_sec,
			COALESCE(last_failed_wal, '')      AS last_failed_wal
		FROM pg_stat_archiver`
	row := db.QueryRow(archQ)
	var ageSec float64
	if err := row.Scan(
		&result.ArchivedCount, &result.FailedCount,
		&result.LastArchivedWal, &ageSec, &result.LastFailedWal,
	); err == nil {
		result.LastArchivedAge = ageSec
		total := result.ArchivedCount + result.FailedCount
		if total > 0 {
			result.FailureRatePct = float64(result.FailedCount) / float64(total) * 100.0
		}
		result.ArchiverEnabled = true
	}

	// Replication slot retention.
	slotQ := `
		SELECT
			COALESCE(slot_name, '')                                            AS slot_name,
			COALESCE(
				(pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn) / 1048576.0)::float8,
				0
			) AS retained_mb
		FROM pg_replication_slots
		WHERE active = false OR restart_lsn IS NOT NULL
		ORDER BY retained_mb DESC
		LIMIT 1`
	sr := db.QueryRow(slotQ)
	var slotName string
	var retMB float64
	if err := sr.Scan(&slotName, &retMB); err == nil {
		result.MaxRetainedSlotMB = retMB
		result.HighRetentionSlot = slotName
	}

	// Derive risk level.
	switch {
	case result.FailureRatePct >= 10 || result.MaxRetainedSlotMB >= 10240 || (result.ArchiverEnabled && result.LastArchivedAge > 3600):
		result.RiskLevel = "critical"
	case result.FailureRatePct >= 5 || result.MaxRetainedSlotMB >= 2048 || (result.ArchiverEnabled && result.LastArchivedAge > 600):
		result.RiskLevel = "high"
	case result.FailureRatePct >= 1 || result.MaxRetainedSlotMB >= 512 || (result.ArchiverEnabled && result.LastArchivedAge > 120):
		result.RiskLevel = "medium"
	default:
		result.RiskLevel = "low"
	}

	return result, nil
}
