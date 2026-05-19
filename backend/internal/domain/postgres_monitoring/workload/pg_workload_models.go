// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Models for PostgreSQL Workload.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package workload

type PgTopQuery struct {
	QueryID      string  `json:"queryid"`
	Query        string  `json:"query"`
	Calls        int64   `json:"calls"`
	TotalTimePct float64 `json:"total_time_pct"`
	CPUPct       float64 `json:"cpu_pct"`
	IOPct        float64 `json:"io_pct"`
	MeanExecTime float64 `json:"mean_exec_time"`
	Rows         int64   `json:"rows"`
}

type WorkloadSummary struct {
	TPS            float64      `json:"tps"`
	ActiveSessions int          `json:"active_sessions"`
	TopQueries     []PgTopQuery `json:"top_queries"`
}
