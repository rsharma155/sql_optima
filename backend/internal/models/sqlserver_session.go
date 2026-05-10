// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Model for SQL Server session snapshots used for workload attribution.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package models

import "time"

// SQLServerSessionSnapshot represents a snapshot of a SQL Server session
type SQLServerSessionSnapshot struct {
	SampleTime        time.Time `json:"sample_time"`
	InstanceID        string    `json:"instance_id"`
	SessionID         int       `json:"session_id"`
	LoginName         string    `json:"login_name"`
	OriginalLoginName string    `json:"original_login_name"`
	HostName          string    `json:"host_name"`
	ProgramName       string    `json:"program_name"`
	DatabaseName      string    `json:"database_name"`
	IsUserProcess     bool      `json:"is_user_process"`
	Status            string    `json:"status"`
	QueryHash         []byte    `json:"query_hash"`
	QueryPlanHash     []byte    `json:"query_plan_hash"`

	// Live Session Fields (NEW - for Live Sessions dashboard)
	TotalElapsedTimeMs *float64 `json:"total_elapsed_time_ms"` // Duration since session started
	CPUTimeMs          *float64 `json:"cpu_time_ms"`            // CPU time used by session
	WaitType           string   `json:"wait_type"`              // Current wait type
	BlockingSessionID   *int     `json:"blocking_session_id"`    // SPID of blocking session (if any)
	QueryText          string   `json:"query_text"`             // Current SQL query text
}
