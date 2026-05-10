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
	"github.com/rsharma155/sql_optima/internal/domain/postgres_observability/domain/entities"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_observability/domain/repositories"
	"sync"
	"time"
)

var (
	// In-memory state to track last seen values for delta calculation and deduplication
	lastSessionState = make(map[string]map[int]entities.SessionActivity) // instanceID -> pid -> activity
	lastQueryStats   = make(map[string]map[int64]entities.QueryWaitProfile) // instanceID -> queryID -> stats
	stateMu         sync.Mutex
)

type PostgresObservabilityCollector struct {
	repo       *repositories.PostgresObservabilityRepository
	instanceID string
}

func NewPostgresObservabilityCollector(repo *repositories.PostgresObservabilityRepository, instanceID string) *PostgresObservabilityCollector {
	return &PostgresObservabilityCollector{
		repo:       repo,
		instanceID: instanceID,
	}
}

func (c *PostgresObservabilityCollector) CollectSessionActivity(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT
			datname, pid, usename, application_name, client_addr, state,
			wait_event_type, wait_event, backend_type, query_id, query,
			xact_start, query_start, state_change, backend_start
		FROM pg_stat_activity
		WHERE pid <> pg_backend_pid()
		  AND backend_type = 'client backend'
		  AND (query IS NULL OR query NOT LIKE '%/* SQL_OPTIMA */%')`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var activities []entities.SessionActivity
	now := time.Now().UTC()
	
	stateMu.Lock()
	if lastSessionState[c.instanceID] == nil {
		lastSessionState[c.instanceID] = make(map[int]entities.SessionActivity)
	}
	currentInstanceSessions := lastSessionState[c.instanceID]
	stateMu.Unlock()

	newSessionState := make(map[int]entities.SessionActivity)

	for rows.Next() {
		var a entities.SessionActivity
		a.TS = now
		a.InstanceID = c.instanceID
		
		var dbName, username, appName, clientAddr, state, waitType, waitEvent, backendType, query sql.NullString
		var queryID sql.NullInt64
		var xactStart, queryStart, stateChange, backendStart sql.NullTime

		err := rows.Scan(
			&dbName, &a.PID, &username, &appName, &clientAddr, &state,
			&waitType, &waitEvent, &backendType, &queryID, &query,
			&xactStart, &queryStart, &stateChange, &backendStart)
		if err != nil {
			return err
		}
		
		a.DBName = dbName.String
		a.Username = username.String
		a.ApplicationName = appName.String
		a.ClientAddr = clientAddr.String
		a.State = state.String
		a.WaitEventType = waitType.String
		a.WaitEvent = waitEvent.String
		a.BackendType = backendType.String
		a.QueryID = queryID.Int64
		a.Query = query.String
		
		if xactStart.Valid { a.XactStart = &xactStart.Time }
		if queryStart.Valid { a.QueryStart = &queryStart.Time }
		if stateChange.Valid { a.StateChange = &stateChange.Time }
		if backendStart.Valid { a.BackendStart = &backendStart.Time }

		newSessionState[a.PID] = a

		// Deduplication Logic:
		// Only save if:
		// 1. Session is new
		// 2. State, Wait Event, or QueryID changed
		// 3. Last recorded sample is older than 5 minutes (heartbeat)
		last, exists := currentInstanceSessions[a.PID]
		shouldSave := !exists || 
		              last.State != a.State || 
					  last.WaitEvent != a.WaitEvent || 
					  last.QueryID != a.QueryID ||
					  now.Sub(last.TS) > 5*time.Minute

		if shouldSave {
			activities = append(activities, a)
		}
	}

	stateMu.Lock()
	lastSessionState[c.instanceID] = newSessionState
	stateMu.Unlock()

	if len(activities) == 0 {
		return nil
	}
	return c.repo.SaveSessionActivity(ctx, activities)
}

func (c *PostgresObservabilityCollector) CollectWaitSummary(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT
			wait_event_type, wait_event, count(*)
		FROM pg_stat_activity
		WHERE wait_event IS NOT NULL
		  AND backend_type = 'client backend'
		GROUP BY wait_event_type, wait_event`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var summaries []entities.WaitEventSummary
	now := time.Now().UTC()
	for rows.Next() {
		var s entities.WaitEventSummary
		s.TS = now
		s.InstanceID = c.instanceID
		err := rows.Scan(&s.WaitEventType, &s.WaitEvent, &s.Sessions)
		if err != nil {
			return err
		}
		summaries = append(summaries, s)
	}

	return c.repo.SaveWaitEventSummary(ctx, summaries)
}

func (c *PostgresObservabilityCollector) CollectDBLoad(ctx context.Context, db *sql.DB) error {
	var load entities.DBLoad
	load.TS = time.Now().UTC()
	load.InstanceID = c.instanceID

	err := db.QueryRowContext(ctx, `
		SELECT
			count(*) FILTER (WHERE state='active'),
			count(*) FILTER (WHERE state='active' AND wait_event IS NULL),
			count(*) FILTER (WHERE wait_event IS NOT NULL),
			count(*) FILTER (WHERE wait_event_type IN ('IO', 'DataFile', 'WAL', 'BufferPin')),
			count(*) FILTER (WHERE wait_event_type = 'Lock'),
			count(*) FILTER (WHERE state='idle in transaction')
		FROM pg_stat_activity
		WHERE backend_type = 'client backend'`).Scan(&load.ActiveSessions, &load.CPUSessions, &load.WaitingSessions, &load.IOWaitSessions, &load.LockWaitSessions, &load.IdleInTxn)
	if err != nil {
		return err
	}

	return c.repo.SaveDBLoad(ctx, load)
}

func (c *PostgresObservabilityCollector) CollectQueryWaitProfile(ctx context.Context, db *sql.DB) error {
	// Check if pg_stat_statements is enabled
	var exists bool
	err := db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&exists)
	if err != nil || !exists {
		return nil // Skip if extension not present
	}

	rows, err := db.QueryContext(ctx, `
		SELECT
			s.queryid, s.calls, s.total_exec_time, s.mean_exec_time, s.rows,
			s.shared_blks_hit, s.shared_blks_read, s.temp_blks_written,
			s.query, r.rolname
		FROM pg_stat_statements s
		LEFT JOIN pg_roles r ON s.userid = r.oid
		WHERE s.query NOT LIKE '%/* SQL_OPTIMA */%'
		ORDER BY s.total_exec_time DESC
		LIMIT 50`) // Increased limit to find more potential deltas
	if err != nil {
		return err
	}
	defer rows.Close()

	var deltas []entities.QueryWaitProfile
	now := time.Now().UTC()
	
	stateMu.Lock()
	if lastQueryStats[c.instanceID] == nil {
		lastQueryStats[c.instanceID] = make(map[int64]entities.QueryWaitProfile)
	}
	currentInstanceStats := lastQueryStats[c.instanceID]
	stateMu.Unlock()

	for rows.Next() {
		var p entities.QueryWaitProfile
		p.TS = now
		p.InstanceID = c.instanceID
		
		var query, username sql.NullString
		err := rows.Scan(
			&p.QueryID, &p.Calls, &p.TotalExecTime, &p.MeanExecTime, &p.Rows,
			&p.SharedBlksHit, &p.SharedBlksRead, &p.TempBlksWritten,
			&query, &username)
		if err != nil {
			return err
		}
		
		p.Query = query.String
		p.Username = username.String
		
		last, exists := currentInstanceStats[p.QueryID]
		if exists {
			// Calculate Delta
			deltaCalls := p.Calls - last.Calls
			if deltaCalls > 0 {
				d := p
				d.Calls = deltaCalls
				d.TotalExecTime = p.TotalExecTime - last.TotalExecTime
				d.Rows = p.Rows - last.Rows
				d.SharedBlksHit = p.SharedBlksHit - last.SharedBlksHit
				d.SharedBlksRead = p.SharedBlksRead - last.SharedBlksRead
				d.TempBlksWritten = p.TempBlksWritten - last.TempBlksWritten
				
				// Ensure no negative deltas (can happen if pg_stat_statements is reset)
				if d.TotalExecTime < 0 { d.TotalExecTime = 0 }
				if d.Rows < 0 { d.Rows = 0 }
				
				if d.Calls > 0 {
					d.MeanExecTime = d.TotalExecTime / float64(d.Calls)
				}
				
				deltas = append(deltas, d)
			}
		} else {
			// First time seeing this query, don't store delta yet as we don't know the baseline
			// Alternatively, store it as is, but deltas are cleaner.
		}
		
		// Update state for next run
		stateMu.Lock()
		lastQueryStats[c.instanceID][p.QueryID] = p
		stateMu.Unlock()
	}

	if len(deltas) == 0 {
		return nil
	}
	return c.repo.SaveQueryWaitProfile(ctx, deltas)
}
