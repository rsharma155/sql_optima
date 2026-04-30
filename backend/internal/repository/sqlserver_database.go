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
		/* SQL_OPTIMA */ SELECT d.name
		FROM sys.databases AS d
		WHERE d.database_id > 4
		AND d.state_desc = N'ONLINE'
		AND d.name <> N'distribution'
		AND d.source_database_id IS NULL
		AND HAS_DBACCESS(d.name) = 1
		ORDER BY d.name;
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
