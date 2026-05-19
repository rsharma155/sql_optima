// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB logger for PostgreSQL locks & blocking incident monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PgSessionSnapshotRow struct {
	CollectedAt     time.Time
	ServerID        uuid.UUID
	PID             int
	UserName        string
	DatabaseName    string
	ApplicationName string
	ClientAddr      string
	State           string
	WaitEventType   string
	WaitEvent       string
	XactStart       *time.Time
	QueryStart      *time.Time
	StateChange     *time.Time
	Query           string
}

type PgLockSnapshotRow struct {
	CollectedAt    time.Time `json:"-"`
	ServerID       uuid.UUID `json:"-"`
	PID            int       `json:"pid"`
	LockType       string    `json:"lock_type"`
	Mode           string    `json:"mode"`
	Granted        bool      `json:"granted"`
	RelationOID    uint32    `json:"-"`
	RelationName   string    `json:"relation"`
	TransactionID  string    `json:"waiting_for"`
	WaitingSeconds float64   `json:"waiting_seconds"`
}

type PgBlockingPairRow struct {
	CollectedAt time.Time
	ServerID    uuid.UUID
	BlockedPID  int
	BlockingPID int
}

type PgBlockingIncident struct {
	IncidentID          int64      `json:"incident_id"`
	ServerID            uuid.UUID  `json:"server_id"`
	StartedAt           time.Time  `json:"started_at"`
	EndedAt             *time.Time `json:"ended_at,omitempty"`
	RootBlockerPID      *int       `json:"root_blocker_pid,omitempty"`
	RootBlockerQuery    string     `json:"root_blocker_query,omitempty"`
	PeakBlockedSessions int        `json:"peak_blocked_sessions"`
	Status              string     `json:"status"`
}

func (tl *TimescaleLogger) LogPgSessionSnapshot(ctx context.Context, rows []PgSessionSnapshotRow) error {
	if len(rows) == 0 {
		return nil
	}

	// Dedup: if current batch hash matches previous for this serverID, skip.
	sig := pgFnv64(rows[0].ServerID, len(rows))
	for _, r := range rows {
		sig = pgFnv64(sig, r.PID, r.State, r.WaitEvent, r.Query)
	}
	tl.mu.Lock()
	if prev, ok := tl.prevPgSessionHash[rows[0].ServerID]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgSessionHash[rows[0].ServerID] = sig
	tl.mu.Unlock()

	const q = `
		INSERT INTO monitor.pg_session_snapshot (
			capture_timestamp, server_id, pid, usename, datname, application_name, client_addr,
			state, wait_event_type, wait_event, xact_start, query_start, state_change, query
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q, r.CollectedAt, r.ServerID, r.PID, r.UserName, r.DatabaseName, r.ApplicationName, r.ClientAddr,
			r.State, r.WaitEventType, r.WaitEvent, r.XactStart, r.QueryStart, r.StateChange, r.Query)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogPgLockSnapshot(ctx context.Context, rows []PgLockSnapshotRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO monitor.pg_lock_snapshot (
			capture_timestamp, server_id, pid, locktype, mode, granted, relation_oid, relation_name, transactionid, waiting_seconds
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q, r.CollectedAt, r.ServerID, r.PID, r.LockType, r.Mode, r.Granted, r.RelationOID, r.RelationName, r.TransactionID, r.WaitingSeconds)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogPgBlockingPairs(ctx context.Context, rows []PgBlockingPairRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO monitor.pg_blocking_pairs (capture_timestamp, server_id, blocked_pid, blocking_pid) VALUES ($1,$2,$3,$4)`
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q, r.CollectedAt, r.ServerID, r.BlockedPID, r.BlockingPID)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) OpenPgBlockingIncident(ctx context.Context, serverID uuid.UUID, startedAt time.Time, rootPID *int, rootQuery string) (int64, error) {
	q := `
		INSERT INTO monitor.pg_blocking_incident (server_id, started_at, root_blocker_pid, root_blocker_query, peak_blocked_sessions, status)
		VALUES ($1,$2,$3,$4,0,'active')
		RETURNING incident_id
	`
	var id int64
	if err := tl.pool.QueryRow(ctx, q, serverID, startedAt, rootPID, rootQuery).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (tl *TimescaleLogger) UpdatePgBlockingIncident(ctx context.Context, incidentID int64, peakVictims int, rootPID *int, rootQuery string) error {
	q := `
		UPDATE monitor.pg_blocking_incident
		SET peak_blocked_sessions = GREATEST(COALESCE(peak_blocked_sessions,0), $2),
		    root_blocker_pid = COALESCE($3, root_blocker_pid),
		    root_blocker_query = CASE WHEN COALESCE($4,'') <> '' THEN $4 ELSE root_blocker_query END
		WHERE incident_id = $1
	`
	_, err := tl.pool.Exec(ctx, q, incidentID, peakVictims, rootPID, rootQuery)
	return err
}

func (tl *TimescaleLogger) ClosePgBlockingIncident(ctx context.Context, incidentID int64, endedAt time.Time) error {
	q := `
		UPDATE monitor.pg_blocking_incident
		SET ended_at = $2, status = 'resolved'
		WHERE incident_id = $1
	`
	_, err := tl.pool.Exec(ctx, q, incidentID, endedAt)
	return err
}

func (tl *TimescaleLogger) GetLatestPgSessionSnapshot(ctx context.Context, serverID uuid.UUID) ([]PgSessionSnapshotRow, error) {
	var latestTs *time.Time
	err := tl.pool.QueryRow(ctx, "SELECT MAX(capture_timestamp) FROM monitor.pg_session_snapshot WHERE server_id = $1", serverID).Scan(&latestTs)
	if err != nil || latestTs == nil {
		return nil, err
	}

	q := `
		SELECT capture_timestamp, server_id, pid, usename, datname, application_name, client_addr,
		       state, wait_event_type, wait_event, xact_start, query_start, state_change, query
		FROM monitor.pg_session_snapshot
		WHERE server_id = $1 AND capture_timestamp = $2
		  AND (usename IS NULL OR usename <> 'dbmonitor_user')
		  AND query NOT LIKE '%/* SQL_OPTIMA */%'
	`
	rows, err := tl.pool.Query(ctx, q, serverID, *latestTs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PgSessionSnapshotRow
	for rows.Next() {
		var r PgSessionSnapshotRow
		if err := rows.Scan(
			&r.CollectedAt, &r.ServerID, &r.PID, &r.UserName, &r.DatabaseName, &r.ApplicationName, &r.ClientAddr,
			&r.State, &r.WaitEventType, &r.WaitEvent, &r.XactStart, &r.QueryStart, &r.StateChange, &r.Query,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (tl *TimescaleLogger) GetLatestPgLockSnapshot(ctx context.Context, serverID uuid.UUID) ([]PgLockSnapshotRow, error) {
	var latestTs *time.Time
	err := tl.pool.QueryRow(ctx, "SELECT MAX(capture_timestamp) FROM monitor.pg_lock_snapshot WHERE server_id = $1", serverID).Scan(&latestTs)
	if err != nil || latestTs == nil {
		return nil, err
	}
	// Only return data if the snapshot is fresh (within last 5 minutes).
	if latestTs.Before(time.Now().UTC().Add(-5 * time.Minute)) {
		return nil, nil
	}

	q := `
		SELECT capture_timestamp, server_id, pid, locktype, mode, granted, relation_oid, relation_name, transactionid, waiting_seconds
		FROM monitor.pg_lock_snapshot
		WHERE server_id = $1 AND capture_timestamp = $2
	`
	rows, err := tl.pool.Query(ctx, q, serverID, *latestTs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PgLockSnapshotRow
	for rows.Next() {
		var r PgLockSnapshotRow
		if err := rows.Scan(
			&r.CollectedAt, &r.ServerID, &r.PID, &r.LockType, &r.Mode, &r.Granted, &r.RelationOID, &r.RelationName, &r.TransactionID, &r.WaitingSeconds,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

type PgBlockingTimelinePoint struct {
	Bucket             time.Time `json:"bucket"`
	BlockedSessionsCnt int       `json:"blocked_sessions"`
}

func (tl *TimescaleLogger) GetPgBlockingTimeline(ctx context.Context, serverID uuid.UUID, window time.Duration) ([]PgBlockingTimelinePoint, error) {
	if window <= 0 {
		window = 24 * time.Hour
	}
	from := time.Now().UTC().Add(-window)
	to := time.Now().UTC()
	return tl.GetPgBlockingTimelineRange(ctx, serverID, from, to)
}

func (tl *TimescaleLogger) GetPgBlockingTimelineRange(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]PgBlockingTimelinePoint, error) {
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("from/to required")
	}
	if to.Before(from) {
		return nil, fmt.Errorf("to must be >= from")
	}
	q := `
		SELECT time_bucket_gapfill('1 minute', capture_timestamp, start => $2, finish => $3) AS bucket,
		       COALESCE(COUNT(DISTINCT blocked_pid), 0)::int AS blocked_sessions
		FROM monitor.pg_blocking_pairs
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		GROUP BY bucket
		ORDER BY bucket ASC
	`
	rows, err := tl.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PgBlockingTimelinePoint
	for rows.Next() {
		var p PgBlockingTimelinePoint
		if err := rows.Scan(&p.Bucket, &p.BlockedSessionsCnt); err != nil {
			continue
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) GetPgBlockingIncidentsInWindow(ctx context.Context, serverID uuid.UUID, window time.Duration) ([]PgBlockingIncident, error) {
	if window <= 0 {
		window = 24 * time.Hour
	}
	from := time.Now().UTC().Add(-window)
	to := time.Now().UTC()
	return tl.GetPgBlockingIncidentsRange(ctx, serverID, from, to)
}

func (tl *TimescaleLogger) GetPgBlockingIncidentsRange(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]PgBlockingIncident, error) {
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("from/to required")
	}
	if to.Before(from) {
		return nil, fmt.Errorf("to must be >= from")
	}
	q := `
		SELECT incident_id, server_id, started_at, ended_at, root_blocker_pid, COALESCE(root_blocker_query,''), COALESCE(peak_blocked_sessions,0), COALESCE(status,'')
		FROM monitor.pg_blocking_incident
		WHERE server_id = $1
		  AND started_at >= $2
		  AND started_at <= $3
		  AND root_blocker_query NOT LIKE '%/* SQL_OPTIMA */%'
		ORDER BY started_at ASC
	`
	rows, err := tl.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PgBlockingIncident
	for rows.Next() {
		var r PgBlockingIncident
		var rootPID *int
		var ended *time.Time
		if err := rows.Scan(&r.IncidentID, &r.ServerID, &r.StartedAt, &ended, &rootPID, &r.RootBlockerQuery, &r.PeakBlockedSessions, &r.Status); err != nil {
			continue
		}
		r.EndedAt = ended
		r.RootBlockerPID = rootPID
		out = append(out, r)
	}
	return out, rows.Err()
}

type PgTopLockedTable struct {
	RelationName string  `json:"relation_name"`
	WaitingCount int     `json:"waiting_count"`
	MaxWaitSec   float64 `json:"max_wait_seconds"`
}

func (tl *TimescaleLogger) GetPgTopLockedTables(ctx context.Context, serverID uuid.UUID, lookback time.Duration, limit int) ([]PgTopLockedTable, error) {
	if limit <= 0 {
		limit = 10
	}
	if lookback <= 0 {
		lookback = 10 * time.Minute
	}
	from := time.Now().UTC().Add(-lookback)
	to := time.Now().UTC()
	return tl.GetPgTopLockedTablesRange(ctx, serverID, from, to, limit)
}

func (tl *TimescaleLogger) GetPgTopLockedTablesRange(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]PgTopLockedTable, error) {
	if limit <= 0 {
		limit = 10
	}
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("from/to required")
	}
	if to.Before(from) {
		return nil, fmt.Errorf("to must be >= from")
	}
	q := `
		SELECT COALESCE(NULLIF(relation_name,''), 'virtual') AS relation_name,
		       COUNT(*)::int AS waiting_count,
		       COALESCE(MAX(waiting_seconds),0)::double precision AS max_wait_seconds
		FROM monitor.pg_lock_snapshot
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		  AND granted = FALSE
		GROUP BY relation_name
		ORDER BY waiting_count DESC, max_wait_seconds DESC
		LIMIT $4
	`
	rows, err := tl.pool.Query(ctx, q, serverID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PgTopLockedTable
	for rows.Next() {
		var r PgTopLockedTable
		if err := rows.Scan(&r.RelationName, &r.WaitingCount, &r.MaxWaitSec); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type PgChronicBlocker struct {
	QuerySample      string  `json:"query_sample"`
	RootBlockerCount int     `json:"root_blocker_count"`
	TotalVictims     int     `json:"total_victims"`
	AvgDurationSec   float64 `json:"avg_duration_sec"`
	MaxDurationSec   float64 `json:"max_duration_sec"`
}

func (tl *TimescaleLogger) GetPgChronicBlockersRange(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]PgChronicBlocker, error) {
	if limit <= 0 {
		limit = 15
	}
	q := `
		SELECT
			COALESCE(root_blocker_query, '') AS query_sample,
			COUNT(*)::int AS root_blocker_count,
			COALESCE(SUM(peak_blocked_sessions), 0)::int AS total_victims,
			COALESCE(AVG(EXTRACT(EPOCH FROM (COALESCE(ended_at, NOW()) - started_at))), 0)::double precision AS avg_duration_sec,
			COALESCE(MAX(EXTRACT(EPOCH FROM (COALESCE(ended_at, NOW()) - started_at))), 0)::double precision AS max_duration_sec
		FROM monitor.pg_blocking_incident
		WHERE server_id = $1
		  AND started_at >= $2
		  AND started_at <= $3
		  AND root_blocker_query IS NOT NULL
		  AND root_blocker_query <> ''
		  AND root_blocker_query NOT LIKE '%/* SQL_OPTIMA%'
		GROUP BY root_blocker_query
		ORDER BY root_blocker_count DESC, total_victims DESC
		LIMIT $4
	`
	rows, err := tl.pool.Query(ctx, q, serverID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PgChronicBlocker
	for rows.Next() {
		var r PgChronicBlocker
		if err := rows.Scan(&r.QuerySample, &r.RootBlockerCount, &r.TotalVictims, &r.AvgDurationSec, &r.MaxDurationSec); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type PgBlockingKpis struct {
	CollectedAt            time.Time  `json:"capture_timestamp"`
	ActiveBlockingSessions int        `json:"active_blocking_sessions"`
	IdleInTxnRiskCount     int        `json:"idle_in_txn_risk_count"`
	RootBlockerPID         *int       `json:"root_blocker_pid,omitempty"`
	RootBlockerQuery       string     `json:"root_blocker_query,omitempty"`
	IncidentID             *int64     `json:"incident_id,omitempty"`
	IncidentStartedAt      *time.Time `json:"incident_started_at,omitempty"`
	IncidentDurationMins   int        `json:"incident_duration_mins"`
	ChainDepth             int        `json:"chain_depth"`
}

func (tl *TimescaleLogger) GetPgBlockingKpis(ctx context.Context, serverID uuid.UUID) (*PgBlockingKpis, error) {
	now := time.Now().UTC()
	from := now.Add(-10 * time.Minute)

	k := &PgBlockingKpis{CollectedAt: now}

	qVictims := `
		SELECT COALESCE(MAX(v),0)::int
		FROM (
		  SELECT capture_timestamp, COUNT(DISTINCT blocked_pid) AS v
		  FROM monitor.pg_blocking_pairs
		  WHERE server_id = $1 AND capture_timestamp >= $2
		  GROUP BY capture_timestamp
		) x
	`
	if err := tl.pool.QueryRow(ctx, qVictims, serverID, now.Add(-2*time.Minute)).Scan(&k.ActiveBlockingSessions); err != nil {
		return nil, err
	}

	qIdle := `
		SELECT COALESCE(MAX(n),0)::int
		FROM (
		  SELECT capture_timestamp,
		         COUNT(*) FILTER (WHERE LOWER(COALESCE(state,'')) = 'idle in transaction'
		                          AND (usename IS NULL OR usename <> 'dbmonitor_user')
		                          AND query NOT LIKE '%/* SQL_OPTIMA */%'
		                          AND state_change IS NOT NULL
		                          AND (capture_timestamp - state_change) > INTERVAL '30 seconds') AS n
		  FROM monitor.pg_session_snapshot
		  WHERE server_id = $1 AND capture_timestamp >= $2
		  GROUP BY capture_timestamp
		) x
	`
	if err := tl.pool.QueryRow(ctx, qIdle, serverID, from).Scan(&k.IdleInTxnRiskCount); err != nil {
		return nil, err
	}

	qInc := `
		SELECT incident_id, started_at, root_blocker_pid, COALESCE(root_blocker_query,'')
		FROM monitor.pg_blocking_incident
		WHERE server_id = $1 AND status = 'active'
		  AND root_blocker_query NOT LIKE '%/* SQL_OPTIMA */%'
		ORDER BY started_at DESC
		LIMIT 1
	`
	var incID int64
	var started time.Time
	var rootPID *int
	var rootQ string
	err := tl.pool.QueryRow(ctx, qInc, serverID).Scan(&incID, &started, &rootPID, &rootQ)
	if err == nil {
		k.IncidentID = &incID
		k.IncidentStartedAt = &started
		k.RootBlockerPID = rootPID
		k.RootBlockerQuery = rootQ
		k.IncidentDurationMins = int(now.Sub(started).Minutes())
	}

	qDepth := `
		WITH RECURSIVE latest AS (
		  SELECT capture_timestamp
		  FROM monitor.pg_blocking_pairs
		  WHERE server_id = $1
		  ORDER BY capture_timestamp DESC
		  LIMIT 1
		),
		edges AS (
		  SELECT blocking_pid, blocked_pid
		  FROM monitor.pg_blocking_pairs p
		  JOIN latest l ON p.capture_timestamp = l.capture_timestamp
		  WHERE p.server_id = $1
		),
		roots AS (
		  SELECT DISTINCT e.blocking_pid AS pid
		  FROM edges e
		  WHERE e.blocking_pid NOT IN (SELECT blocked_pid FROM edges)
		),
		rec AS (
		  SELECT r.pid AS root, r.pid AS pid, 1 AS depth, ARRAY[r.pid] AS path
		  FROM roots r
		  UNION ALL
		  SELECT rec.root, e.blocked_pid, rec.depth + 1, rec.path || e.blocked_pid
		  FROM rec
		  JOIN edges e ON e.blocking_pid = rec.pid
		  WHERE rec.depth < 32
		    AND NOT (e.blocked_pid = ANY(rec.path))
		)
		SELECT COALESCE(MAX(depth), 0)::int FROM rec
	`
	var depth int
	if err := tl.pool.QueryRow(ctx, qDepth, serverID).Scan(&depth); err == nil {
		k.ChainDepth = depth
	}

	return k, nil
}

func (tl *TimescaleLogger) EnsurePgLocksBlockingSchema(ctx context.Context) error {
	var ok bool
	q := `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema='monitor' AND table_name='pg_blocking_incident'
		)
	`
	if err := tl.pool.QueryRow(ctx, q).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("monitor.pg_blocking_incident not found (run infrastructure/sql_scripts/01_timescale_schema.sql)")
	}
	return nil
}

// PgBlockingSessionAt is a session snapshot row shaped for the UI (at one capture time).
type PgBlockingSessionAt struct {
	PID        int        `json:"pid"`
	User       string     `json:"user,omitempty"`
	Database   string     `json:"database,omitempty"`
	State      string     `json:"state"`
	QueryStart *time.Time `json:"query_start,omitempty"`
	Duration   string     `json:"duration"`
	WaitEvent  string     `json:"wait_event"`
	Query      string     `json:"query"`
}

// PgBlockingNodeAt mirrors repository.PgBlockingNode but is generated from Timescale snapshots.
type PgBlockingNodeAt struct {
	PID               int                `json:"pid"`
	User              string             `json:"user,omitempty"`
	Database          string             `json:"database,omitempty"`
	ApplicationName   string             `json:"application_name,omitempty"`
	ClientAddr        string             `json:"client_addr,omitempty"`
	State             string             `json:"state"`
	IsIdleInTxn       bool               `json:"is_idle_in_txn"`
	QueryStart        *time.Time         `json:"query_start,omitempty"`
	XactStart         *time.Time         `json:"xact_start,omitempty"`
	Duration          string             `json:"duration"`
	WaitEventType     string             `json:"wait_event_type,omitempty"`
	WaitEvent         string             `json:"wait_event"`
	Query             string             `json:"query"`
	LockModes         []string           `json:"lock_modes,omitempty"`
	AffectedRelations []string           `json:"affected_relations,omitempty"`
	BlockedBy         []PgBlockingNodeAt `json:"blocked_by"`
}

// PgBlockingIncidentSummary represents an incident with computed duration for the UI.
type PgBlockingIncidentSummary struct {
	IncidentID          int64      `json:"incident_id"`
	ServerID            uuid.UUID  `json:"server_id"`
	StartedAt           time.Time  `json:"started_at"`
	EndedAt             *time.Time `json:"ended_at,omitempty"`
	DurationSeconds     float64    `json:"duration_seconds"`
	RootBlockerPID      *int       `json:"root_blocker_pid,omitempty"`
	RootBlockerQuery    string     `json:"root_blocker_query,omitempty"`
	PeakBlockedSessions int        `json:"peak_blocked_sessions"`
	Status              string     `json:"status"`
}

// PgChronicBlockerRow aggregates chronic root-blocker query statistics.
type PgChronicBlockerRow struct {
	QueryFingerprint  string   `json:"query_fingerprint"`
	QuerySample       string   `json:"query_sample"`
	RootBlockerCount  int      `json:"root_blocker_count"`
	TotalVictims      int      `json:"total_victims"`
	AvgDurationSec    float64  `json:"avg_duration_sec"`
	MaxDurationSec    float64  `json:"max_duration_sec"`
	AffectedRelations []string `json:"affected_relations"`
}

type PgBlockingDetailsResponse struct {
	CollectedAt  time.Time          `json:"capture_timestamp"`
	ServerID     uuid.UUID          `json:"server_id"`
	BlockingTree []PgBlockingNodeAt `json:"blocking_tree"`
}

// GetPgBlockingDetailsInRange picks the latest capture within [from,to] and reconstructs a blocking tree
// using monitor.pg_blocking_pairs + monitor.pg_session_snapshot at that capture timestamp.
func (tl *TimescaleLogger) GetPgBlockingDetailsInRange(ctx context.Context, serverID uuid.UUID, from, to time.Time) (*PgBlockingDetailsResponse, error) {
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("from/to required")
	}
	if to.Before(from) {
		return nil, fmt.Errorf("to must be >= from")
	}

	var at *time.Time
	err := tl.pool.QueryRow(ctx, `
		SELECT MAX(capture_timestamp)
		FROM monitor.pg_blocking_pairs
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
	`, serverID, from, to).Scan(&at)
	if err != nil || at == nil {
		return &PgBlockingDetailsResponse{CollectedAt: time.Time{}, ServerID: serverID, BlockingTree: []PgBlockingNodeAt{}}, nil
	}

	type pair struct{ blocked, blocking int }
	pairRows, err := tl.pool.Query(ctx, `
		SELECT blocked_pid, blocking_pid
		FROM monitor.pg_blocking_pairs
		WHERE server_id = $1 AND capture_timestamp = $2
	`, serverID, at)
	if err != nil {
		return nil, err
	}
	defer pairRows.Close()
	var pairs []pair
	pidSet := map[int]struct{}{}
	blockedSet := map[int]struct{}{}
	blockerSet := map[int]struct{}{}
	for pairRows.Next() {
		var b, s int
		if err := pairRows.Scan(&b, &s); err != nil {
			continue
		}
		pairs = append(pairs, pair{blocked: b, blocking: s})
		pidSet[b] = struct{}{}
		pidSet[s] = struct{}{}
		blockedSet[b] = struct{}{}
		blockerSet[s] = struct{}{}
	}
	if err := pairRows.Err(); err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return &PgBlockingDetailsResponse{CollectedAt: *at, ServerID: serverID, BlockingTree: []PgBlockingNodeAt{}}, nil
	}

	sRows, err := tl.pool.Query(ctx, `
		SELECT pid,
		       COALESCE(usename,''), COALESCE(datname,''), COALESCE(application_name,''), COALESCE(client_addr::text,''),
		       COALESCE(state,''), xact_start, query_start, state_change,
		       COALESCE(wait_event_type,''), COALESCE(wait_event,''),
		       COALESCE(query,'')
		FROM monitor.pg_session_snapshot
		WHERE server_id = $1 AND capture_timestamp = $2 AND pid = ANY($3)
		  AND (usename IS NULL OR usename <> 'dbmonitor_user')
		  AND query NOT LIKE '%/* SQL_OPTIMA */%'
	`, serverID, *at, intSliceKeys(pidSet))
	if err != nil {
		return nil, err
	}
	defer sRows.Close()

	nodeMap := make(map[int]PgBlockingNodeAt, len(pidSet))
	for sRows.Next() {
		var pid int
		var us, dn, app, addr, st, wet, we, q string
		var qs, xs, sc *time.Time
		if err := sRows.Scan(&pid, &us, &dn, &app, &addr, &st, &xs, &qs, &sc, &wet, &we, &q); err != nil {
			continue
		}
		wait := ""
		if wet != "" && we != "" {
			wait = wet + ":" + we
		} else if wet != "" {
			wait = wet
		} else {
			wait = we
		}
		dur := "-"
		if sc != nil {
			sec := int(at.Sub(sc.UTC()).Seconds())
			if sec < 0 {
				sec = 0
			}
			dur = fmt.Sprintf("%dm %ds", sec/60, sec%60)
		}
		nodeMap[pid] = PgBlockingNodeAt{
			PID:             pid,
			User:            us,
			Database:        dn,
			ApplicationName: app,
			ClientAddr:      addr,
			State:           st,
			IsIdleInTxn:     containsIdle(st),
			XactStart:       xs,
			QueryStart:      qs,
			Duration:        dur,
			WaitEventType:   wet,
			WaitEvent:       wait,
			Query:           q,
		}
	}

	lkRows, lkErr := tl.pool.Query(ctx, `
		SELECT pid,
		       array_agg(DISTINCT mode ORDER BY mode) AS lock_modes,
		       array_agg(DISTINCT relation_name ORDER BY relation_name) AS relations
		FROM monitor.pg_lock_snapshot
		WHERE server_id = $1
		  AND capture_timestamp BETWEEN $2 AND $3
		  AND pid = ANY($4)
		GROUP BY pid
	`, serverID, at.Add(-30*time.Second), at.Add(30*time.Second), intSliceKeys(pidSet))
	if lkErr == nil {
		defer lkRows.Close()
		for lkRows.Next() {
			var pid int
			var modes, rels []string
			if err := lkRows.Scan(&pid, &modes, &rels); err != nil {
				continue
			}
			if node, ok := nodeMap[pid]; ok {
				node.LockModes = filterStrings(modes, "")
				node.AffectedRelations = filterStrings(rels, "virtual")
				nodeMap[pid] = node
			}
		}
	}

	for pid := range pidSet {
		if _, ok := nodeMap[pid]; ok {
			continue
		}
		nodeMap[pid] = PgBlockingNodeAt{PID: pid, State: "unknown", Duration: "-", WaitEvent: "", Query: ""}
	}

	children := make(map[int][]int)
	for _, p := range pairs {
		children[p.blocking] = append(children[p.blocking], p.blocked)
	}
	var roots []int
	for pid := range blockerSet {
		if _, isBlocked := blockedSet[pid]; !isBlocked {
			roots = append(roots, pid)
		}
	}
	sort.Ints(roots)

	var build func(pid int, seen map[int]struct{}) PgBlockingNodeAt
	build = func(pid int, seen map[int]struct{}) PgBlockingNodeAt {
		n := nodeMap[pid]
		if seen == nil {
			seen = map[int]struct{}{}
		}
		if _, ok := seen[pid]; ok {
			return n
		}
		seen2 := make(map[int]struct{}, len(seen)+1)
		for k := range seen {
			seen2[k] = struct{}{}
		}
		seen2[pid] = struct{}{}
		for _, ch := range children[pid] {
			n.BlockedBy = append(n.BlockedBy, build(ch, seen2))
		}
		return n
	}

	out := make([]PgBlockingNodeAt, 0, len(roots))
	for _, r := range roots {
		out = append(out, build(r, map[int]struct{}{}))
	}

	return &PgBlockingDetailsResponse{
		CollectedAt:  *at,
		ServerID:     serverID,
		BlockingTree: out,
	}, nil
}

func intSliceKeys(m map[int]struct{}) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func containsIdle(state string) bool {
	s := strings.ToLower(state)
	return len(s) >= 15 && s[:15] == "idle in transac"
}

func filterStrings(ss []string, exclude string) []string {
	var out []string
	for _, s := range ss {
		if s != "" && s != exclude {
			out = append(out, s)
		}
	}
	return out
}

func (tl *TimescaleLogger) GetPgBlockingIncidents(ctx context.Context, serverID uuid.UUID, windowHours, limit int) ([]PgBlockingIncidentSummary, error) {
	if windowHours <= 0 {
		windowHours = 168
	}
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT incident_id, server_id, started_at, ended_at,
		       root_blocker_pid, COALESCE(root_blocker_query,''),
		       COALESCE(peak_blocked_sessions,0), COALESCE(status,''),
		       EXTRACT(EPOCH FROM (COALESCE(ended_at, now()) - started_at)) AS duration_seconds
		FROM monitor.pg_blocking_incident
		WHERE server_id = $1
		  AND started_at >= now() - ($2 * INTERVAL '1 hour')
		  AND COALESCE(root_blocker_query,'') NOT LIKE '%/* SQL_OPTIMA */%'
		ORDER BY started_at DESC
		LIMIT $3
	`
	rows, err := tl.pool.Query(ctx, q, serverID, windowHours, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PgBlockingIncidentSummary
	for rows.Next() {
		var r PgBlockingIncidentSummary
		var rootPID *int
		var ended *time.Time
		if err := rows.Scan(&r.IncidentID, &r.ServerID, &r.StartedAt, &ended, &rootPID, &r.RootBlockerQuery, &r.PeakBlockedSessions, &r.Status, &r.DurationSeconds); err != nil {
			continue
		}
		r.EndedAt = ended
		r.RootBlockerPID = rootPID
		out = append(out, r)
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) GetPgChronicBlockers(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]PgChronicBlockerRow, error) {
	if limit <= 0 {
		limit = 15
	}
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("from/to required")
	}
	q := `
		WITH incidents AS (
		    SELECT incident_id, root_blocker_pid, root_blocker_query, started_at, ended_at
		    FROM monitor.pg_blocking_incident
		    WHERE server_id = $1
		      AND started_at BETWEEN $2 AND $3
		      AND root_blocker_query IS NOT NULL
		      AND root_blocker_query NOT LIKE '%/* SQL_OPTIMA */%'
		),
		enriched AS (
		    SELECT
		        md5(regexp_replace(i.root_blocker_query, '\$\d+|\d+', '?', 'g')) AS fingerprint,
		        i.root_blocker_query                                              AS query_sample,
		        COUNT(*)::int                                                     AS root_count,
		        SUM(p.victim_count)::int                                          AS total_victims,
		        AVG(EXTRACT(EPOCH FROM (COALESCE(i.ended_at, now()) - i.started_at))) AS avg_dur,
		        MAX(EXTRACT(EPOCH FROM (COALESCE(i.ended_at, now()) - i.started_at))) AS max_dur
		    FROM incidents i
		    LEFT JOIN LATERAL (
		        SELECT COUNT(DISTINCT blocked_pid) AS victim_count
		        FROM monitor.pg_blocking_pairs p
		        WHERE p.server_id = $1
		          AND p.blocking_pid = i.root_blocker_pid
		          AND p.capture_timestamp BETWEEN i.started_at AND COALESCE(i.ended_at, now())
		    ) p ON true
		    GROUP BY fingerprint, i.root_blocker_query
		)
		SELECT fingerprint, query_sample,
		       root_count, COALESCE(total_victims,0) AS total_victims,
		       COALESCE(avg_dur,0) AS avg_dur, COALESCE(max_dur,0) AS max_dur
		FROM enriched
		ORDER BY root_count DESC, total_victims DESC
		LIMIT $4
	`
	rows, err := tl.pool.Query(ctx, q, serverID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PgChronicBlockerRow
	for rows.Next() {
		var r PgChronicBlockerRow
		if err := rows.Scan(&r.QueryFingerprint, &r.QuerySample, &r.RootBlockerCount, &r.TotalVictims, &r.AvgDurationSec, &r.MaxDurationSec); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
