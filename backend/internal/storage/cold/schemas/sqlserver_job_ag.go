// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/sqlserver_job_ag.go
// Purpose: Typed Parquet schemas for SQL Server Job metrics and Availability Group Cluster information.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// SQLServerJobMetricsRow is the Parquet schema for sqlserver_job_metrics.
type SQLServerJobMetricsRow struct {
	CaptureTimestampMs    int64  `parquet:"name=capture_timestamp_ms,    type=INT64"`
	ServerID              string `parquet:"name=server_id,               type=BYTE_ARRAY, converted=STRING"`
	TotalJobs             int32  `parquet:"name=total_jobs,              type=INT32"`
	EnabledJobs           int32  `parquet:"name=enabled_jobs,            type=INT32"`
	DisabledJobs          int32  `parquet:"name=disabled_jobs,           type=INT32"`
	RunningJobs           int32  `parquet:"name=running_jobs,            type=INT32"`
	FailedJobs24h         int32  `parquet:"name=failed_jobs_24h,         type=INT32"`
	CriticalJobsDisabled int32  `parquet:"name=critical_jobs_disabled,  type=INT32"`
	ErrorMessage          string `parquet:"name=error_message,           type=BYTE_ARRAY, converted=STRING"`
}

// SQLServerAGClusterRow is the Parquet schema for monitor.sqlserver_ag_cluster_info.
type SQLServerAGClusterRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	ClusterName        string `parquet:"name=cluster_name,         type=BYTE_ARRAY, converted=STRING"`
	QuorumType         string `parquet:"name=quorum_type,          type=BYTE_ARRAY, converted=STRING"`
	QuorumState        string `parquet:"name=quorum_state,         type=BYTE_ARRAY, converted=STRING"`
	MembersJSON        string `parquet:"name=members_json,         type=BYTE_ARRAY, converted=STRING"`
}
