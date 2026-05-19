// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL lock analysis and hierarchical blocking detection (STRICT STAGING ONLY).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// PgLock represents a PostgreSQL lock with metadata.
type PgLock struct {
	PID         int     `json:"pid"`
	LockType    string  `json:"lock_type"`
	Relation    string  `json:"relation"`
	Mode        string  `json:"mode"`
	Granted     bool    `json:"granted"`
	WaitSeconds float64 `json:"waiting_seconds"`
	WaitingFor  string  `json:"waiting_for"`
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

// GetLocks returns all current locks from optimized staging snapshots.
// STRICT: Never queries production servers directly.
func (c *PgRepository) GetLocks(instanceName string) ([]PgLock, error) {
	if c.LocalPool == nil {
		return nil, fmt.Errorf("datasource error: TimescaleDB is not connected")
	}

	serverID, ok := c.GetServerID(instanceName)
	if !ok {
		return nil, fmt.Errorf("instance %s not found in registry", instanceName)
	}

	query := `
		/* SQL_OPTIMA STAGING ONLY */ SELECT   
			pid, locktype, table_name as relation, mode, granted,
			extract(epoch from query_duration) as waiting_seconds
		FROM staging.postgres_activity_locks_raw
		WHERE server_id = $1
		  AND capture_timestamp = (SELECT max(capture_timestamp) FROM staging.postgres_activity_locks_raw WHERE server_id = $1)
		ORDER BY granted, waiting_seconds DESC
		LIMIT 100`

	rows, err := c.LocalPool.Query(context.Background(), query, serverID)
	if err != nil {
		return nil, fmt.Errorf("staging read error: %w", err)
	}
	defer rows.Close()

	var locks []PgLock
	for rows.Next() {
		var l PgLock
		if err := rows.Scan(&l.PID, &l.LockType, &l.Relation, &l.Mode, &l.Granted, &l.WaitSeconds); err == nil {
			if !l.Granted && l.WaitSeconds > 0 {
				d := time.Duration(l.WaitSeconds) * time.Second
				l.WaitingFor = fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
			} else {
				l.WaitingFor = "-"
			}
			locks = append(locks, l)
		}
	}

	if len(locks) == 0 {
		return nil, fmt.Errorf("no recent lock data found for %s", instanceName)
	}

	return locks, nil
}

// GetBlockingTree returns hierarchical blocking relationships from staging data.
// STRICT: Never queries production servers directly.
func (c *PgRepository) GetBlockingTree(instanceName string) ([]PgBlockingNode, error) {
	if c.LocalPool == nil {
		return nil, fmt.Errorf("datasource error: TimescaleDB is not connected")
	}

	serverID, ok := c.GetServerID(instanceName)
	if !ok {
		return nil, fmt.Errorf("instance %s not found in registry", instanceName)
	}

	query := `
		/* SQL_OPTIMA STAGING ONLY */ SELECT   
			pid, usename, datname, application_name, client_addr, state,
			query_start, xact_start, extract(epoch from query_duration) as duration_sec,
			wait_event_type, wait_event, query, blocking_pids
		FROM staging.postgres_activity_locks_raw
		WHERE server_id = $1
		  AND capture_timestamp = (SELECT max(capture_timestamp) FROM staging.postgres_activity_locks_raw WHERE server_id = $1)`

	rows, err := c.LocalPool.Query(context.Background(), query, serverID)
	if err != nil {
		return nil, fmt.Errorf("staging read error: %w", err)
	}
	defer rows.Close()

	sessionMap := make(map[int]*PgBlockingNode)
	type pair struct{ blocked, blocking int }
	var pairs []pair

	for rows.Next() {
		var pid int
		var user, db, app, addr, state, query, waitET, waitE string
		var qStart, xStart time.Time
		var qStartPtr, xStartPtr *time.Time
		var durationSec float64
		var bPids []int64

		if err := rows.Scan(
			&pid, &user, &db, &app, &addr, &state,
			&qStartPtr, &xStartPtr, &durationSec,
			&waitET, &waitE, &query, pq.Array(&bPids),
		); err != nil {
			continue
		}

		node := &PgBlockingNode{
			PID:             pid,
			User:            user,
			Database:        db,
			ApplicationName: app,
			ClientAddr:      addr,
			State:           state,
			IsIdleInTxn:     state == "idle in transaction",
			WaitEventType:   waitET,
			WaitEvent:       waitE,
			Query:           query,
			BlockedBy:       []PgBlockingNode{},
		}
		if qStartPtr != nil {
			qStart = *qStartPtr
			node.QueryStart = &qStart
		}
		if xStartPtr != nil {
			xStart = *xStartPtr
			node.XactStart = &xStart
		}
		node.Duration = fmt.Sprintf("%.0fs", durationSec)

		sessionMap[pid] = node
		for _, bpid := range bPids {
			pairs = append(pairs, pair{blocked: pid, blocking: int(bpid)})
		}
	}

	if len(sessionMap) == 0 {
		return nil, fmt.Errorf("no recent blocking data received for %s", instanceName)
	}

	roots := make(map[int]struct{})
	for pid := range sessionMap {
		roots[pid] = struct{}{}
	}
	for _, p := range pairs {
		if _, ok := sessionMap[p.blocked]; ok {
			delete(roots, p.blocked)
		}
	}

	var build func(int) PgBlockingNode
	build = func(pid int) PgBlockingNode {
		n := *sessionMap[pid]
		for _, p := range pairs {
			if p.blocking == pid {
				n.BlockedBy = append(n.BlockedBy, build(p.blocked))
			}
		}
		return n
	}

	var out []PgBlockingNode
	for pid := range roots {
		if node := build(pid); len(node.BlockedBy) > 0 {
			out = append(out, node)
		}
	}
	return out, nil
}

func (c *PgRepository) GetBlockingTreeFast(instanceName string) ([]PgBlockingNode, error) {
	return c.GetBlockingTree(instanceName)
}
