// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: sqlserver_session.go
// Purpose: SQL Server session and active request data models.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package models

import (
	"github.com/google/uuid"
	"time"
)

type SQLServerSessionSnapshot struct {
	SampleTime           time.Time `json:"sample_time"`
	CaptureTimestamp     time.Time `json:"capture_timestamp"`
	ServerID             uuid.UUID `json:"server_id"`
	SessionID            int       `json:"session_id"`
	LoginName            string    `json:"login_name"`
	OriginalLoginName    string    `json:"original_login_name"`
	HostName             string    `json:"host_name"`
	DatabaseName         string    `json:"database_name"`
	ProgramName          string    `json:"program_name"`
	Status               string    `json:"status"`
	IsUserProcess        bool      `json:"is_user_process"`
	CPUTimeMs            int       `json:"cpu_time_ms"`
	TotalElapsedTimeMs   int       `json:"total_elapsed_time_ms"`
	MemoryUsagePages     int       `json:"memory_usage_pages"`
	Reads                int64     `json:"reads"`
	Writes               int64     `json:"writes"`
	LogicalReads         int64     `json:"logical_reads"`
	OpenTransactionCount int       `json:"open_transaction_count"`
	WaitType             string    `json:"wait_type"`
	WaitTimeMs           int       `json:"wait_time_ms"`
	LastWaitType         string    `json:"last_wait_type"`
	WaitResource         string    `json:"wait_resource"`
	BlockingSessionID    int       `json:"blocking_session_id"`
	QueryHash            string    `json:"query_hash"`
	QueryPlanHash        string    `json:"query_plan_hash"`
	QueryText            string    `json:"query_text"`
}
