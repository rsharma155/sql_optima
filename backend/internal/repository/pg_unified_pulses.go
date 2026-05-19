// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unified "Mega-Query" pulse methods for PostgreSQL to consolidate DMV hits.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// PgTier1Row represents a combined activity and lock snapshot for Postgres
type PgTier1Row struct {
	PID                 int
	DatName             string
	UserName            string
	AppName             string
	ClientAddr          sql.NullString
	BackendType         string
	State               string
	WaitEventType       sql.NullString
	WaitEvent           sql.NullString
	QueryStart          sql.NullTime
	XactStart           sql.NullTime
	StateChange         sql.NullTime
	QueryDuration       sql.NullString // Interval as string
	TransactionDuration sql.NullString // Interval as string
	BlockingPids        []int64        // Array mapped to INT[]
	LockType            sql.NullString
	LockMode            sql.NullString
	LockGranted         sql.NullBool
	TableName           sql.NullString
	Query               string
	CaptureTimestamp    time.Time
}

// FetchPgTier1MegaQuery executes the consolidated activity/locks sweep (T1)
func (c *PgRepository) FetchPgTier1MegaQuery(ctx context.Context, instanceName string) ([]PgTier1Row, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
	/* SQL_OPTIMA PULSE T1 */
	SELECT
		a.pid, a.datname, a.usename, a.application_name, a.client_addr, a.backend_type,
		a.state, a.wait_event_type, a.wait_event, a.query_start, a.xact_start,
		a.state_change, (now() - a.query_start)::text, (now() - a.xact_start)::text,
		pg_blocking_pids(a.pid), l.locktype, l.mode, l.granted, 
		l.relation::regclass::text, a.query, now()
	FROM pg_stat_activity a
	LEFT JOIN pg_locks l ON a.pid = l.pid
	WHERE a.pid <> pg_backend_pid()
	  AND a.backend_type = 'client backend'
	  AND a.datname NOT IN ('postgres', 'template0', 'template1')
	  AND a.state IN ('active', 'idle in transaction');`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []PgTier1Row
	for rows.Next() {
		var r PgTier1Row
		err := rows.Scan(
			&r.PID, &r.DatName, &r.UserName, &r.AppName, &r.ClientAddr, &r.BackendType,
			&r.State, &r.WaitEventType, &r.WaitEvent, &r.QueryStart, &r.XactStart,
			&r.StateChange, &r.QueryDuration, &r.TransactionDuration, pq.Array(&r.BlockingPids),
			&r.LockType, &r.LockMode, &r.LockGranted, &r.TableName, &r.Query, &r.CaptureTimestamp,
		)
		if err == nil {
			results = append(results, r)
		}
	}
	return results, rows.Err()
}
