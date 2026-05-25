// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for SQL Server read-time query filters.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"strings"
	"testing"
)

func TestSqlServerCollectorSQLExcludeSQL_usesQueryTextRaw(t *testing.T) {
	f := sqlServerCollectorSQLExcludeSQL("q.")
	if !strings.Contains(f, "q.query_text_raw") {
		t.Fatalf("expected filter on query_text_raw: %s", f)
	}
	if !strings.Contains(f, "%/* SQL_OPTIMA%") {
		t.Fatalf("expected /* SQL_OPTIMA tag match: %s", f)
	}
	if strings.Contains(f, "statement_text") {
		t.Fatal("monitoring collector filter must not use statement_text")
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
	if strings.Contains(checked, "is_user_workload") {
		t.Fatal("checked filter should not gate on is_user_workload (CRUD batches must not be dropped)")
	}
	if strings.Contains(checked, "NOT LIKE 'SET %'") {
		t.Fatal("checked filter must not exclude SET batches (normal CRUD)")
	}
	if !strings.Contains(checked, "'my_monitor'") {
		t.Fatal("checked filter should exclude optima_servers monitoring login")
	}
	if strings.Contains(checked, "total_cpu_ms > 1") {
		t.Fatal("cpu threshold heuristic must not be used")
	}
	if strings.Contains(checked, "statement_text") {
		t.Fatal("system SQL heuristics should use query_text_raw only, not statement_text")
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
	if sqlServerQueryAnalysisClassificationFilter(false, "q.") != "" {
		t.Fatal("classification filter off when include system")
	}
	if !strings.Contains(sqlServerQueryAnalysisClassificationFilter(true, "q."), "SYSTEM") {
		t.Fatal("classification filter on when exclude system")
	}
}
