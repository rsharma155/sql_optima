// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Statement fingerprinting for SQL Server Query Analysis top-query rollup.
//          Groups parameterized / ad hoc variants of the same logical statement.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

var (
	sqlWhitespaceCollapse   = regexp.MustCompile(`\s+`)
	sqlSingleQuotedLiteral  = regexp.MustCompile(`'([^']|'')*'`)
	sqlUnicodeQuotedLiteral = regexp.MustCompile(`(?i)N'([^']|'')*'`)
	sqlNumericLiteral       = regexp.MustCompile(`\b\d+(\.\d+)?\b`)
)

// NormalizeStatementTextForFingerprint collapses literals so the same logical SQL maps to one key.
func NormalizeStatementTextForFingerprint(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}
	t = sqlWhitespaceCollapse.ReplaceAllString(t, " ")
	t = sqlUnicodeQuotedLiteral.ReplaceAllString(t, "N'?'")
	t = sqlSingleQuotedLiteral.ReplaceAllString(t, "'?'")
	t = sqlNumericLiteral.ReplaceAllString(t, "?")
	return strings.TrimSpace(t)
}

func isPlanEvictedOrEmptyStatement(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return true
	}
	return strings.EqualFold(t, "plan evicted")
}

// StatementFingerprint returns a stable hex digest for normalized statement text (md5, matches SQL rollup).
func StatementFingerprint(text string) string {
	norm := NormalizeStatementTextForFingerprint(text)
	if norm == "" {
		return ""
	}
	sum := md5.Sum([]byte(norm))
	return hex.EncodeToString(sum[:])
}

// StatementFingerprintKey returns the rollup group key for a hypertable row.
// Empty or plan-evicted text falls back to per-query_hash grouping.
func StatementFingerprintKey(statementText, queryTextRaw string, queryHash int64) string {
	text := strings.TrimSpace(statementText)
	if text == "" {
		text = strings.TrimSpace(queryTextRaw)
	}
	if isPlanEvictedOrEmptyStatement(text) {
		return fmt.Sprintf("hash:%d", queryHash)
	}
	if fp := StatementFingerprint(text); fp != "" {
		return fp
	}
	return fmt.Sprintf("hash:%d", queryHash)
}

// sqlServerQueryAnalysisStatementTextExpr picks the best available SQL text column.
func sqlServerQueryAnalysisStatementTextExpr(col string) string {
	return `COALESCE(NULLIF(TRIM(` + col + `statement_text), ''), NULLIF(TRIM(` + col + `query_text_raw), ''))`
}

// sqlServerQueryAnalysisFingerprintExpr mirrors NormalizeStatementTextForFingerprint in SQL (md5 hex).
func sqlServerQueryAnalysisFingerprintExpr(col string) string {
	text := sqlServerQueryAnalysisStatementTextExpr(col)
	normalized := `regexp_replace(
		regexp_replace(
			regexp_replace(
				regexp_replace(lower(` + text + `), E'\\s+', ' ', 'g'),
				E'n''([^'']|'')*''', E'n''?''', 'gi'),
			E'''([^'']|'')*''', E'''?''', 'g'),
		E'\\m\\d+(\\.\\d+)?\\M', '?', 'g')`
	return `CASE
		WHEN ` + text + ` IS NULL OR ` + text + ` = '' OR lower(` + text + `) = 'plan evicted'
		THEN 'hash:' || ` + col + `query_hash::text
		ELSE md5(` + normalized + `)
	END`
}

// sqlServerQueryAnalysisGroupByFingerprintSQL groups top queries by statement pattern (Query Analysis only).
func sqlServerQueryAnalysisGroupByFingerprintSQL(col string, databaseScoped bool) string {
	fp := sqlServerQueryAnalysisFingerprintExpr(col)
	if databaseScoped {
		return ` GROUP BY ` + fp
	}
	return ` GROUP BY ` + fp + `, LOWER(TRIM(COALESCE(` + col + `database_name, 'unknown')))`
}

// sqlServerQueryAnalysisFingerprintSelectSQL is the SELECT list for fingerprint-based Query Analysis rollup.
func sqlServerQueryAnalysisFingerprintSelectSQL(col string) string {
	fp := sqlServerQueryAnalysisFingerprintExpr(col)
	text := sqlServerQueryAnalysisStatementTextExpr(col)
	return `
		` + fp + ` AS statement_fingerprint,
		(array_agg(` + col + `query_hash ORDER BY ` + col + `total_cpu_ms DESC NULLS LAST))[1] AS query_hash,
		COUNT(DISTINCT ` + col + `query_hash)::bigint AS hash_variant_count,
		(array_agg(` + text + ` ORDER BY length(COALESCE(` + text + `, '')) DESC NULLS LAST))[1] AS statement_text,
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
