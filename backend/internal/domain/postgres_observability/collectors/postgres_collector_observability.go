// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Observability data collectors.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_observability/domain/entities"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_observability/domain/repositories"
	"sync"
	"time"
)

var (
	// In-memory state to track last seen values for delta calculation and deduplication
	lastSessionState = make(map[uuid.UUID]map[int]entities.SessionActivity)    // serverID -> pid -> activity
	lastQueryStats   = make(map[uuid.UUID]map[int64]entities.QueryWaitProfile) // serverID -> queryID -> stats
	stateMu          sync.Mutex
)

type PostgresObservabilityCollector struct {
	repo *repositories.PostgresObservabilityRepository
}

func NewPostgresObservabilityCollector(repo *repositories.PostgresObservabilityRepository) *PostgresObservabilityCollector {
	return &PostgresObservabilityCollector{
		repo: repo,
	}
}

func (c *PostgresObservabilityCollector) CollectSessionActivity(ctx context.Context, serverID uuid.UUID, db *sql.DB) error {
	// query_id in pg_stat_activity is only available in PostgreSQL 14+.
	var pgVersionNum int
	if err := db.QueryRowContext(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&pgVersionNum); err != nil {
		pgVersionNum = 130000 // assume PG13 on failure to be safe
	}

	queryIDExpr := `COALESCE(query_id, 0)`
	if pgVersionNum < 140000 {
		queryIDExpr = `0`
	}

	querySQL := `
		SELECT
			COALESCE(datname, ''), pid, COALESCE(usename, ''), COALESCE(application_name, ''), client_addr, COALESCE(state, ''),
			COALESCE(wait_event_type, ''), COALESCE(wait_event, ''), COALESCE(backend_type, ''), ` + queryIDExpr + ` AS query_id, COALESCE(query, ''),
			xact_start, query_start, state_change, backend_start,
			pg_blocking_pids(pid) as blocking_pids,
			now() - query_start as query_duration
		FROM pg_stat_activity
		WHERE pid <> pg_backend_pid()
		  AND (query IS NULL OR query NOT LIKE '%/* SQL_OPTIMA */%')`

	rows, err := db.QueryContext(ctx, querySQL)
	if err != nil {
		return err
	}
	defer rows.Close()

	var activities []entities.SessionActivity
	now := time.Now().UTC()

	stateMu.Lock()
	if _, ok := lastSessionState[serverID]; !ok {
		lastSessionState[serverID] = make(map[int]entities.SessionActivity)
	}
	instanceState := lastSessionState[serverID]
	stateMu.Unlock()

	for rows.Next() {
		var a entities.SessionActivity
		var clientAddr sql.NullString
		var blockingPids []int64
		var queryDuration sql.NullString // using string for interval for now, or just ignore if not needed forentities
		err := rows.Scan(
			&a.DBName, &a.PID, &a.Username, &a.ApplicationName, &clientAddr, &a.State,
			&a.WaitEventType, &a.WaitEvent, &a.BackendType, &a.QueryID, &a.Query,
			&a.XactStart, &a.QueryStart, &a.StateChange, &a.BackendStart,
			pq.Array(&blockingPids), &queryDuration,
		)
		if err != nil {
			return err
		}
		a.TS = now
		a.ServerID = serverID
		if clientAddr.Valid {
			a.ClientAddr = clientAddr.String
		}
		a.BlockingPids = blockingPids

		// Simple deduplication: only log if state changed or query changed or blocking changed
		stateMu.Lock()
		prev, ok := instanceState[a.PID]
		if !ok || prev.State != a.State || prev.Query != a.Query || prev.WaitEvent != a.WaitEvent || !equalPids(prev.BlockingPids, a.BlockingPids) {
			activities = append(activities, a)
			instanceState[a.PID] = a
		}
		stateMu.Unlock()
	}

	return c.repo.SaveSessionActivity(ctx, activities)
}

func equalPids(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (c *PostgresObservabilityCollector) CollectWaitEventSummary(ctx context.Context, serverID uuid.UUID, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT COALESCE(wait_event_type, ''), COALESCE(wait_event, ''), count(*)
		FROM pg_stat_activity
		WHERE wait_event IS NOT NULL
		GROUP BY 1, 2`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var summaries []entities.WaitEventSummary
	now := time.Now().UTC()
	for rows.Next() {
		var s entities.WaitEventSummary
		err := rows.Scan(&s.WaitEventType, &s.WaitEvent, &s.Sessions)
		if err != nil {
			return err
		}
		s.TS = now
		s.ServerID = serverID
		summaries = append(summaries, s)
	}

	return c.repo.SaveWaitEventSummary(ctx, summaries)
}

func (c *PostgresObservabilityCollector) CollectDBLoad(ctx context.Context, serverID uuid.UUID, db *sql.DB) error {
	var load entities.DBLoad
	load.TS = time.Now().UTC()
	load.ServerID = serverID

	err := db.QueryRowContext(ctx, `
		SELECT
			count(*) FILTER (WHERE state = 'active') as active,
			count(*) FILTER (WHERE wait_event_type = 'CPU') as cpu,
			count(*) FILTER (WHERE wait_event IS NOT NULL) as waiting,
			count(*) FILTER (WHERE wait_event_type = 'IO') as io,
			count(*) FILTER (WHERE wait_event_type = 'Lock') as lock,
			count(*) FILTER (WHERE state = 'idle in transaction') as idle_in_txn
		FROM pg_stat_activity
		WHERE backend_type = 'client backend'`).Scan(
		&load.ActiveSessions, &load.CPUSessions, &load.WaitingSessions,
		&load.IOWaitSessions, &load.LockWaitSessions, &load.IdleInTxn,
	)
	if err != nil {
		return err
	}

	return c.repo.SaveDBLoad(ctx, load)
}

func (c *PostgresObservabilityCollector) CollectQueryWaitProfile(ctx context.Context, serverID uuid.UUID, db *sql.DB) error {
	// Only if pg_stat_statements exists
	rows, err := db.QueryContext(ctx, `
		SELECT queryid, calls, total_exec_time, mean_exec_time, rows, 
		       shared_blks_hit, shared_blks_read, temp_blks_written, query, usename
		FROM pg_stat_statements
		JOIN pg_authid ON pg_stat_statements.userid = pg_authid.oid
		ORDER BY total_exec_time DESC
		LIMIT 50`)
	if err != nil {
		// Might not be installed, skip silently or log once
		return nil
	}
	defer rows.Close()

	var profiles []entities.QueryWaitProfile
	now := time.Now().UTC()

	stateMu.Lock()
	if _, ok := lastQueryStats[serverID]; !ok {
		lastQueryStats[serverID] = make(map[int64]entities.QueryWaitProfile)
	}
	instanceStats := lastQueryStats[serverID]
	stateMu.Unlock()

	for rows.Next() {
		var p entities.QueryWaitProfile
		err := rows.Scan(
			&p.QueryID, &p.Calls, &p.TotalExecTime, &p.MeanExecTime, &p.Rows,
			&p.SharedBlksHit, &p.SharedBlksRead, &p.TempBlksWritten, &p.Query, &p.Username,
		)
		if err != nil {
			return err
		}
		p.TS = now
		p.ServerID = serverID

		// Delta calculation
		stateMu.Lock()
		prev, ok := instanceStats[p.QueryID]
		if ok {
			delta := p
			delta.Calls = p.Calls - prev.Calls
			delta.TotalExecTime = p.TotalExecTime - prev.TotalExecTime
			delta.Rows = p.Rows - prev.Rows
			delta.SharedBlksHit = p.SharedBlksHit - prev.SharedBlksHit
			delta.SharedBlksRead = p.SharedBlksRead - prev.SharedBlksRead
			delta.TempBlksWritten = p.TempBlksWritten - prev.TempBlksWritten

			if delta.Calls > 0 {
				profiles = append(profiles, delta)
			}
		} else {
			// First time seeing this query, log full stats as starting point
			profiles = append(profiles, p)
		}
		instanceStats[p.QueryID] = p
		stateMu.Unlock()
	}

	return c.repo.SaveQueryWaitProfile(ctx, profiles)
}

func (c *PostgresObservabilityCollector) CleanupStaleState(serverID uuid.UUID) {
	stateMu.Lock()
	defer stateMu.Unlock()
	delete(lastSessionState, serverID)
	delete(lastQueryStats, serverID)
}
