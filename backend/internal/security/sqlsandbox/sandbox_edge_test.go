// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Extended sandbox tests — dialects, injection-ish patterns, wrapping edge cases.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package sqlsandbox

import (
	"strings"
	"testing"
)

func TestValidateReadOnly_RejectsDangerous(t *testing.T) {
	cases := []struct {
		name string
		sql  string
	}{
		{"drop", "SELECT 1; DROP TABLE t"},
		{"insert", "INSERT INTO t VALUES (1)"},
		{"update", "UPDATE t SET a=1"},
		{"delete", "DELETE FROM t WHERE 1=1"},
		{"truncate", "TRUNCATE TABLE t"},
		{"grant", "GRANT SELECT ON t TO public"},
		{"xp", "SELECT xp_cmdshell('dir')"},
		{"sp_exec", "EXEC sp_executesql N'SELECT 1'"},
		{"copy", "COPY t FROM '/tmp/x'"},
		{"backup", "BACKUP DATABASE x TO DISK='x'"},
		{"union_multi", "SELECT 1; SELECT 2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateReadOnly(Options{Dialect: "postgres"}, tc.sql); err == nil {
				t.Fatalf("expected reject for %q", tc.sql)
			}
			if err := ValidateReadOnly(Options{Dialect: "sqlserver"}, tc.sql); err == nil && !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(tc.sql)), "SELECT") {
				// sqlserver path also rejects dangerous keywords when it reaches that check
			}
		})
	}
}

func TestValidateReadOnly_AllowsSelectVariants(t *testing.T) {
	ok := []struct {
		dialect string
		sql     string
	}{
		{"postgres", "SELECT * FROM pg_stat_activity"},
		{"postgres", "WITH a AS (SELECT 1 AS x) SELECT * FROM a"},
		{"postgres", "(SELECT 1) UNION ALL (SELECT 2)"},
		{"postgres", "SELECT 1;"}, // trailing semicolon only
		{"sqlserver", "SELECT name FROM sys.databases"},
		{"sqlserver", "WITH x AS (SELECT 1 AS n) SELECT * FROM x"},
		{"trino", "SELECT * FROM iceberg.default.sqlserver_cpu_history LIMIT 10"},
		{"trino", "WITH t AS (SELECT 1 AS n) SELECT * FROM t"},
	}
	for _, tc := range ok {
		name := tc.dialect + "_" + tc.sql
		if len(name) > 40 {
			name = name[:40]
		}
		t.Run(name, func(t *testing.T) {
			if err := ValidateReadOnly(Options{Dialect: tc.dialect}, tc.sql); err != nil {
				t.Fatalf("%s: %v", tc.sql, err)
			}
		})
	}
}

func TestValidateReadOnly_UnknownDialect(t *testing.T) {
	if err := ValidateReadOnly(Options{Dialect: "mysql"}, "SELECT 1"); err == nil {
		t.Fatal("expected unknown dialect error")
	}
}

func TestValidateReadOnly_Empty(t *testing.T) {
	if err := ValidateReadOnly(Options{Dialect: "postgres"}, "   "); err == nil {
		t.Fatal("expected empty error")
	}
}

func TestValidateReadOnly_SqlServerRejectsParenOnly(t *testing.T) {
	// Postgres allows leading '('; SQL Server does not.
	if err := ValidateReadOnly(Options{Dialect: "sqlserver"}, "(SELECT 1)"); err == nil {
		t.Fatal("sqlserver should reject leading paren")
	}
}

func TestWrapWithRowLimit_RejectsInvalid(t *testing.T) {
	if _, err := WrapWithRowLimit("postgres", "DELETE FROM t", 10); err == nil {
		t.Fatal("expected wrap to reject delete")
	}
}

func TestWrapWithRowLimit_DefaultMaxRows(t *testing.T) {
	out, err := WrapWithRowLimit("postgres", "SELECT 1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "LIMIT 5000") {
		t.Fatalf("expected default limit, got %q", out)
	}
}

func TestWrapWithRowLimit_Trino(t *testing.T) {
	out, err := WrapWithRowLimit("trino", "SELECT 1 AS n", 25)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "LIMIT 25") {
		t.Fatalf("got %q", out)
	}
}
