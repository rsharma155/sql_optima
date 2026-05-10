// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Classifies pg_stat_statements query text into a single-char type code.
//
//	Used at delta-insertion time to enable query-type filtering in the dashboard.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import "strings"

// ClassifyQueryType returns a single-char code for the leading DML/DDL verb:
//
//	S = SELECT or WITH (read)
//	I = INSERT
//	U = UPDATE
//	D = DELETE
//	E = DDL (CREATE / ALTER / DROP / TRUNCATE)
//	O = Other (CALL, MERGE, COPY, unknown)
func ClassifyQueryType(queryText string) string {
	q := strings.TrimSpace(strings.ToUpper(queryText))
	switch {
	case strings.HasPrefix(q, "SELECT") || strings.HasPrefix(q, "WITH"):
		return "S"
	case strings.HasPrefix(q, "INSERT"):
		return "I"
	case strings.HasPrefix(q, "UPDATE"):
		return "U"
	case strings.HasPrefix(q, "DELETE"):
		return "D"
	case strings.HasPrefix(q, "CREATE") || strings.HasPrefix(q, "ALTER") ||
		strings.HasPrefix(q, "DROP") || strings.HasPrefix(q, "TRUNCATE"):
		return "E"
	default:
		return "O"
	}
}
