// Package repository provides data access layer for database operations.
// It handles connections and queries for both PostgreSQL and SQL Server databases.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL session monitoring and active query analysis.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/rsharma155/sql_optima/internal/models"
)

// GetConnectionStats returns active, idle, and total connection counts for a PostgreSQL instance.
// Used for connection pool monitoring and capacity planning.
func (c *PgRepository) GetConnectionStats(instanceName string) (active int, idle int, total int, err error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("[POSTGRES] GetConnectionStats: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return 0, 0, 0, fmt.Errorf("connection not found after reconnect")
			}
		} else {
			return 0, 0, 0, fmt.Errorf("connection not found")
		}
	}

	query := `
		SELECT /* SQL_OPTIMA */   
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE state = 'active') as active,
			COUNT(*) FILTER (WHERE state = 'idle') as idle
		FROM pg_stat_activity
		WHERE datname IS NOT NULL
	`
	err = db.QueryRow(query).Scan(&total, &active, &idle)
	return
}

// GetLongRunningQueries returns queries running longer than specified duration.
// Used to identify problematic long-running queries.
func (c *PgRepository) GetLongRunningQueries(instanceName string, minDurationSeconds int) ([]models.PgSession, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		/* SQL_OPTIMA */ SELECT  
			pid,
			COALESCE(usename::text, '') AS usename,
			COALESCE(datname::text, '') AS datname,
			COALESCE(client_addr::text, '') AS client_addr,
			client_port,
			backend_start,
			query_start,
			state_change,
			wait_event_type,
			wait_event,
			state,
			query,
			extract(epoch FROM (now() - COALESCE(query_start, xact_start))) * 1000 as duration_ms
		FROM pg_stat_activity
		WHERE pid <> pg_backend_pid()
		AND state = 'active'
		AND COALESCE(query_start, xact_start) IS NOT NULL
		AND extract(epoch FROM (now() - COALESCE(query_start, xact_start))) > $1
		AND query NOT ILIKE '%pg_stat_activity%'
		AND query NOT ILIKE 'autovacuum:%'
		ORDER BY COALESCE(query_start, xact_start) ASC NULLS LAST
		LIMIT 50
	`

	rows, err := db.Query(query, minDurationSeconds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.PgSession
	for rows.Next() {
		var s models.PgSession
		err := rows.Scan(
			&s.PID,
			&s.UserName,
			&s.Database,
			&s.ClientAddr,
			&s.ClientPort,
			&s.BackendStart,
			&s.QueryStart,
			&s.StateChange,
			&s.WaitEventType,
			&s.WaitEvent,
			&s.State,
			&s.Query,
			&s.DurationMs,
		)
		if err != nil {
			continue
		}
		// Apply lightweight string filtering in Go code to keep collector queries fast
		if s.UserName == "dbmonitor_user" || strings.Contains(s.Query, "/* SQL_OPTIMA */") {
			continue
		}
		sessions = append(sessions, s)
	}

	return sessions, nil
}

// GetActiveQueries returns all currently active queries.
func (c *PgRepository) GetActiveQueries(instanceName string) ([]models.PgSession, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		/* SQL_OPTIMA */ SELECT  
			pid,
			COALESCE(usename::text, '') AS usename,
			COALESCE(datname::text, '') AS datname,
			COALESCE(client_addr::text, '') AS client_addr,
			client_port,
			backend_start,
			query_start,
			state_change,
			wait_event_type,
			wait_event,
			state,
			query,
			extract(epoch FROM (now() - COALESCE(query_start, xact_start))) * 1000 as duration_ms
		FROM pg_stat_activity
		WHERE pid <> pg_backend_pid()
		AND state = 'active'
		AND COALESCE(query_start, xact_start) IS NOT NULL
		AND query NOT ILIKE '%pg_stat_activity%'
		AND query NOT ILIKE 'autovacuum:%'
		ORDER BY COALESCE(query_start, xact_start) ASC NULLS LAST
		LIMIT 100
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.PgSession
	for rows.Next() {
		var s models.PgSession
		err := rows.Scan(
			&s.PID,
			&s.UserName,
			&s.Database,
			&s.ClientAddr,
			&s.ClientPort,
			&s.BackendStart,
			&s.QueryStart,
			&s.StateChange,
			&s.WaitEventType,
			&s.WaitEvent,
			&s.State,
			&s.Query,
			&s.DurationMs,
		)
		if err != nil {
			continue
		}
		// Apply lightweight string filtering in Go code to keep collector queries fast
		if s.UserName == "dbmonitor_user" || strings.Contains(s.Query, "/* SQL_OPTIMA */") {
			continue
		}
		sessions = append(sessions, s)
	}

	return sessions, nil
}

// PgSession represents a PostgreSQL session/connection with query details.
type PgSession struct {
	PID        int        `json:"pid"`
	User       string     `json:"user"`
	Database   string     `json:"database"`
	AppName    string     `json:"app_name"`
	State      string     `json:"state"`
	Duration   string     `json:"duration"`
	WaitEvent  string     `json:"wait_event"`
	BlockedBy  *int       `json:"blocked_by"`
	Query      string     `json:"query"`
	QueryStart *time.Time `json:"-"`
}

// GetSessions returns active sessions with blocking information.
func (c *PgRepository) GetSessions(instanceName string) ([]PgSession, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	query := `
		/* SQL_OPTIMA */ SELECT   
			pid,
			usename,
			datname,
			application_name,
			state,
			EXTRACT(EPOCH FROM (now() - state_change)) as duration_seconds,
			CASE
				WHEN wait_event_type IS NULL AND wait_event IS NULL THEN ''
				ELSE COALESCE(wait_event_type, '') || ':' || COALESCE(wait_event, '')
			END as wait_event,
			pg_blocking_pids(pid) as blocked_by,
			query
		FROM pg_stat_activity 
		WHERE pid <> pg_backend_pid()
		ORDER BY state_change DESC
		LIMIT 100
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []PgSession
	for rows.Next() {
		var s PgSession
		var durationSeconds float64
		var blockedByArr []int
		err := rows.Scan(&s.PID, &s.User, &s.Database, &s.AppName, &s.State, &durationSeconds, &s.WaitEvent, pq.Array(&blockedByArr), &s.Query)
		if err != nil {
			continue
		}

		// Apply lightweight string filtering in Go code to keep collector queries fast
		if s.User == "dbmonitor_user" || strings.Contains(s.Query, "/* SQL_OPTIMA */") {
			continue
		}

		duration := time.Duration(durationSeconds) * time.Second
		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) % 60
		seconds := int(duration.Seconds()) % 60
		s.Duration = fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)

		// Set blocked_by if any
		if len(blockedByArr) > 0 {
			s.BlockedBy = &blockedByArr[0]
		}

		sessions = append(sessions, s)
	}

	return sessions, nil
}

// TerminateSession kills a PostgreSQL backend process by PID.
func (c *PgRepository) TerminateSession(instanceName string, pid int) error {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return fmt.Errorf("connection not found")
	}

	_, err := db.Exec("/* SQL_OPTIMA */ SELECT   pg_terminate_backend($1)", pid)
	if err != nil {
		return fmt.Errorf("failed to terminate session %d: %w", pid, err)
	}

	log.Printf("[POSTGRES] Terminated session PID %d on %s", pid, instanceName)
	return nil
}

// GetWaitEventSummary returns a summary of wait events by category for active sessions.
func (c *PgRepository) GetWaitEventSummary(instanceName string) (map[string]int, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		/* SQL_OPTIMA */ SELECT   
			COALESCE(wait_event_type, 'CPU') as wait_type, 
			COUNT(*) as count 
		FROM pg_stat_activity 
		WHERE state = 'active' 
		GROUP BY wait_event_type
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summary := make(map[string]int)
	for rows.Next() {
		var waitType string
		var count int
		if err := rows.Scan(&waitType, &count); err != nil {
			continue
		}
		summary[waitType] = count
	}

	return summary, nil
}

// GetTopWaitEvents returns the top wait events across all sessions.
func (c *PgRepository) GetTopWaitEvents(instanceName string, limit int) (map[string]int, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	if limit <= 0 {
		limit = 10
	}

	query := fmt.Sprintf(`
		/* SQL_OPTIMA */ SELECT   
			wait_event, 
			COUNT(*) as count 
		FROM pg_stat_activity 
		WHERE wait_event IS NOT NULL 
		GROUP BY wait_event 
		ORDER BY count DESC 
		LIMIT %d
	`, limit)

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make(map[string]int)
	for rows.Next() {
		var event string
		var count int
		if err := rows.Scan(&event, &count); err != nil {
			continue
		}
		events[event] = count
	}

	return events, nil
}
