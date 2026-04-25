// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server connection statistics by application.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"fmt"
	"strings"
)

func (c *SqlServerRepository) CollectConnectionStats(db *sql.DB, database string) ([]map[string]interface{}, error) {
	dbName := strings.TrimSpace(database)
	if dbName != "" {
		dbName = fmt.Sprintf("AND DB_NAME(r.database_id) = '%s'", strings.ReplaceAll(dbName, "'", "''"))
	}

	query := fmt.Sprintf(`
		SELECT /* SQL_OPTIMA */   
			s.login_name,
			DB_NAME(r.database_id) AS database_name,
			COUNT(*) AS active_connections,
			SUM(CASE WHEN r.status = 'running' THEN 1 ELSE 0 END) AS active_requests
		FROM sys.dm_exec_sessions s
		LEFT JOIN sys.dm_exec_requests r ON s.session_id = r.session_id
		WHERE s.is_user_process = 1
		  AND s.session_id > 50
		  %s
		GROUP BY s.login_name, r.database_id
		ORDER BY active_connections DESC
	`, dbName)

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var loginName, databaseName string
		var activeConns, activeReqs int
		if err := rows.Scan(&loginName, &databaseName, &activeConns, &activeReqs); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"login_name":         loginName,
			"database_name":      databaseName,
			"active_connections": activeConns,
			"active_requests":    activeReqs,
		})
	}
	return results, rows.Err()
}
