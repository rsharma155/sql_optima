// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Read-time SQL filters for SQL Server query metrics (TimescaleDB only).
//          Collectors store broad snapshots; dashboards apply user vs system semantics here.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import "fmt"

const sqlServerOptimaBatchTag = `%/* SQL_OPTIMA%`

// sqlServerQueryMetricsV2LateralSelect is the column set from sqlserver_query_metrics_v2
// required when a LATERAL subquery is aliased as qm and filtered with sqlServerQueryAnalysisScopeSQL.
const sqlServerQueryMetricsV2LateralSelect = `query_text_raw, statement_text, application_name, login_name, database_name, total_cpu_ms, total_elapsed_ms, is_user_workload`

// sqlServerCollectorSQLExcludeSQL excludes SQL Optima collector DMV/monitoring batches.
// Tag may appear on query_text_raw (full batch) or statement_text (statement slice).
func sqlServerCollectorSQLExcludeSQL(col string) string {
	return `
		  AND COALESCE(` + col + `query_text_raw, '') NOT ILIKE '` + sqlServerOptimaBatchTag + `'
		  AND COALESCE(` + col + `statement_text, '') NOT ILIKE '` + sqlServerOptimaBatchTag + `'`
}

// sqlServerUserWorkloadOnlySQL keeps rows the collector marked as user workload (exclude_system=true).
func sqlServerUserWorkloadOnlySQL(col string) string {
	return ` AND COALESCE(` + col + `is_user_workload, 0) = 1`
}

// sqlServerMonitoringSelfNoiseSQL strips collector-tagged SQL and monitor application noise.
// Used when exclude_system=false (broader view). Does not apply login blanket exclusion.
func sqlServerMonitoringSelfNoiseSQL(col string, monitoringLogins []string) string {
	_ = monitoringLogins
	return sqlServerCollectorSQLExcludeSQL(col) +
		sqlServerMonitoringAppExcludeSQL(col)
}

// sqlServerUserWorkloadNoiseSQL is the strict dashboard filter (exclude_system=true).
// Requires collector is_user_workload=1, strips DMV/SQL_OPTIMA/monitor-app noise on both text columns,
// and drops legacy rows mis-tagged as user workload when attribution was unknown.
func sqlServerUserWorkloadNoiseSQL(col string, monitoringLogins []string) string {
	return sqlServerCollectorSQLExcludeSQL(col) +
		sqlServerMonitoringAppExcludeSQL(col) +
		sqlServerQueryTextSystemNoiseSQL(col) +
		sqlServerMisclassifiedWorkloadSQL(col, monitoringLogins) +
		sqlServerUserWorkloadOnlySQL(col)
}

// sqlServerSnapshotReadFilter applies read filters for sqlserver_query_stats_snapshot_v2 (text columns only).
func sqlServerSnapshotReadFilter(excludeSystem bool, col string) string {
	f := sqlServerCollectorSQLExcludeSQL(col)
	if excludeSystem {
		f += sqlServerQueryTextSystemNoiseSQL(col)
	}
	return f
}

// sqlServerQueryAnalysisDistributionExcludeSQL drops replication distributor database noise from Query Analysis.
func sqlServerQueryAnalysisDistributionExcludeSQL(col string) string {
	return ` AND LOWER(TRIM(COALESCE(` + col + `database_name, ''))) <> 'distribution'`
}

// sqlServerQueryAnalysisReadFilter applies Option A read semantics for Query Analysis.
// excludeSystem true: user workload only; false: stored metrics minus monitoring self-noise.
// monitoringLogins should include optima_servers.username for the target instance.
func sqlServerQueryAnalysisReadFilter(excludeSystem bool, col string, monitoringLogins []string) string {
	if excludeSystem {
		return sqlServerUserWorkloadNoiseSQL(col, monitoringLogins)
	}
	return sqlServerMonitoringSelfNoiseSQL(col, monitoringLogins)
}

// sqlServerQueryAnalysisScopeSQL is the standard Query Analysis WHERE fragment (read filters + no distribution DB).
func sqlServerQueryAnalysisScopeSQL(excludeSystem bool, col string, monitoringLogins []string) string {
	return sqlServerQueryAnalysisReadFilter(excludeSystem, col, monitoringLogins) +
		sqlServerQueryAnalysisDistributionExcludeSQL(col)
}

// sqlServerQueryAnalysisQualifyingHashesCTE scans metrics_v2 once for user-workload query_hashes in range.
// Use instead of per-row LATERAL joins on sqlserver_query_stats_history (avoids 60s+ plans on large hypertables).
// Expects $1=server_id, $2=from, $3=to; optional $dbArgPos database name when databaseScoped.
func sqlServerQueryAnalysisQualifyingHashesCTE(excludeSystem bool, monitoringLogins []string, databaseScoped bool, dbArgPos int) string {
	if !excludeSystem {
		return ""
	}
	dbClause := ""
	if databaseScoped && dbArgPos > 0 {
		dbClause = fmt.Sprintf(" AND LOWER(TRIM(COALESCE(m.database_name, ''))) = LOWER(TRIM($%d))", dbArgPos)
	}
	// Extend the lower bound by 1 hour so a narrow selected range does not exclude hashes
	// whose latest metrics_v2 snapshot landed just before $2 due to collection timing skew.
	// The actual aggregation window [$2, $3] is still enforced by the outer history query.
	return `WITH qualifying_hashes AS (
    SELECT DISTINCT m.query_hash
    FROM sqlserver_query_metrics_v2 m
    WHERE m.server_id = $1
      AND m.capture_timestamp >= ($2::timestamptz - INTERVAL '1 hour')
      AND m.capture_timestamp <= $3::timestamptz` +
		sqlServerUserWorkloadNoiseSQL("m.", monitoringLogins) + dbClause + `
  )`
}

// sqlServerQueryAnalysisHistoryHashJoin filters history to hashes from qualifying_hashes CTE.
func sqlServerQueryAnalysisHistoryHashJoin() string {
	return `INNER JOIN qualifying_hashes qq ON qq.query_hash = qh.query_hash`
}

// sqlServerQueryAnalysisSnapshotHashJoin filters snapshot rows to qualifying_hashes CTE.
func sqlServerQueryAnalysisSnapshotHashJoin() string {
	return `INNER JOIN qualifying_hashes qq ON qq.query_hash = s.query_hash`
}

// sqlServerQueryAnalysisMetricsLateralJoin attaches the latest metrics_v2 row for a history/snapshot query_hash.
// Prefer sqlServerQueryAnalysisQualifyingHashesCTE when excludeSystem is true (summary/history aggregates).
// histAlias is the outer table alias (e.g. qh, s). When excludeSystem is true, uses INNER JOIN LATERAL (legacy path).
func sqlServerQueryAnalysisMetricsLateralJoin(excludeSystem bool, histAlias string, monitoringLogins []string) (joinSQL string, outerScopeSQL string) {
	base := `
			SELECT ` + sqlServerQueryMetricsV2LateralSelect + `
			FROM sqlserver_query_metrics_v2 m
			WHERE m.server_id = ` + histAlias + `.server_id AND m.query_hash = ` + histAlias + `.query_hash`
	if excludeSystem {
		return `INNER JOIN LATERAL (` + base + sqlServerUserWorkloadNoiseSQL("m.", monitoringLogins) + `
			ORDER BY m.capture_timestamp DESC
			LIMIT 1
		) qm ON true`, ""
	}
	return `LEFT JOIN LATERAL (` + base + `
			ORDER BY m.capture_timestamp DESC
			LIMIT 1
		) qm ON true`, sqlServerQueryAnalysisScopeSQL(excludeSystem, "qm.", monitoringLogins)
}

func sqlServerQueryAnalysisClassificationFilter(excludeSystem bool, tableCol string) string {
	if !excludeSystem {
		return ""
	}
	if tableCol == "" {
		tableCol = "q."
	}
	// EXISTS avoids row multiplication if classification_dim ever has extra rows per hash.
	return ` AND NOT EXISTS (
		  SELECT 1 FROM sqlserver_query_classification_dim class
		  WHERE class.server_id = ` + tableCol + `server_id
		    AND class.query_hash = ` + tableCol + `query_hash
		    AND class.classification = 'SYSTEM'
		)`
}

// sqlServerOptimaTaggedSQLExcludeSQL excludes collector-tagged text on an arbitrary column (e.g. regressions.query_text).
func sqlServerOptimaTaggedSQLExcludeSQL(col, column string) string {
	return `
		  AND COALESCE(` + col + column + `, '') NOT ILIKE '` + sqlServerOptimaBatchTag + `'`
}
