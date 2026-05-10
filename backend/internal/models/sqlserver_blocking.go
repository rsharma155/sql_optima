// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: sqlserver_blocking.go
// Purpose: Models for SQL Server blocking and deadlocks.
// Metadata:
//   - Version: 1.1
//   - Package: models
//   - Entities: BlockingSnapshot, BlockingLock, DeadlockEvent
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package models

import "time"

// SQLServerBlockingSnapshot represents a row from the core blocking query
type SQLServerBlockingSnapshot struct {
	Timestamp            time.Time `json:"ts"`
	InstanceID           string    `json:"instance_id"`
	SessionID            int       `json:"session_id"`
	BlockingSessionID    int       `json:"blocking_session_id"`
	WaitType             string    `json:"wait_type"`
	WaitDurationMs       int64     `json:"wait_duration_ms"`
	DatabaseName         string    `json:"database_name"`
	SqlHash              string    `json:"sql_hash"`
	PlanHash             string    `json:"plan_hash"`
	LoginName            string    `json:"login_name"`
	HostName             string    `json:"host_name"`
	ProgramName          string    `json:"program_name"`
	OpenTransactionCount     int       `json:"open_transaction_count"`
	TransactionStartTime     *time.Time `json:"transaction_start_time,omitempty"`
	TransactionIsolationLevel string    `json:"transaction_isolation_level,omitempty"`
	CpuTime                  int64     `json:"cpu_time"`
	Reads                int64     `json:"reads"`
	Writes               int64     `json:"writes"`
	LogicalReads         int64     `json:"logical_reads"`
	MemoryUsage          int64     `json:"memory_usage"`
	TransactionLogBytesUsed     int64     `json:"transaction_log_bytes_used"`
	TransactionLogBytesReserved int64     `json:"transaction_log_bytes_reserved"`
	WaitResource         string    `json:"wait_resource"`
	PercentComplete      float64   `json:"percent_complete"`
	QueryText            string    `json:"query_text,omitempty"`
	QueryPlan            string    `json:"query_plan,omitempty"`
}

// SQLServerBlockingLock represents a resource lock held by a session
type SQLServerBlockingLock struct {
	Timestamp           time.Time `json:"ts"`
	InstanceID          string    `json:"instance_id"`
	SessionID           int       `json:"session_id"`
	ResourceType        string    `json:"resource_type"`
	RequestMode         string    `json:"request_mode"`
	RequestStatus       string    `json:"request_status"`
	ResourceDescription string    `json:"resource_description"`
	WaitDurationMs      int64     `json:"wait_duration_ms"`
}

// SQLServerDeadlockEvent represents a deadlock occurrence
type SQLServerDeadlockEvent struct {
	Timestamp       time.Time `json:"ts"`
	InstanceID      string    `json:"instance_id"`
	DatabaseName    string    `json:"database_name"`
	VictimSessionID int       `json:"victim_session_id"`
	VictimSqlHash   string    `json:"victim_sql_hash"`
	VictimSqlText   string    `json:"victim_sql_text,omitempty"`
	DeadlockGraph   string    `json:"deadlock_graph"`
}
