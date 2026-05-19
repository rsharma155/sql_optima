// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Test suite for SQL sandbox functionality.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package sqlsandbox

import (
	"strings"
	"testing"
)

func TestValidateReadOnly_Postgres(t *testing.T) {
	err := ValidateReadOnly(Options{Dialect: "postgres"}, "SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	if ValidateReadOnly(Options{Dialect: "postgres"}, "DELETE FROM t") == nil {
		t.Fatal("expected error")
	}
	if ValidateReadOnly(Options{Dialect: "postgres"}, "WITH x AS (SELECT 1) SELECT * FROM x") != nil {
		t.Fatal("expected CTE ok")
	}
}

func TestWrapWithRowLimit(t *testing.T) {
	// Postgres wrapping
	out, err := WrapWithRowLimit("postgres", "SELECT n FROM generate_series(1,3) n", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "LIMIT 10") {
		t.Fatalf("got %q", out)
	}

	// SQL Server wrapping (standard)
	out, err = WrapWithRowLimit("sqlserver", "SELECT name FROM sys.databases", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "SELECT TOP (10) * FROM") {
		t.Fatalf("expected TOP (10) wrapping for sqlserver, got %q", out)
	}

	// SQL Server CTE (no wrapping)
	cte := "WITH DBs AS (SELECT name FROM sys.databases) SELECT * FROM DBs"
	out, err = WrapWithRowLimit("sqlserver", cte, 10)
	if err != nil {
		t.Fatal(err)
	}
	if out != cte {
		t.Fatalf("expected no wrapping for SQL Server CTE, got %q", out)
	}
}
