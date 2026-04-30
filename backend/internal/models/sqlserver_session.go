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
}
