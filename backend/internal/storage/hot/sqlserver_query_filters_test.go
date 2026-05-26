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

func TestSqlServerQueryAnalysisClassificationFilter(t *testing.T) {
	join, scope := sqlServerQueryAnalysisMetricsLateralJoin(true, "qh", []string{"mon"})
	if !strings.Contains(join, "INNER JOIN LATERAL") || strings.Contains(scope, "qm.") {
		t.Fatalf("exclude_system=true should filter inside lateral join, outer scope=%q", scope)
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
