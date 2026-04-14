// Package sqlsandbox enforces read-only, bounded SQL for rules, widgets, and ad-hoc execution paths.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL query sandbox for safe query execution with security boundaries.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package sqlsandbox

import (
	"fmt"
	"regexp"
	"strings"
)

const DefaultMaxRows = 5000

var (
	dangerousKeywords = regexp.MustCompile(`(?is)\b(INSERT|UPDATE|DELETE|MERGE|INTO\s|DROP|ALTER|CREATE|TRUNCATE|GRANT|REVOKE|COPY\s|CALL\s|DO\s*\(|EXECUTE\s+IMMEDIATE|xp_|sp_executesql|sp_configure|BACKUP|RESTORE|SHUTDOWN|DBCC\s|BULK\s+INSERT)\b`)
)

// Options tune validation (timeouts are enforced by callers via context).
type Options struct {
	Dialect  string // postgres | sqlserver
	MaxRows  int
	AllowCTE bool // always true for postgres WITH
}

// ValidateReadOnly returns an error if sql is not acceptable for monitoring rule / widget execution.
func ValidateReadOnly(opt Options, sql string) error {
	if opt.MaxRows <= 0 {
		opt.MaxRows = DefaultMaxRows
	}
	s := strings.TrimSpace(sql)
	if s == "" {
		return fmt.Errorf("sqlsandbox: empty SQL")
	}
	if strings.Count(s, ";") > 0 {
		// Disallow multi-statement batches; allow single trailing semicolon.
		stripped := strings.TrimSuffix(s, ";")
		stripped = strings.TrimSpace(stripped)
		if strings.Contains(stripped, ";") {
			return fmt.Errorf("sqlsandbox: multiple statements are not allowed")
		}
		s = stripped
	}
	if dangerousKeywords.MatchString(s) {
		return fmt.Errorf("sqlsandbox: disallowed keyword or construct in SQL")
	}

	u := strings.ToUpper(s)
	switch opt.Dialect {
	case "postgres", "":
		if !(strings.HasPrefix(u, "SELECT") || strings.HasPrefix(u, "WITH") || strings.HasPrefix(u, "(")) {
			return fmt.Errorf("sqlsandbox: postgres SQL must begin with SELECT, WITH, or (")
		}
	case "sqlserver":
		if !(strings.HasPrefix(u, "SELECT") || strings.HasPrefix(u, "WITH")) {
			return fmt.Errorf("sqlsandbox: SQL Server SQL must begin with SELECT or WITH")
		}
	default:
		return fmt.Errorf("sqlsandbox: unknown dialect %q", opt.Dialect)
	}
	return nil
}

// WrapWithRowLimit wraps validated SQL so the engine cannot return unbounded rows.
func WrapWithRowLimit(dialect, sql string, maxRows int) (string, error) {
	if maxRows <= 0 {
		maxRows = DefaultMaxRows
	}
	s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sql), ";"))
	if err := ValidateReadOnly(Options{Dialect: dialect, MaxRows: maxRows}, s); err != nil {
		return "", err
	}
	switch dialect {
	case "postgres", "":
		return fmt.Sprintf("SELECT * FROM (%s) AS _optima_sandbox LIMIT %d", s, maxRows), nil
	case "sqlserver":
		return fmt.Sprintf("SELECT TOP (%d) * FROM (%s) AS _optima_sandbox", maxRows, s), nil
	default:
		return "", fmt.Errorf("sqlsandbox: unknown dialect %q", dialect)
	}
}
