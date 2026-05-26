// SQL Optima — query-text noise predicates for SQL Server workload reads.
package hot

import "strings"

// sqlServerQueryTextSystemNoiseSQL excludes DMV/catalog-shaped SQL on query_text_raw OR statement_text.
func sqlServerQueryTextSystemNoiseSQL(col string) string {
	raw := col + `query_text_raw`
	stmt := col + `statement_text`
	match := func(pattern string) string {
		p := strings.ReplaceAll(pattern, `'`, `''`)
		return `(COALESCE(` + raw + `, '') ILIKE '` + p + `' OR COALESCE(` + stmt + `, '') ILIKE '` + p + `')`
	}
	parts := []string{
		match(`%sys.dm_%`),
		match(`%sys.partitions%`),
		match(`%sys.plan_%`),
		match(`%sys.all_objects%`),
		match(`%sys.dm_exec_query_stats%`),
		match(`%sys.dm_exec_sql_text%`),
		match(`%sys.dm_exec_plan_attributes%`),
		match(`%is_ms_shipped%`),
		match(`%sp_mshistory_cleanup%`),
		match(`(@_msparam_0%`),
	}
	upper := func(like string) string {
		return `(UPPER(COALESCE(` + raw + `, '')) LIKE '` + strings.ReplaceAll(like, `'`, `''`) + `' OR UPPER(COALESCE(` + stmt + `, '')) LIKE '` + strings.ReplaceAll(like, `'`, `''`) + `')`
	}
	parts = append(parts,
		match(`%db_id()%system_type_id%`),
		match(`%db_id()%is_inlineable%`),
		match(`%db_id()%vector_column_count%`),
		upper(`%BACKUP DATABASE%`),
		upper(`%RESTORE DATABASE%`),
		upper(`SELECT % FROM SYS.%`),
		upper(`SELECT % FROM [SYS].%`),
		upper(`SELECT % FROM MSDB.%`),
		upper(`SELECT % FROM INFORMATION_SCHEMA.%`),
	)
	return `
		  AND NOT (` + strings.Join(parts, `
		    OR `) + `)`
}

// sqlServerMisclassifiedWorkloadSQL drops legacy rows stored as is_user_workload=1 without user attribution.
func sqlServerMisclassifiedWorkloadSQL(col string, monitoringLogins []string) string {
	logins := mergeSqlServerExcludedLogins(monitoringLogins)
	quoted := []string{`''`, `'unknown'`, `'sql-optima'`, `'sql_optima'`, `'sqloptima'`}
	for _, l := range logins {
		quoted = append(quoted, `'`+strings.ReplaceAll(l, `'`, `''`)+`'`)
	}
	raw := col + `query_text_raw`
	stmt := col + `statement_text`
	return `
		  AND NOT (
		    COALESCE(` + col + `is_user_workload, 0) = 1
		    AND LOWER(TRIM(COALESCE(` + col + `login_name, ''))) IN (` + strings.Join(quoted, ", ") + `)
		    AND (
		      COALESCE(` + raw + `, '') ILIKE '%sys.dm_%'
		      OR COALESCE(` + stmt + `, '') ILIKE '%sys.dm_%'
		      OR COALESCE(` + raw + `, '') ILIKE '%/* SQL_OPTIMA%'
		      OR COALESCE(` + stmt + `, '') ILIKE '%/* SQL_OPTIMA%'
		      OR LOWER(TRIM(COALESCE(` + col + `application_name, ''))) IN ('sql-optima', 'go-mssqldb', 'sqlserverms')
		    )
		  )`
}
