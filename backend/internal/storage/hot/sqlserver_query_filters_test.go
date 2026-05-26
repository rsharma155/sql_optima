// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for SQL Server read-time query filters.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"fmt"
	"strings"
	"testing"
)

func TestSqlServerCollectorSQLExcludeSQL_usesQueryTextRaw(t *testing.T) {
	f := sqlServerCollectorSQLExcludeSQL("q.")
	if !strings.Contains(f, "q.query_text_raw") || !strings.Contains(f, "q.statement_text") {
		t.Fatalf("expected /* SQL_OPTIMA filter on query_text_raw and statement_text: %s", f)
	}
	if !strings.Contains(f, "%/* SQL_OPTIMA%") {
		t.Fatalf("expected /* SQL_OPTIMA tag match: %s", f)
	}
}

func TestSqlServerQueryAnalysisDistributionExclude(t *testing.T) {
	sql := sqlServerQueryAnalysisDistributionExcludeSQL("q.")
	if !strings.Contains(sql, "distribution") {
		t.Fatalf("expected distribution exclusion: %s", sql)
	}
}

func TestSqlServerQueryAnalysisReadFilter_OptionA(t *testing.T) {
	monitorUser := []string{"my_monitor"}
	checked := sqlServerQueryAnalysisReadFilter(true, "qm.", monitorUser)
	if !strings.Contains(checked, "qm.query_text_raw") {
		t.Fatal("checked filter should apply predicates on query_text_raw")
	}
	if !strings.Contains(checked, "%/* SQL_OPTIMA%") {
		t.Fatal("checked filter should exclude /* SQL_OPTIMA collector SQL")
	}
	if !strings.Contains(checked, "is_user_workload") || strings.Contains(checked, "is_user_workload, 1)") {
		t.Fatal("checked filter should require is_user_workload = 1 with NULL defaulting to 0")
	}
	if !strings.Contains(checked, "is_user_workload, 0)") {
		t.Fatal("expected COALESCE(is_user_workload, 0)")
	}
	if strings.Contains(checked, "NOT LIKE 'SET %'") {
		t.Fatal("checked filter must not exclude SET batches (normal CRUD)")
	}
	if !strings.Contains(checked, "sys.dm_") {
		t.Fatal("checked filter should exclude DMV-shaped SQL on either text column")
	}
	if !strings.Contains(checked, "statement_text") {
		t.Fatal("collector tag filter should cover statement_text as well as query_text_raw")
	}
	if strings.Contains(checked, "total_cpu_ms > 1") {
		t.Fatal("cpu threshold heuristic must not be used")
	}
	noise := sqlServerQueryTextSystemNoiseSQL("qm.")
	if !strings.Contains(noise, "qm.statement_text") || !strings.Contains(noise, "qm.query_text_raw") {
		t.Fatal("system SQL heuristics should check query_text_raw and statement_text")
	}
	if !strings.Contains(noise, "sys.dm_exec_query_stats") {
		t.Fatal("expected collector DMV shape exclusion")
	}

	unchecked := sqlServerQueryAnalysisReadFilter(false, "q.", monitorUser)
	if strings.Contains(unchecked, "is_user_workload") {
		t.Fatal("unchecked should not gate on is_user_workload")
	}
	if !strings.Contains(unchecked, "q.query_text_raw") || !strings.Contains(unchecked, "%/* SQL_OPTIMA%") {
		t.Fatal("unchecked should still exclude collector SQL on query_text_raw")
	}
}

func TestSqlServerSnapshotReadFilter_noLoginColumns(t *testing.T) {
	f := sqlServerSnapshotReadFilter(true, "s.")
	if strings.Contains(f, "login_name") || strings.Contains(f, "application_name") || strings.Contains(f, "is_user_workload") {
		t.Fatalf("snapshot filter must not reference metrics-only columns: %s", f)
	}
	if !strings.Contains(f, "s.query_text_raw") {
		t.Fatal("snapshot filter should use query_text_raw")
	}
}

func TestSqlServerQueryAnalysisQualifyingHashesCTE_syntax(t *testing.T) {
	cte := sqlServerQueryAnalysisQualifyingHashesCTE(true, []string{"mon"}, true, 4)
	if !strings.HasPrefix(cte, "WITH qualifying_hashes AS (") {
		t.Fatalf("expected WITH qualifying_hashes CTE prefix: %s", cte)
	}
	if strings.HasSuffix(strings.TrimSpace(cte), "),") || strings.HasSuffix(strings.TrimSpace(cte), "), ") {
		t.Fatalf("single-CTE fragment must not end with trailing comma (invalid before SELECT): %s", cte)
	}
	if !strings.HasSuffix(strings.TrimSpace(cte), ")") {
		t.Fatalf("CTE fragment should end with closing paren: %s", cte)
	}
	// Chained CTEs: comma belongs between definitions, not after the only CTE.
	chained := cte + `,
		total AS (SELECT 1)`
	if strings.Contains(chained, "), SELECT") || strings.Contains(chained, "),\n\t\tSELECT") {
		t.Fatalf("chained SQL must not have dangling comma before SELECT: %s", chained)
	}
}

func TestSqlServerQueryAnalysisClassificationFilter(t *testing.T) {
	cte := sqlServerQueryAnalysisQualifyingHashesCTE(true, []string{"mon"}, true, 4)
	if !strings.Contains(cte, "qualifying_hashes") || strings.Contains(cte, "LATERAL") {
		t.Fatal("exclude_system=true should use qualifying_hashes CTE, not LATERAL")
	}
	if !strings.Contains(sqlServerQueryAnalysisHistoryHashJoin(), "qualifying_hashes") {
		t.Fatal("history join should reference qualifying_hashes CTE")
	}
	join, scope := sqlServerQueryAnalysisMetricsLateralJoin(true, "qh", []string{"mon"})
	if !strings.Contains(join, "INNER JOIN LATERAL") || strings.Contains(scope, "qm.") {
		t.Fatalf("lateral join helper retained for legacy paths, outer scope=%q", scope)
	}
	// Join must be passed as a fmt.Sprintf argument, not embedded in the format string (ILIKE '%…%').
	built := fmt.Sprintf(`FROM t %s WHERE 1=1`, join)
	if strings.Contains(built, "%!") {
		t.Fatalf("fmt.Sprintf must not interpret ILIKE wildcards in join SQL")
	}
	if !strings.Contains(built, `'%sys.dm_%'`) {
		t.Fatal("expected literal sys.dm ILIKE pattern in built SQL")
	}

	if sqlServerQueryAnalysisClassificationFilter(false, "q.") != "" {
		t.Fatal("classification filter off when include system")
	}
	if !strings.Contains(sqlServerQueryAnalysisClassificationFilter(true, "q."), "SYSTEM") {
		t.Fatal("classification filter on when exclude system")
	}
}

// TestSqlServerQualifyingHashesCTE_extendedLookback verifies the CTE widens its lower time
// bound by 1 hour before $2 to survive narrow selected ranges and collection-timing skew.
// Without this, a freshly set-up instance or a sub-5-minute query window could yield an
// empty qualifying_hashes set and silently return zero rows from the history join.
func TestSqlServerQualifyingHashesCTE_extendedLookback(t *testing.T) {
	cte := sqlServerQueryAnalysisQualifyingHashesCTE(true, []string{"mon_user"}, false, 0)
	if !strings.Contains(cte, "($2::timestamptz - INTERVAL '1 hour')") {
		t.Fatalf("CTE must extend lower bound by 1 hour to handle collection skew: %s", cte)
	}
	if !strings.Contains(cte, "$3::timestamptz") {
		t.Fatalf("CTE must still cap at $3 (range end): %s", cte)
	}
	if !strings.Contains(cte, "is_user_workload") {
		t.Fatalf("CTE must still require is_user_workload = 1: %s", cte)
	}
	// excludeSystem=false must return empty (CTE not needed for the broad view)
	if empty := sqlServerQueryAnalysisQualifyingHashesCTE(false, nil, false, 0); empty != "" {
		t.Fatalf("CTE should be empty when excludeSystem=false: %q", empty)
	}
}

// capturedQueryRow represents a row as the SQL Server collector inserts into sqlserver_query_metrics_v2.
type capturedQueryRow struct {
	queryText       string
	statementText   string
	applicationName string
	loginName       string
	databaseName    string
	isUserWorkload  int
}

// simulateSystemNoiseFilter returns true if the row matches any exclusion predicate from
// sqlServerQueryTextSystemNoiseSQL or sqlServerCollectorSQLExcludeSQL / sqlServerMonitoringAppExcludeSQL.
// This mirrors the SQL ILIKE / LIKE logic in Go so tests can run without a live database.
func simulateSystemNoiseFilter(r capturedQueryRow) bool {
	lowered := strings.ToLower(r.queryText + " " + r.statementText)
	uppered := strings.ToUpper(r.queryText + " " + r.statementText)

	// Collector tag (/* SQL_OPTIMA)
	if strings.Contains(lowered, "/* sql_optima") {
		return true
	}
	// DMV and catalog patterns
	dmvPatterns := []string{
		"sys.dm_", "sys.partitions", "sys.plan_", "sys.all_objects",
		"sys.dm_exec_query_stats", "sys.dm_exec_sql_text", "sys.dm_exec_plan_attributes",
		"is_ms_shipped", "sp_mshistory_cleanup", "(@_msparam_0",
	}
	for _, p := range dmvPatterns {
		if strings.Contains(lowered, p) {
			return true
		}
	}
	if strings.Contains(lowered, "db_id()") &&
		(strings.Contains(lowered, "system_type_id") || strings.Contains(lowered, "is_inlineable") || strings.Contains(lowered, "vector_column_count")) {
		return true
	}
	// UPPER patterns (mirrors sqlServerQueryTextSystemNoiseSQL upper() block)
	if strings.Contains(uppered, "BACKUP DATABASE") || strings.Contains(uppered, "RESTORE DATABASE") {
		return true
	}
	if strings.Contains(uppered, "SELECT") {
		for _, schema := range []string{"FROM SYS.", "FROM [SYS].", "FROM MSDB.", "FROM INFORMATION_SCHEMA."} {
			if strings.Contains(uppered, schema) {
				return true
			}
		}
	}
	// Monitoring application exclusion (mirrors sqlServerMonitoringAppExcludeSQL)
	app := strings.ToLower(strings.TrimSpace(r.applicationName))
	if app == "sql-optima" || app == "go-mssqldb" || app == "sqlserverms" {
		return true
	}
	return false
}

// TestSqlServerFilter_CapturedQuerySimulation uses representative query rows that mirror what
// the SQL Server collector writes into sqlserver_query_metrics_v2 to verify the read-time
// filter correctly admits user workload and rejects monitoring / system noise.
func TestSqlServerFilter_CapturedQuerySimulation(t *testing.T) {
	// User-application queries: must NOT be excluded by any noise predicate.
	userQueries := []capturedQueryRow{
		{
			queryText:       "SELECT order_id, customer_id, total_amount FROM orders WHERE customer_id = @p1 AND status = @p2",
			applicationName: "ECommerceApp", loginName: "app_user", databaseName: "AppDB", isUserWorkload: 1,
		},
		{
			queryText:       "INSERT INTO order_items (order_id, product_id, qty, unit_price) VALUES (@p1, @p2, @p3, @p4)",
			applicationName: "ECommerceApp", loginName: "ecom_svc", databaseName: "AppDB", isUserWorkload: 1,
		},
		{
			queryText:       "UPDATE customers SET last_login = @p1, login_count = login_count + 1 WHERE id = @p2",
			applicationName: "AuthService", loginName: "auth_user", databaseName: "AppDB", isUserWorkload: 1,
		},
		{
			queryText:       "DELETE FROM session_tokens WHERE expires_at < @p1",
			applicationName: "SessionManager", loginName: "app_user", databaseName: "AppDB", isUserWorkload: 1,
		},
		{
			queryText:       "EXEC sp_GetProductsByCategory @category_id = @p1, @page = @p2, @page_size = @p3",
			applicationName: "ProductCatalog", loginName: "catalog_svc", databaseName: "AppDB", isUserWorkload: 1,
		},
		{
			queryText:       "SELECT p.product_id, p.name, i.qty_on_hand FROM products p JOIN inventory i ON i.product_id = p.product_id WHERE i.qty_on_hand < @p1",
			applicationName: "WarehouseApp", loginName: "wh_user", databaseName: "AppDB", isUserWorkload: 1,
		},
	}
	for _, q := range userQueries {
		if simulateSystemNoiseFilter(q) {
			preview := q.queryText
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			t.Errorf("user query incorrectly excluded — app=%q query=%q", q.applicationName, preview)
		}
	}

	// Monitoring / system queries: must be excluded by at least one noise predicate.
	type monQ struct {
		name string
		row  capturedQueryRow
	}
	monitoringQueries := []monQ{
		{
			name: "sql_optima_collector_batch",
			row: capturedQueryRow{
				queryText:       "/* SQL_OPTIMA: query_stats_v2 */ SELECT qs.query_hash, qs.total_cpu_ms FROM sys.dm_exec_query_stats qs CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) st",
				applicationName: "sql-optima", loginName: "dbmonitor_user", databaseName: "master",
			},
		},
		{
			name: "dmv_exec_requests",
			row: capturedQueryRow{
				queryText:       "SELECT r.session_id, r.status, r.wait_type, r.blocking_session_id FROM sys.dm_exec_requests r WHERE r.session_id > 50",
				applicationName: "Monitoring", loginName: "mon_login", databaseName: "master",
			},
		},
		{
			name: "catalog_all_objects_is_ms_shipped",
			row: capturedQueryRow{
				queryText:       "SELECT o.object_id, o.name, o.is_ms_shipped, o.type FROM sys.all_objects o WHERE o.type IN ('U','V')",
				applicationName: "SSMS", loginName: "sa", databaseName: "master",
			},
		},
		{
			name: "backup_database",
			row: capturedQueryRow{
				queryText:       "BACKUP DATABASE [MyAppDB] TO DISK = N'C:\\Backups\\MyAppDB_Full.bak' WITH NOFORMAT, NOINIT, NAME = 'Full Backup'",
				applicationName: "SQLAgent", loginName: "sa", databaseName: "master",
			},
		},
		{
			name: "information_schema_columns",
			row: capturedQueryRow{
				queryText:       "SELECT TABLE_NAME, COLUMN_NAME, DATA_TYPE FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = 'dbo' ORDER BY TABLE_NAME",
				applicationName: "SSMS", loginName: "dba_user", databaseName: "AppDB",
			},
		},
		{
			name: "sys_select",
			row: capturedQueryRow{
				queryText:       "SELECT name, database_id, state_desc FROM SYS.DATABASES WHERE state_desc = 'ONLINE'",
				applicationName: "InternalTool", loginName: "sa", databaseName: "master",
			},
		},
		{
			name: "go_mssqldb_monitoring_app",
			row: capturedQueryRow{
				queryText:       "SELECT 1",
				applicationName: "go-mssqldb", loginName: "mon_login", databaseName: "master",
			},
		},
		{
			name: "sp_mshistory_cleanup",
			row: capturedQueryRow{
				queryText:       "EXEC sp_mshistory_cleanup @retention = 48",
				applicationName: "SQLAgent", loginName: "sa", databaseName: "msdb",
			},
		},
	}
	for _, tc := range monitoringQueries {
		if !simulateSystemNoiseFilter(tc.row) {
			t.Errorf("monitoring/system query %q was NOT excluded — app=%q query=%q",
				tc.name, tc.row.applicationName, func() string {
					if len(tc.row.queryText) > 80 {
						return tc.row.queryText[:80] + "..."
					}
					return tc.row.queryText
				}())
		}
	}
}

// TestSqlServerUserWorkloadNoiseSQL_userQueryPassthrough verifies the generated SQL filter
// string contains all required exclusion patterns without dropping any CRUD-shaped SQL.
func TestSqlServerUserWorkloadNoiseSQL_userQueryPassthrough(t *testing.T) {
	f := sqlServerUserWorkloadNoiseSQL("q.", []string{"mon_user"})

	required := []string{
		"q.query_text_raw", "q.statement_text",
		"%/* SQL_OPTIMA%",
		"sys.dm_",
		"is_ms_shipped",
		"COALESCE(q.is_user_workload, 0) = 1",
		"sql-optima", "go-mssqldb", "sqlserverms",
	}
	for _, want := range required {
		if !strings.Contains(f, want) {
			t.Errorf("user-workload noise filter missing required pattern %q", want)
		}
	}

	// Patterns that must NOT appear (would incorrectly exclude normal DML)
	forbidden := []string{"NOT LIKE 'SET %'", "total_cpu_ms > 1", "NOT LIKE 'SELECT 1'"}
	for _, bad := range forbidden {
		if strings.Contains(f, bad) {
			t.Errorf("user-workload noise filter contains over-broad exclusion %q", bad)
		}
	}
}
