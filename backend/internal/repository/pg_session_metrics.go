// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL session monitoring and active query analysis (STRICT STAGING ONLY).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/rsharma155/sql_optima/internal/models"
)

// GetConnectionStats returns active, idle, and total connection counts from staging.
// STRICT: Never queries production servers directly.
func (c *PgRepository) GetConnectionStats(instanceName string) (active int, idle int, total int, err error) {
	if c.LocalPool == nil {
		return 0, 0, 0, fmt.Errorf("datasource error: TimescaleDB is not connected")
	}

	serverID, ok := c.GetServerID(instanceName)
	if !ok {
		return 0, 0, 0, fmt.Errorf("instance %s not found in registry", instanceName)
	}

	query := `
		SELECT /* SQL_OPTIMA STAGING ONLY */   
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE state = 'active') as active,
			COUNT(*) FILTER (WHERE state = 'idle') as idle
		FROM staging.postgres_activity_locks_raw
		WHERE server_id = $1
		  AND capture_timestamp = (SELECT max(capture_timestamp) FROM staging.postgres_activity_locks_raw WHERE server_id = $1)
	`
	err = c.LocalPool.QueryRow(context.Background(), query, serverID).Scan(&total, &active, &idle)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, 0, fmt.Errorf("no data: collector may be delayed for %s", instanceName)
		}
		return 0, 0, 0, fmt.Errorf("staging read error: %w", err)
	}
	return
}

// GetLongRunningQueries returns queries running longer than specified duration from staging.
// STRICT: Never queries production servers directly.
func (c *PgRepository) GetLongRunningQueries(instanceName string, minDurationSeconds int) ([]models.PgSession, error) {
	if c.LocalPool == nil {
		return nil, fmt.Errorf("datasource error: TimescaleDB is not connected")
	}

	serverID, ok := c.GetServerID(instanceName)
	if !ok {
		return nil, fmt.Errorf("instance %s not found in registry", instanceName)
	}

	query := `
		/* SQL_OPTIMA STAGING ONLY */ SELECT  
			pid,
			COALESCE(usename, '') AS usename,
			COALESCE(datname, '') AS datname,
			COALESCE(client_addr::text, '') AS client_addr,
			0 as client_port,
			now() as backend_start,
			query_start,
			state_change,
			wait_event_type,
			wait_event,
			state,
			query,
			extract(epoch FROM query_duration) * 1000 as duration_ms
		FROM staging.postgres_activity_locks_raw
		WHERE server_id = $1
		  AND state = 'active'
		  AND capture_timestamp = (SELECT max(capture_timestamp) FROM staging.postgres_activity_locks_raw WHERE server_id = $1)
		  AND extract(epoch FROM query_duration) > $2
		ORDER BY query_start ASC NULLS LAST
		LIMIT 50
	`

	rows, err := c.LocalPool.Query(context.Background(), query, serverID, minDurationSeconds)
	if err != nil {
		return nil, fmt.Errorf("staging read error: %w", err)
	}
	defer rows.Close()

	var sessions []models.PgSession
	for rows.Next() {
		var s models.PgSession
		var waitET, waitE sql.NullString
		err := rows.Scan(
			&s.PID, &s.UserName, &s.Database, &s.ClientAddr, &s.ClientPort,
			&s.BackendStart, &s.QueryStart, &s.StateChange, &waitET, &waitE,
			&s.State, &s.Query, &s.DurationMs,
		)
		if err != nil {
			continue
		}
		s.WaitEventType = waitET
		s.WaitEvent = waitE
		sessions = append(sessions, s)
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no recent data: check collector health for %s", instanceName)
	}

	return sessions, nil
}

// GetActiveQueries returns all currently executing queries for an instance from staging.
func (c *PgRepository) GetActiveQueries(instanceName string) ([]models.PgSession, error) {
	return c.GetLongRunningQueries(instanceName, -1)
}

// GetWaitEventSummary returns a map of wait events and their counts from staging.
func (c *PgRepository) GetWaitEventSummary(instanceName string) (map[string]int, error) {
	if c.LocalPool == nil {
		return nil, fmt.Errorf("datasource error: TimescaleDB unreachable")
	}

	serverID, ok := c.GetServerID(instanceName)
	if !ok {
		return nil, fmt.Errorf("instance %s not found in registry", instanceName)
	}

	query := `
		SELECT /* SQL_OPTIMA STAGING ONLY */ 
			COALESCE(wait_event, 'CPU'), COUNT(*)
		FROM staging.postgres_activity_locks_raw
		WHERE server_id = $1
		  AND capture_timestamp = (SELECT max(capture_timestamp) FROM staging.postgres_activity_locks_raw WHERE server_id = $1)
		  AND state = 'active'
		GROUP BY wait_event`

	rows, err := c.LocalPool.Query(context.Background(), query, serverID)
	if err != nil {
		return nil, fmt.Errorf("staging read error: %w", err)
	}
	defer rows.Close()

	events := make(map[string]int)
	for rows.Next() {
		var event string
		var count int
		if err := rows.Scan(&event, &count); err == nil {
			events[event] = count
		}
	}
	return events, nil
}

// GetTopWaitEvents returns the most frequent wait events from staging.
func (c *PgRepository) GetTopWaitEvents(instanceName string, limit int) (map[string]int, error) {
	return c.GetWaitEventSummary(instanceName)
}
