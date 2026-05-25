// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB read/write for sqlserver_active_sessions and
// sqlserver_memory_grants_detail tables.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ActiveSessionWriteRow is the payload written to sqlserver_active_sessions.
type ActiveSessionWriteRow struct {
	SessionID            int
	LoginName            string
	HostName             string
	ProgramName          string
	DatabaseName         string
	RequestStatus        string
	WaitType             string
	WaitTimeMs           int64
	BlockingSessionID    int
	CPUTimeMs            int64
	TotalElapsedMs       int64
	LogicalReads         int64
	Reads                int64
	Writes               int64
	GrantedQueryMemoryKB int64
	DOP                  int
	QueryHash            string
	QueryText            string
}

// LogSqlServerActiveSessions batch-inserts a session snapshot into TimescaleDB.
func (tl *TimescaleLogger) LogSqlServerActiveSessions(ctx context.Context, serverID uuid.UUID, capturedAt time.Time, rows []ActiveSessionWriteRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	const q = `
		INSERT INTO sqlserver_active_sessions (
			capture_timestamp, server_id,
			session_id, login_name, host_name, program_name, database_name,
			request_status, wait_type, wait_time_ms, blocking_session_id,
			cpu_time_ms, total_elapsed_ms, logical_reads, reads, writes,
			granted_query_memory_kb, dop, query_hash, query_text
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`
	for _, r := range rows {
		batch.Queue(q, capturedAt, serverID,
			r.SessionID, r.LoginName, r.HostName, r.ProgramName, r.DatabaseName,
			r.RequestStatus, r.WaitType, r.WaitTimeMs, r.BlockingSessionID,
			r.CPUTimeMs, r.TotalElapsedMs, r.LogicalReads, r.Reads, r.Writes,
			r.GrantedQueryMemoryKB, r.DOP, r.QueryHash, r.QueryText,
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("sqlserver_active_sessions insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

// GetActiveSessionsForServer returns all session rows for a given server within the
// provided time window. Used by blocking detection and connection stats readers.
func (tl *TimescaleLogger) GetActiveSessionsForServer(ctx context.Context, serverID uuid.UUID, maxAgeSeconds int) ([]map[string]interface{}, error) {
	if maxAgeSeconds <= 0 {
		maxAgeSeconds = 120
	}
	q := fmt.Sprintf(`
		SELECT
			capture_timestamp, session_id, login_name, host_name, program_name,
			database_name, request_status, wait_type, wait_time_ms, blocking_session_id,
			cpu_time_ms, total_elapsed_ms, logical_reads, reads, writes,
			granted_query_memory_kb, dop, query_hash, query_text
		FROM sqlserver_active_sessions
		WHERE server_id = $1
		  AND capture_timestamp >= NOW() - INTERVAL '%d seconds'
		ORDER BY capture_timestamp DESC, blocking_session_id DESC
		LIMIT 5000`, maxAgeSeconds)

	rows, err := tl.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var sessionID, blockingID, dop int
		var waitTimeMs, cpuMs, elapsedMs, logicalReads, reads, writes, grantedKB int64
		var loginName, hostName, programName, dbName, reqStatus, waitType, queryHash, queryText string
		if err := rows.Scan(&ts,
			&sessionID, &loginName, &hostName, &programName,
			&dbName, &reqStatus, &waitType, &waitTimeMs, &blockingID,
			&cpuMs, &elapsedMs, &logicalReads, &reads, &writes,
			&grantedKB, &dop, &queryHash, &queryText,
		); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"capture_timestamp":      ts,
			"session_id":             sessionID,
			"login_name":             loginName,
			"host_name":              hostName,
			"program_name":           programName,
			"database_name":          dbName,
			"request_status":         reqStatus,
			"wait_type":              waitType,
			"wait_time_ms":           waitTimeMs,
			"blocking_session_id":    blockingID,
			"cpu_time_ms":            cpuMs,
			"total_elapsed_ms":       elapsedMs,
			"logical_reads":          logicalReads,
			"reads":                  reads,
			"writes":                 writes,
			"granted_query_memory_kb": grantedKB,
			"dop":                    dop,
			"query_hash":             queryHash,
			"query_text":             queryText,
		})
	}
	return out, rows.Err()
}

// MemoryGrantDetailWriteRow is the payload for sqlserver_memory_grants_detail.
type MemoryGrantDetailWriteRow struct {
	SessionID          int
	LoginName          string
	DatabaseName       string
	QueryCost          float64
	RequestedMemoryKB  int64
	GrantedMemoryKB    int64
	UsedMemoryKB       int64
	MaxUsedMemoryKB    int64
	DOP                int
	GrantTime          *time.Time
	QueueTime          *time.Time
}

// LogSqlServerMemoryGrantsDetail batch-inserts per-session memory grant detail rows.
func (tl *TimescaleLogger) LogSqlServerMemoryGrantsDetail(ctx context.Context, serverID uuid.UUID, capturedAt time.Time, rows []MemoryGrantDetailWriteRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	const q = `
		INSERT INTO sqlserver_memory_grants_detail (
			capture_timestamp, server_id,
			session_id, login_name, database_name,
			query_cost, requested_memory_kb, granted_memory_kb,
			used_memory_kb, max_used_memory_kb, dop,
			grant_time, queue_time
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`
	for _, r := range rows {
		batch.Queue(q, capturedAt, serverID,
			r.SessionID, r.LoginName, r.DatabaseName,
			r.QueryCost, r.RequestedMemoryKB, r.GrantedMemoryKB,
			r.UsedMemoryKB, r.MaxUsedMemoryKB, r.DOP,
			r.GrantTime, r.QueueTime,
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("sqlserver_memory_grants_detail insert failed at row %d: %w", i, err)
		}
	}
	return nil
}
