// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server database listing and utilities.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
	"strings"
)

func (c *SqlServerRepository) ListSQLServerUserDatabases(instanceName string) ([]string, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	const q = `
		SELECT /* SQL_OPTIMA */   d.name
		FROM sys.databases d
		WHERE d.database_id > 4
		  AND d.state = 0
		  AND LOWER(d.name) <> N'distribution'
		ORDER BY d.name
	`
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			continue
		}
		n = strings.TrimSpace(n)
		if n != "" {
			names = append(names, n)
		}
	}
	return names, rows.Err()
}

func sqlServerQuoteBracket(ident string) string {
	if ident == "" {
		return "[]"
	}
	return "[" + strings.ReplaceAll(ident, "]", "]]") + "]"
}
