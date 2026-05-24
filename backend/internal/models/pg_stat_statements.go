// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Response DTOs for the enhanced pg_stat_statements dashboard API endpoints.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package models

import "time"

// PgssWorkloadPoint is a single time-series point for the workload overview.
type PgssWorkloadPoint struct {
	Timestamp          time.Time `json:"ts"`
	QueryLoadMsSec     float64   `json:"query_load_ms_sec"`
	QPS                float64   `json:"qps"`
	RowsSec            float64   `json:"rows_sec"`
	CacheHitRatio      float64   `json:"cache_hit_ratio"`
	WalBytesSec        float64   `json:"wal_bytes_sec"`
	PlanningMsSec      float64   `json:"planning_ms_sec"`
	ExecPct            float64   `json:"exec_pct"`
	PlanPct            float64   `json:"plan_pct"`
	RowsPerQuery       float64   `json:"rows_per_query"`
	TempMbSec          float64   `json:"temp_mb_sec"`
	BlksReadSec        float64   `json:"blks_read_sec"`
	TempBlksWrittenSec float64   `json:"temp_blks_written_sec"`
	CpuSaturationMsSec float64   `json:"cpu_saturation_ms_sec"`
}

// PgssLatencyPoint holds latency percentile approximations for a time bucket.
type PgssLatencyPoint struct {
	Timestamp time.Time `json:"ts"`
	P50       float64   `json:"p50"`
	P95       float64   `json:"p95"`
	P99       float64   `json:"p99"`
	MaxExec   float64   `json:"max_exec"`
}

// PgssTopQuery represents a single row in the top queries table.
type PgssTopQuery struct {
	QueryID      int64     `json:"query_id"`
	Query        string    `json:"query"`
	DbName       string    `json:"db_name"`
	UserName     string    `json:"username"`
	AppName      string    `json:"app_name"`
	QueryType    string    `json:"query_type"`
	TotalTime    float64   `json:"total_time_ms"`
	PctDBTime    float64   `json:"pct_db_time"`
	Calls        int64     `json:"calls"`
	AvgMs        float64   `json:"avg_ms"`
	RowsPerCall  float64   `json:"rows_per_call"`
	HitPct       float64   `json:"hit_pct"`
	TempMB       float64   `json:"temp_mb"`
	WalMB        float64   `json:"wal_mb"`
	ReadsPerCall float64   `json:"reads_per_call"`
	PlanRatio    float64   `json:"plan_ratio"`
	Flags        []string  `json:"flags"`
	CapturedAt   time.Time `json:"captured_at"`
}

// PgssFilterOptions holds distinct dimension values for filter dropdowns.
type PgssFilterOptions struct {
	Instance  string   `json:"instance"`
	Databases []string `json:"databases"`
	Users     []string `json:"users"`
	AppNames  []string `json:"app_names"`
}

// PgssDbBreakdown is a per-database workload summary row.
type PgssDbBreakdown struct {
	DbName         string  `json:"db_name"`
	TotalExecMs    float64 `json:"total_exec_ms"`
	PctOfServer    float64 `json:"pct_of_server"`
	TotalCalls     int64   `json:"total_calls"`
	AvgMs          float64 `json:"avg_ms"`
	CacheHitPct    float64 `json:"cache_hit_pct"`
	UniqueQueryIDs int64   `json:"unique_query_ids"`
}

// PgssUserBreakdown is a per-login workload summary row.
type PgssUserBreakdown struct {
	UserName       string  `json:"username"`
	TotalExecMs    float64 `json:"total_exec_ms"`
	PctOfServer    float64 `json:"pct_of_server"`
	TotalCalls     int64   `json:"total_calls"`
	AvgMs          float64 `json:"avg_ms"`
	UniqueQueryIDs int64   `json:"unique_query_ids"`
}

// PgssDbBreakdownResponse is the API response for /api/postgres/pgss/by-database.
type PgssDbBreakdownResponse struct {
	Instance string            `json:"instance"`
	Rows     []PgssDbBreakdown `json:"rows"`
}

// PgssUserBreakdownResponse is the API response for /api/postgres/pgss/by-user.
type PgssUserBreakdownResponse struct {
	Instance string              `json:"instance"`
	Rows     []PgssUserBreakdown `json:"rows"`
}

// PgssRegression represents a query that degraded between two time windows.
type PgssRegression struct {
	QueryID    int64     `json:"query_id"`
	Query      string    `json:"query"`
	PrevAvgMs  float64   `json:"prev_avg_ms"`
	CurrAvgMs  float64   `json:"curr_avg_ms"`
	ChangePct  float64   `json:"change_pct"`
	Status     string    `json:"status"`
	DetectedAt time.Time `json:"detected_at"`
}

// PgssWorkloadResponse is the API response for /api/postgres/pgss/workload.
type PgssWorkloadResponse struct {
	Instance string              `json:"instance"`
	Points   []PgssWorkloadPoint `json:"points"`
}

// PgssLatencyResponse is the API response for /api/postgres/pgss/latency.
type PgssLatencyResponse struct {
	Instance string             `json:"instance"`
	Points   []PgssLatencyPoint `json:"points"`
}

// PgssTopQueriesResponse is the API response for /api/postgres/pgss/top.
type PgssTopQueriesResponse struct {
	Instance string         `json:"instance"`
	SortBy   string         `json:"sort_by"`
	Queries  []PgssTopQuery `json:"queries"`
}

// PgssRegressionsResponse is the API response for /api/postgres/pgss/regressions.
type PgssRegressionsResponse struct {
	Instance    string           `json:"instance"`
	Regressions []PgssRegression `json:"regressions"`
}

// PgssStatusResponse is the API response for /api/postgres/pgss/status.
type PgssStatusResponse struct {
	Instance            string `json:"instance"`
	ExtensionInstalled  bool   `json:"extension_installed"`
	SharedPreloadActive bool   `json:"shared_preload_active"`
	Ready               bool   `json:"ready"`
	Message             string `json:"message,omitempty"`
}

// PgssSummaryResponse is the API response for /api/postgres/pgss/summary.
type PgssSummaryResponse struct {
	Instance           string  `json:"instance"`
	QueryLoadMsSec     float64 `json:"query_load_ms_sec"`
	QPS                float64 `json:"qps"`
	P99Ms              float64 `json:"p99_ms"`
	CacheHitPct        float64 `json:"cache_hit_pct"`
	TempMbSec          float64 `json:"temp_mb_sec"`
	WalMbSec           float64 `json:"wal_mb_sec"`
	CpuSaturationMsSec float64 `json:"cpu_saturation_ms_sec"`
	UniqueQueryCount   int64   `json:"unique_query_count"`
}
