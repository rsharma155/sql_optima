// Package repository provides data access layer for database operations.
// It handles connections and queries for both PostgreSQL and SQL Server databases.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL lock analysis and hierarchical blocking detection.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// buildINClause constructs a SQL "IN ($1,$2,...)" placeholder string and args slice
// for a slice of int PIDs starting at offset (1-based).
func buildINClause(pids []int, offset int) (string, []any) {
	ph := make([]string, len(pids))
	args := make([]any, len(pids))
	for i, pid := range pids {
		ph[i] = fmt.Sprintf("$%d", offset+i)
		args[i] = pid
	}
	return strings.Join(ph, ","), args
}

// PgLock represents a PostgreSQL lock with metadata.
type PgLock struct {
	PID        int    `json:"pid"`
	LockType   string `json:"lock_type"`
	Relation   string `json:"relation"`
	Mode       string `json:"mode"`
	Granted    bool   `json:"granted"`
	WaitingFor string `json:"waiting_for"`
}

// GetLocks returns all current locks with waiting time for ungranted locks.
func (c *PgRepository) GetLocks(instanceName string) ([]PgLock, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		/* SQL_OPTIMA */ SELECT   
			l.pid,
			l.locktype,
			COALESCE(r.relname || ' (' || r.oid || ')', 'virtual') as relation,
			l.mode,
			l.granted,
			CASE WHEN l.granted = false THEN EXTRACT(EPOCH FROM (now() - a.state_change)) ELSE 0 END as waiting_seconds
		FROM pg_locks l
		LEFT JOIN pg_class r ON l.relation = r.oid
		LEFT JOIN pg_stat_activity a ON l.pid = a.pid
		WHERE l.pid <> pg_backend_pid()
		ORDER BY l.granted, waiting_seconds DESC
		LIMIT 100
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locks []PgLock
	for rows.Next() {
		var l PgLock
		var waitingSeconds float64
		err := rows.Scan(&l.PID, &l.LockType, &l.Relation, &l.Mode, &l.Granted, &waitingSeconds)
		if err != nil {
			continue
		}

		if !l.Granted && waitingSeconds > 0 {
			duration := time.Duration(waitingSeconds) * time.Second
			minutes := int(duration.Minutes())
			seconds := int(duration.Seconds()) % 60
			l.WaitingFor = fmt.Sprintf("%dm %ds", minutes, seconds)
		} else {
			l.WaitingFor = "-"
		}

		locks = append(locks, l)
	}

	return locks, nil
}

// PgBlockingNode represents a node in the blocking tree visualization.
type PgBlockingNode struct {
	PID               int              `json:"pid"`
	User              string           `json:"user,omitempty"`
	Database          string           `json:"database,omitempty"`
	ApplicationName   string           `json:"application_name,omitempty"`
	ClientAddr        string           `json:"client_addr,omitempty"`
	State             string           `json:"state"`
	IsIdleInTxn       bool             `json:"is_idle_in_txn"`
	QueryStart        *time.Time       `json:"query_start,omitempty"`
	XactStart         *time.Time       `json:"xact_start,omitempty"`
	Duration          string           `json:"duration"`
	WaitEventType     string           `json:"wait_event_type,omitempty"`
	WaitEvent         string           `json:"wait_event"`
	Query             string           `json:"query"`
	LockModes         []string         `json:"lock_modes,omitempty"`
	AffectedRelations []string         `json:"affected_relations,omitempty"`
	BlockedBy         []PgBlockingNode `json:"blocked_by"`
}

// GetBlockingTree returns hierarchical blocking relationships between sessions.
func (c *PgRepository) GetBlockingTree(instanceName string) ([]PgBlockingNode, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// Use pg_locks to discover blocked->blocking pairs.
	// This is more reliable than filtering pg_stat_activity by state and is less sensitive to
	// limited visibility of other sessions.
	pairsQ := `
		WITH waiting AS (
			/* SQL_OPTIMA */ SELECT   *
			FROM pg_locks
			WHERE NOT granted
		),
		holding AS (
			SELECT /* SQL_OPTIMA */   *
			FROM pg_locks
			WHERE granted
		)
		SELECT /* SQL_OPTIMA */  
			w.pid  AS blocked_pid,
			h.pid  AS blocking_pid
		FROM waiting w
		JOIN holding h
		  ON h.locktype IS NOT DISTINCT FROM w.locktype
		 AND h.database IS NOT DISTINCT FROM w.database
		 AND h.relation IS NOT DISTINCT FROM w.relation
		 AND h.page IS NOT DISTINCT FROM w.page
		 AND h.tuple IS NOT DISTINCT FROM w.tuple
		 AND h.virtualxid IS NOT DISTINCT FROM w.virtualxid
		 AND h.transactionid IS NOT DISTINCT FROM w.transactionid
		 AND h.classid IS NOT DISTINCT FROM w.classid
		 AND h.objid IS NOT DISTINCT FROM w.objid
		 AND h.objsubid IS NOT DISTINCT FROM w.objsubid
		 AND h.pid <> w.pid
	`
	pairRows, err := db.Query(pairsQ)
	if err != nil {
		return nil, err
	}
	defer pairRows.Close()

	type pair struct{ blocked, blocking int }
	var pairs []pair
	pidSet := make(map[int]struct{})
	for pairRows.Next() {
		var bpid, spid int
		if err := pairRows.Scan(&bpid, &spid); err != nil {
			continue
		}
		pairs = append(pairs, pair{blocked: bpid, blocking: spid})
		pidSet[bpid] = struct{}{}
		pidSet[spid] = struct{}{}
	}
	if err := pairRows.Err(); err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return []PgBlockingNode{}, nil
	}

	// Load pg_stat_activity rows for involved PIDs.
	var pidList []int
	for pid := range pidSet {
		pidList = append(pidList, pid)
	}

	// Build a dynamic IN clause safely.
	args := make([]any, 0, len(pidList))
	ph := make([]string, 0, len(pidList))
	for i, pid := range pidList {
		args = append(args, pid)
		ph = append(ph, fmt.Sprintf("$%d", i+1))
	}

	actQ := fmt.Sprintf(`
		/* SQL_OPTIMA */ SELECT   
			a.pid,
			a.usename,
			a.datname,
			a.state,
			a.query_start,
			EXTRACT(EPOCH FROM (now() - a.state_change)) as duration_seconds,
			COALESCE(a.wait_event_type || ':' || a.wait_event, '') as wait_event,
			LEFT(a.query, 400) as query
		FROM pg_stat_activity a
		WHERE a.pid IN (%s)
	`, strings.Join(ph, ","))

	actRows, err := db.Query(actQ, args...)
	if err != nil {
		return nil, err
	}
	defer actRows.Close()

	sessionMap := make(map[int]PgBlockingNode)
	for actRows.Next() {
		var node PgBlockingNode
		var durationSeconds float64
		if err := actRows.Scan(&node.PID, &node.User, &node.Database, &node.State, &node.QueryStart, &durationSeconds, &node.WaitEvent, &node.Query); err != nil {
			continue
		}
		duration := time.Duration(durationSeconds) * time.Second
		minutes := int(duration.Minutes())
		seconds := int(duration.Seconds()) % 60
		node.Duration = fmt.Sprintf("%dm %ds", minutes, seconds)
		sessionMap[node.PID] = node
	}

	// Ensure we keep placeholders for PIDs we couldn't read (permissions/race).
	for pid := range pidSet {
		if _, ok := sessionMap[pid]; ok {
			continue
		}
		sessionMap[pid] = PgBlockingNode{
			PID:       pid,
			State:     "unknown",
			Duration:  "-",
			WaitEvent: "",
			Query:     "",
		}
	}

	// Build parent->children map.
	roots := make(map[int]struct{})
	for _, p := range pairs {
		roots[p.blocked] = struct{}{}
	}
	for _, p := range pairs {
		blocker := sessionMap[p.blocking]
		blocked := sessionMap[p.blocked]
		blocker.BlockedBy = append(blocker.BlockedBy, blocked)
		sessionMap[p.blocking] = blocker
		delete(roots, p.blocked)
	}

	var out []PgBlockingNode
	for pid := range roots {
		out = append(out, sessionMap[pid])
	}
	return out, nil
}

// GetBlockingTreeFast returns a blocking tree using pg_blocking_pids(), which is the same signal used by
// the incident collector (monitor.pg_blocking_pairs). This is typically more consistent for "live" UI.
func (c *PgRepository) GetBlockingTreeFast(instanceName string) ([]PgBlockingNode, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// Build pairs directly from pg_blocking_pids().
	type pair struct{ blocked, blocking int }
	var pairs []pair
	pidSet := make(map[int]struct{})

	rows, err := db.Query(`
		/* SQL_OPTIMA */SELECT  a.pid AS blocked_pid, unnest(pg_blocking_pids(a.pid)) AS blocking_pid
		FROM pg_stat_activity a
		WHERE cardinality(pg_blocking_pids(a.pid)) > 0
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var bpid, spid int
		if err := rows.Scan(&bpid, &spid); err != nil {
			continue
		}
		pairs = append(pairs, pair{blocked: bpid, blocking: spid})
		pidSet[bpid] = struct{}{}
		pidSet[spid] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return []PgBlockingNode{}, nil
	}

	// Load pg_stat_activity rows for involved PIDs.
	var pidList []int
	for pid := range pidSet {
		pidList = append(pidList, pid)
	}

	inClause, args := buildINClause(pidList, 1)
	actQ := fmt.Sprintf(`
		/* SQL_OPTIMA */ SELECT
			a.pid,
			COALESCE(a.usename,''),
			COALESCE(a.datname,''),
			COALESCE(a.application_name,''),
			COALESCE(a.client_addr::text,''),
			COALESCE(a.state,''),
			a.xact_start,
			a.query_start,
			a.state_change,
			COALESCE(a.wait_event_type,'') AS wait_event_type,
			COALESCE(a.wait_event,'') AS wait_event,
			COALESCE(a.query,'') AS query
		FROM pg_stat_activity a
		WHERE a.pid IN (%s)
	`, inClause)

	actRows, err := db.Query(actQ, args...)
	if err != nil {
		return nil, err
	}
	defer actRows.Close()

	sessionMap := make(map[int]PgBlockingNode)
	for actRows.Next() {
		var node PgBlockingNode
		var sc *time.Time
		if err := actRows.Scan(
			&node.PID, &node.User, &node.Database, &node.ApplicationName, &node.ClientAddr,
			&node.State, &node.XactStart, &node.QueryStart, &sc,
			&node.WaitEventType, &node.WaitEvent, &node.Query,
		); err != nil {
			continue
		}
		if node.WaitEventType != "" && node.WaitEvent != "" {
			node.WaitEvent = node.WaitEventType + ":" + node.WaitEvent
		} else if node.WaitEventType != "" {
			node.WaitEvent = node.WaitEventType
		}
		node.IsIdleInTxn = strings.Contains(strings.ToLower(node.State), "idle in transaction")
		if sc != nil {
			sec := int(time.Since(*sc).Seconds())
			if sec < 0 {
				sec = 0
			}
			node.Duration = fmt.Sprintf("%dm %ds", sec/60, sec%60)
		} else {
			node.Duration = "-"
		}
		sessionMap[node.PID] = node
	}

	// Fetch lock modes and relations per PID from pg_locks.
	lockInClause, lockArgs := buildINClause(pidList, 1)
	lockQ := fmt.Sprintf(`
		/* SQL_OPTIMA */ SELECT l.pid, COALESCE(l.mode,''), COALESCE(r.relname,'virtual')
		FROM pg_locks l
		LEFT JOIN pg_class r ON l.relation = r.oid
		WHERE l.pid IN (%s)
	`, lockInClause)
	lockRows, lockErr := db.Query(lockQ, lockArgs...)
	if lockErr == nil {
		defer lockRows.Close()
		modeSets := make(map[int]map[string]struct{})
		relSets := make(map[int]map[string]struct{})
		for lockRows.Next() {
			var pid int
			var mode, rel string
			if err := lockRows.Scan(&pid, &mode, &rel); err != nil {
				continue
			}
			if modeSets[pid] == nil {
				modeSets[pid] = make(map[string]struct{})
			}
			if relSets[pid] == nil {
				relSets[pid] = make(map[string]struct{})
			}
			if mode != "" {
				modeSets[pid][mode] = struct{}{}
			}
			if rel != "" && rel != "virtual" {
				relSets[pid][rel] = struct{}{}
			}
		}
		for pid, node := range sessionMap {
			if ms := modeSets[pid]; len(ms) > 0 {
				for m := range ms {
					node.LockModes = append(node.LockModes, m)
				}
				sort.Strings(node.LockModes)
			}
			if rs := relSets[pid]; len(rs) > 0 {
				for r := range rs {
					node.AffectedRelations = append(node.AffectedRelations, r)
				}
				sort.Strings(node.AffectedRelations)
			}
			sessionMap[pid] = node
		}
	}

	for pid := range pidSet {
		if _, ok := sessionMap[pid]; ok {
			continue
		}
		sessionMap[pid] = PgBlockingNode{
			PID:       pid,
			State:     "unknown",
			Duration:  "-",
			WaitEvent: "",
			Query:     "",
		}
	}

	// Determine roots: blockers that are not blocked.
	blockedSet := make(map[int]struct{})
	blockerSet := make(map[int]struct{})
	for _, p := range pairs {
		blockedSet[p.blocked] = struct{}{}
		blockerSet[p.blocking] = struct{}{}
	}
	var rootPIDs []int
	for pid := range blockerSet {
		if _, isBlocked := blockedSet[pid]; !isBlocked {
			rootPIDs = append(rootPIDs, pid)
		}
	}
	sort.Ints(rootPIDs)

	children := make(map[int][]int)
	for _, p := range pairs {
		children[p.blocking] = append(children[p.blocking], p.blocked)
	}

	var build func(pid int, seen map[int]struct{}) PgBlockingNode
	build = func(pid int, seen map[int]struct{}) PgBlockingNode {
		n := sessionMap[pid]
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

	out := make([]PgBlockingNode, 0, len(rootPIDs))
	for _, pid := range rootPIDs {
		out = append(out, build(pid, map[int]struct{}{}))
	}
	return out, nil
}
