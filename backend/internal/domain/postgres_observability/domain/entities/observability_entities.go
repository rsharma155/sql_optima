// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Observability domain entities.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package entities

import "time"

// SessionActivity represents a snapshot of an active session.
type SessionActivity struct {
	TS              time.Time `json:"ts"`
	InstanceID      string    `json:"instance_id"`
	DBName          string    `json:"dbname"`
	PID             int       `json:"pid"`
	Username        string    `json:"usename"`
	ApplicationName string    `json:"application_name"`
	ClientAddr      string    `json:"client_addr"`
	State           string    `json:"state"`
	WaitEventType   string    `json:"wait_event_type"`
	WaitEvent       string    `json:"wait_event"`
	BackendType     string    `json:"backend_type"`
	QueryID         int64     `json:"query_id"`
	Query           string    `json:"query"`
	XactStart       *time.Time `json:"xact_start"`
	QueryStart      *time.Time `json:"query_start"`
	StateChange     *time.Time `json:"state_change"`
	BackendStart    *time.Time `json:"backend_start"`
}

// WaitEventSummary represents aggregated wait event data.
type WaitEventSummary struct {
	TS            time.Time `json:"ts"`
	InstanceID    string    `json:"instance_id"`
	WaitEventType string    `json:"wait_event_type"`
	WaitEvent     string    `json:"wait_event"`
	Sessions      int       `json:"sessions"`
}

// DBLoad represents database load metrics.
type DBLoad struct {
	TS               time.Time `json:"ts"`
	InstanceID       string    `json:"instance_id"`
	ActiveSessions   int       `json:"active_sessions"`
	CPUSessions      int       `json:"cpu_sessions"`
	WaitingSessions  int       `json:"waiting_sessions"`
	IOWaitSessions   int       `json:"io_wait_sessions"`
	LockWaitSessions int       `json:"lock_wait_sessions"`
	IdleInTxn        int       `json:"idle_in_txn"`
}

// QueryWaitProfile represents query wait profiling data from pg_stat_statements.
type QueryWaitProfile struct {
	TS               time.Time `json:"ts"`
	InstanceID       string    `json:"instance_id"`
	QueryID          int64     `json:"queryid"`
	Calls            int64     `json:"calls"`
	TotalExecTime    float64   `json:"total_exec_time"`
	MeanExecTime     float64   `json:"mean_exec_time"`
	Rows             int64     `json:"rows"`
	SharedBlksHit    int64     `json:"shared_blks_hit"`
	SharedBlksRead   int64     `json:"shared_blks_read"`
	TempBlksWritten  int64     `json:"temp_blks_written"`
	Query            string    `json:"query"`
	Username         string    `json:"usename"`
}
