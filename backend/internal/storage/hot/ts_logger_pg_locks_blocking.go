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
	"time"
)

type PgSessionSnapshotRow struct {
	CollectedAt      time.Time
	ServerID         string
	PID              int
	UserName         string
	DatabaseName     string
	ApplicationName  string
	ClientAddr       string
	State            string
	WaitEventType    string
	WaitEvent        string
	XactStart        *time.Time
	QueryStart       *time.Time
	StateChange      *time.Time
	Query            string
}

type PgLockSnapshotRow struct {
	CollectedAt     time.Time
	ServerID        string
	PID             int
	LockType        string
	Mode            string
	Granted         bool
	RelationOID     uint32 // oid fits uint32; 0 means "virtual/no relation"
	RelationName    string
	TransactionID   string
	WaitingSeconds  float64
}

type PgBlockingPairRow struct {
	CollectedAt time.Time
	ServerID    string
	BlockedPID  int
	BlockingPID int
}

type PgBlockingIncident struct {
	IncidentID         int64      `json:"incident_id"`
	ServerID           string     `json:"server_id"`
	StartedAt          time.Time  `json:"started_at"`
	EndedAt            *time.Time `json:"ended_at,omitempty"`
	RootBlockerPID     *int       `json:"root_blocker_pid,omitempty"`
	RootBlockerQuery   string     `json:"root_blocker_query,omitempty"`
	PeakBlockedSessions int       `json:"peak_blocked_sessions"`
	Status             string     `json:"status"`
}

func (tl *TimescaleLogger) LogPgSessionSnapshot(ctx context.Context, rows []PgSessionSnapshotRow) error {
	if len(rows) == 0 {
		return nil
	}
	q := `
		INSERT INTO monitor.pg_session_snapshot (
			collected_at, server_id, pid, usename, datname, application_name, client_addr,
			state, wait_event_type, wait_event, xact_start, query_start, state_change, query
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`
	for _, r := range rows {
		_, err := tl.pool.Exec(ctx, q,
			r.CollectedAt, r.ServerID, r.PID, r.UserName, r.DatabaseName, r.ApplicationName, r.ClientAddr,
			r.State, r.WaitEventType, r.WaitEvent, r.XactStart, r.QueryStart, r.StateChange, r.Query,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogPgLockSnapshot(ctx context.Context, rows []PgLockSnapshotRow) error {
	if len(rows) == 0 {
		return nil
	}
	q := `
		INSERT INTO monitor.pg_lock_snapshot (
			collected_at, server_id, pid, locktype, mode, granted, relation_oid, relation_name, transactionid, waiting_seconds
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`
	for _, r := range rows {
		_, err := tl.pool.Exec(ctx, q,
			r.CollectedAt, r.ServerID, r.PID, r.LockType, r.Mode, r.Granted, r.RelationOID, r.RelationName, r.TransactionID, r.WaitingSeconds,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogPgBlockingPairs(ctx context.Context, rows []PgBlockingPairRow) error {
	if len(rows) == 0 {
		return nil
	}
	q := `INSERT INTO monitor.pg_blocking_pairs (collected_at, server_id, blocked_pid, blocking_pid) VALUES ($1,$2,$3,$4)`
	for _, r := range rows {
		_, err := tl.pool.Exec(ctx, q, r.CollectedAt, r.ServerID, r.BlockedPID, r.BlockingPID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) OpenPgBlockingIncident(ctx context.Context, serverID string, startedAt time.Time, rootPID *int, rootQuery string) (int64, error) {
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
		  AND status = 'active'
	`
	_, err := tl.pool.Exec(ctx, q, incidentID, endedAt)
	return err
}

type PgBlockingTimelinePoint struct {
	Bucket             time.Time `json:"bucket"`
	BlockedSessionsCnt int       `json:"blocked_sessions"`
}

func (tl *TimescaleLogger) GetPgBlockingTimeline(ctx context.Context, serverID string, window time.Duration) ([]PgBlockingTimelinePoint, error) {
	if window <= 0 {
		window = 24 * time.Hour
	}
	from := time.Now().UTC().Add(-window)
	to := time.Now().UTC()
	return tl.GetPgBlockingTimelineRange(ctx, serverID, from, to)
}

func (tl *TimescaleLogger) GetPgBlockingTimelineRange(ctx context.Context, serverID string, from, to time.Time) ([]PgBlockingTimelinePoint, error) {
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("from/to required")
	}
	if to.Before(from) {
		return nil, fmt.Errorf("to must be >= from")
	}
	// Use time_bucket_gapfill so the UI gets explicit zero buckets when there is no blocking,
	// allowing the trendline to naturally fall back to 0.
	q := `
		SELECT time_bucket_gapfill('1 minute', collected_at, start => $2, finish => $3) AS bucket,
		       COALESCE(COUNT(DISTINCT blocked_pid), 0)::int AS blocked_sessions
		FROM monitor.pg_blocking_pairs
		WHERE server_id = $1
		  AND collected_at >= $2
		  AND collected_at <= $3
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

func (tl *TimescaleLogger) GetPgBlockingIncidentsInWindow(ctx context.Context, serverID string, window time.Duration) ([]PgBlockingIncident, error) {
	if window <= 0 {
		window = 24 * time.Hour
	}
	from := time.Now().UTC().Add(-window)
	to := time.Now().UTC()
	return tl.GetPgBlockingIncidentsRange(ctx, serverID, from, to)
}

func (tl *TimescaleLogger) GetPgBlockingIncidentsRange(ctx context.Context, serverID string, from, to time.Time) ([]PgBlockingIncident, error) {
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
	RelationName  string  `json:"relation_name"`
	WaitingCount  int     `json:"waiting_count"`
	MaxWaitSec    float64 `json:"max_wait_seconds"`
}

func (tl *TimescaleLogger) GetPgTopLockedTables(ctx context.Context, serverID string, lookback time.Duration, limit int) ([]PgTopLockedTable, error) {
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

func (tl *TimescaleLogger) GetPgTopLockedTablesRange(ctx context.Context, serverID string, from, to time.Time, limit int) ([]PgTopLockedTable, error) {
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
		  AND collected_at >= $2
		  AND collected_at <= $3
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

type PgBlockingKpis struct {
	CollectedAt            time.Time `json:"collected_at"`
	ActiveBlockingSessions int       `json:"active_blocking_sessions"`
	IdleInTxnRiskCount     int       `json:"idle_in_txn_risk_count"`
	RootBlockerPID         *int      `json:"root_blocker_pid,omitempty"`
	RootBlockerQuery       string    `json:"root_blocker_query,omitempty"`
	IncidentID             *int64    `json:"incident_id,omitempty"`
	IncidentStartedAt      *time.Time `json:"incident_started_at,omitempty"`
	IncidentDurationMins   int       `json:"incident_duration_mins"`
	ChainDepth             int       `json:"chain_depth"`
}

func (tl *TimescaleLogger) GetPgBlockingKpis(ctx context.Context, serverID string) (*PgBlockingKpis, error) {
	// KPI source: latest incident row + latest pair snapshot victims + latest idle-in-txn risk from sessions snapshot.
	// Use a short lookback to avoid heavy scans.
	now := time.Now().UTC()
	from := now.Add(-10 * time.Minute)

	k := &PgBlockingKpis{CollectedAt: now}

	// Active victims from last 2 minutes.
	qVictims := `
		SELECT COALESCE(MAX(v),0)::int
		FROM (
		  SELECT collected_at, COUNT(DISTINCT blocked_pid) AS v
		  FROM monitor.pg_blocking_pairs
		  WHERE server_id = $1 AND collected_at >= $2
		  GROUP BY collected_at
		) x
	`
	if err := tl.pool.QueryRow(ctx, qVictims, serverID, now.Add(-2*time.Minute)).Scan(&k.ActiveBlockingSessions); err != nil {
		return nil, err
	}

	// Idle in txn risk count: state = 'idle in transaction' and age > 30s based on state_change.
	qIdle := `
		SELECT COALESCE(MAX(n),0)::int
		FROM (
		  SELECT collected_at,
		         COUNT(*) FILTER (WHERE LOWER(COALESCE(state,'')) = 'idle in transaction'
		                          AND state_change IS NOT NULL
		                          AND (collected_at - state_change) > INTERVAL '30 seconds') AS n
		  FROM monitor.pg_session_snapshot
		  WHERE server_id = $1 AND collected_at >= $2
		  GROUP BY collected_at
		) x
	`
	if err := tl.pool.QueryRow(ctx, qIdle, serverID, from).Scan(&k.IdleInTxnRiskCount); err != nil {
		return nil, err
	}

	// Latest active incident, if any.
	qInc := `
		SELECT incident_id, started_at, root_blocker_pid, COALESCE(root_blocker_query,'')
		FROM monitor.pg_blocking_incident
		WHERE server_id = $1 AND status = 'active'
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

	// Chain depth: approximate from most recent pairs in last 2 minutes.
	// This runs in SQL via recursive traversal; bounded to prevent runaway.
	qDepth := `
		WITH latest AS (
		  SELECT collected_at
		  FROM monitor.pg_blocking_pairs
		  WHERE server_id = $1
		  ORDER BY collected_at DESC
		  LIMIT 1
		),
		edges AS (
		  SELECT blocking_pid, blocked_pid
		  FROM monitor.pg_blocking_pairs p
		  JOIN latest l ON p.collected_at = l.collected_at
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
	// Defensive check: if tables are missing, callers get clearer error.
	// This is intentionally lightweight (no DDL), since schema is managed by sql_scripts.
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
		return fmt.Errorf("monitor.pg_blocking_incident not found (run infrastructure/sql_scripts/00_timescale_schema.sql)")
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
	PID        int               `json:"pid"`
	User       string            `json:"user,omitempty"`
	Database   string            `json:"database,omitempty"`
	State      string            `json:"state"`
	QueryStart *time.Time        `json:"query_start,omitempty"`
	Duration   string            `json:"duration"`
	WaitEvent  string            `json:"wait_event"`
	Query      string            `json:"query"`
	BlockedBy  []PgBlockingNodeAt `json:"blocked_by"`
}

type PgBlockingDetailsResponse struct {
	CollectedAt time.Time           `json:"collected_at"`
	ServerID    string              `json:"server_id"`
	BlockingTree []PgBlockingNodeAt  `json:"blocking_tree"`
}

// GetPgBlockingDetailsInRange picks the latest capture within [from,to] and reconstructs a blocking tree
// using monitor.pg_blocking_pairs + monitor.pg_session_snapshot at that capture timestamp.
func (tl *TimescaleLogger) GetPgBlockingDetailsInRange(ctx context.Context, serverID string, from, to time.Time) (*PgBlockingDetailsResponse, error) {
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("from/to required")
	}
	if to.Before(from) {
		return nil, fmt.Errorf("to must be >= from")
	}

	// Find the most recent capture that has any blocking pairs in the selected range.
	var at time.Time
	err := tl.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(collected_at), 'epoch'::timestamptz)
		FROM monitor.pg_blocking_pairs
		WHERE server_id = $1
		  AND collected_at >= $2
		  AND collected_at <= $3
	`, serverID, from, to).Scan(&at)
	if err != nil {
		return nil, err
	}
	if at.Equal(time.Unix(0, 0).UTC()) {
		return &PgBlockingDetailsResponse{CollectedAt: at, ServerID: serverID, BlockingTree: []PgBlockingNodeAt{}}, nil
	}

	// Load all pairs at that capture time.
	type pair struct{ blocked, blocking int }
	pairRows, err := tl.pool.Query(ctx, `
		SELECT blocked_pid, blocking_pid
		FROM monitor.pg_blocking_pairs
		WHERE server_id = $1 AND collected_at = $2
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
		return &PgBlockingDetailsResponse{CollectedAt: at, ServerID: serverID, BlockingTree: []PgBlockingNodeAt{}}, nil
	}

	// Fetch session snapshots for involved pids at same collected_at.
	sRows, err := tl.pool.Query(ctx, `
		SELECT pid, COALESCE(usename,''), COALESCE(datname,''), COALESCE(state,''),
		       query_start, state_change,
		       COALESCE(wait_event_type,''), COALESCE(wait_event,''),
		       COALESCE(LEFT(query,400),'')
		FROM monitor.pg_session_snapshot
		WHERE server_id = $1 AND collected_at = $2 AND pid = ANY($3)
	`, serverID, at, intSliceKeys(pidSet))
	if err != nil {
		return nil, err
	}
	defer sRows.Close()

	nodeMap := make(map[int]PgBlockingNodeAt, len(pidSet))
	for sRows.Next() {
		var pid int
		var us, dn, st, wet, we, q string
		var qs *time.Time
		var sc *time.Time
		if err := sRows.Scan(&pid, &us, &dn, &st, &qs, &sc, &wet, &we, &q); err != nil {
			continue
		}
		wait := ""
		if wet != "" || we != "" {
			if wet != "" && we != "" {
				wait = wet + ":" + we
			} else if wet != "" {
				wait = wet
			} else {
				wait = we
			}
		}
		dur := "-"
		if sc != nil {
			sec := int(at.Sub(sc.UTC()).Seconds())
			if sec < 0 {
				sec = 0
			}
			m := sec / 60
			s := sec % 60
			dur = fmt.Sprintf("%dm %ds", m, s)
		}
		nodeMap[pid] = PgBlockingNodeAt{
			PID:        pid,
			User:       us,
			Database:   dn,
			State:      st,
			QueryStart: qs,
			Duration:   dur,
			WaitEvent:  wait,
			Query:      q,
		}
	}
	// Add placeholders for pids missing in snapshot (race/permissions).
	for pid := range pidSet {
		if _, ok := nodeMap[pid]; ok {
			continue
		}
		nodeMap[pid] = PgBlockingNodeAt{PID: pid, State: "unknown", Duration: "-", WaitEvent: "", Query: ""}
	}

	// Build children adjacency and roots.
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
		CollectedAt:  at,
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

