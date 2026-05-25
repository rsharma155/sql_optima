// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Shared rollup for SQL Server top-query dashboards (Query Analysis + Workload).
//          metrics_v2 stores per-interval deltas; dashboards SUM them over the selected window.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

// sqlServerTopQueryGroupBySQL deduplicates hypertable rows to one line per query_hash.
// When a database scope is active, database_name is already filtered — grouping only by hash
// avoids duplicate rows caused by inconsistent database_name strings in storage.
func sqlServerTopQueryGroupBySQL(col string, databaseScoped bool) string {
	if databaseScoped {
		return ` GROUP BY ` + col + `query_hash`
	}
	return ` GROUP BY ` + col + `query_hash, LOWER(TRIM(COALESCE(` + col + `database_name, 'unknown')))`
}

// sqlServerTopQueryMetricsAggSQL returns SUM of interval deltas and weighted per-execution averages.
func sqlServerTopQueryMetricsAggSQL(col string) string {
	return `
		       MAX(` + col + `statement_text) AS statement_text,
		       MAX(` + col + `database_name) AS database_name,
		       MAX(` + col + `login_name) AS login_name,
		       MAX(` + col + `application_name) AS application_name,
		       COUNT(DISTINCT ` + col + `plan_handle) AS plan_count,
		       SUM(` + col + `total_executions)::bigint AS total_executions,
		       SUM(` + col + `total_cpu_ms)::bigint AS total_cpu_ms,
		       SUM(COALESCE(` + col + `total_elapsed_ms, 0))::bigint AS total_elapsed_ms,
		       SUM(` + col + `total_logical_reads)::bigint AS total_logical_reads,
		       CASE WHEN SUM(` + col + `total_executions) > 0
		            THEN SUM(` + col + `total_cpu_ms)::float8 / SUM(` + col + `total_executions)
		            ELSE 0 END AS avg_cpu_ms,
		       CASE WHEN SUM(` + col + `total_executions) > 0
		            THEN SUM(COALESCE(` + col + `total_elapsed_ms, 0))::float8 / SUM(` + col + `total_executions)
		            ELSE 0 END AS avg_elapsed_ms,
		       CASE WHEN SUM(` + col + `total_executions) > 0
		            THEN SUM(` + col + `total_logical_reads)::float8 / SUM(` + col + `total_executions)
		            ELSE 0 END AS avg_logical_reads,
		       MAX(` + col + `last_execution_time) AS last_execution_time`
}
