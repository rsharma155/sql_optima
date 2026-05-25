// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Read-time SQL filters for SQL Server query metrics (TimescaleDB only).
//          Collectors store broad snapshots; dashboards apply user vs system semantics here.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

const sqlServerOptimaBatchTag = `%/* SQL_OPTIMA%`

// sqlServerCollectorSQLExcludeSQL excludes SQL Optima collector DMV/monitoring batches.
// All collector queries are tagged with /* SQL_OPTIMA */ in the monitored batch; the full
// text is stored in query_text_raw (dm_exec_sql_text), not statement_text.
func sqlServerCollectorSQLExcludeSQL(col string) string {
	return `
		  AND COALESCE(` + col + `query_text_raw, '') NOT ILIKE '` + sqlServerOptimaBatchTag + `'`
}

// sqlServerMonitoringSelfNoiseSQL strips collector-tagged SQL and monitor login/app noise.
func sqlServerMonitoringSelfNoiseSQL(col string, monitoringLogins []string) string {
	return sqlServerCollectorSQLExcludeSQL(col) +
		sqlServerMonitoringLoginExcludeSQL(col, monitoringLogins) +
		sqlServerMonitoringAppExcludeSQL(col)
}

// sqlServerQueryTextRawSystemNoiseSQL excludes catalog/DMV-shaped batches on query_text_raw.
// Intentionally omits SET/DECLARE/CREATE/ALTER/(@% — those appear in normal user CRUD batches.
func sqlServerQueryTextRawSystemNoiseSQL(col string) string {
	return `
		  AND COALESCE(` + col + `query_text_raw, '') NOT ILIKE '%sys.dm_%'
		  AND COALESCE(` + col + `query_text_raw, '') NOT ILIKE '%sys.partitions%'
		  AND COALESCE(` + col + `query_text_raw, '') NOT ILIKE '%sys.plan_%'
		  AND COALESCE(` + col + `query_text_raw, '') NOT ILIKE '%sys.all_objects%'
		  AND UPPER(COALESCE(` + col + `query_text_raw, '')) NOT LIKE '%BACKUP DATABASE%'
		  AND UPPER(COALESCE(` + col + `query_text_raw, '')) NOT LIKE '%RESTORE DATABASE%'
		  AND COALESCE(` + col + `query_text_raw, '') NOT ILIKE '%is_ms_shipped%'
		  AND UPPER(COALESCE(` + col + `query_text_raw, '')) NOT LIKE 'SELECT % FROM SYS.%'
		  AND UPPER(COALESCE(` + col + `query_text_raw, '')) NOT LIKE 'SELECT % FROM [SYS].%'
		  AND UPPER(COALESCE(` + col + `query_text_raw, '')) NOT LIKE 'SELECT % FROM MSDB.%'
		  AND UPPER(COALESCE(` + col + `query_text_raw, '')) NOT LIKE 'SELECT % FROM INFORMATION_SCHEMA.%'
		  AND COALESCE(` + col + `query_text_raw, '') NOT ILIKE '%sp_mshistory_cleanup%'
		  AND COALESCE(` + col + `query_text_raw, '') NOT ILIKE '(@_msparam_0%'`
}

// sqlServerUserWorkloadNoiseSQL excludes monitoring-tagged SQL and catalog/DMV batches.
func sqlServerUserWorkloadNoiseSQL(col string, monitoringLogins []string) string {
	return sqlServerMonitoringSelfNoiseSQL(col, monitoringLogins) + sqlServerQueryTextRawSystemNoiseSQL(col)
}

// sqlServerSnapshotReadFilter applies read filters for sqlserver_query_stats_snapshot_v2 (text columns only).
func sqlServerSnapshotReadFilter(excludeSystem bool, col string) string {
	f := sqlServerCollectorSQLExcludeSQL(col)
	if excludeSystem {
		f += sqlServerQueryTextRawSystemNoiseSQL(col)
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
