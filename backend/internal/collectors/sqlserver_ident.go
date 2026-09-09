// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server identifier quoting helpers for catalog queries that must
// target a user database without relying on USE (which can yield an empty first
// result set under database/sql + TDS).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import "strings"

func sqlServerBracketIdent(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

func sqlServerNString(name string) string {
	return "N'" + strings.ReplaceAll(name, "'", "''") + "'"
}
