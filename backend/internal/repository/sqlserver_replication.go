// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server replication status monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"fmt"
	"strings"
)

func (c *SqlServerRepository) FetchReplicationStatus(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return []map[string]interface{}{}, nil
	}

	dbNames, err := c.ListSQLServerUserDatabases(ctx, instanceName)
	if err != nil {
		return []map[string]interface{}{}, nil
	}

	var results []map[string]interface{}
	for _, dbn := range dbNames {
		qb := sqlServerQuoteBracket(dbn)
		dbNameLit := strings.ReplaceAll(dbn, "'", "''")
		query := fmt.Sprintf(`
			/* SQL_OPTIMA */ SELECT   
				'Publication' as type,
				p.name as name,
				'%s' as database_name,
				CASE p.repl_freq WHEN 0 THEN 'Continuous' ELSE 'Scheduled' END as details,
				(SELECT /* SQL_OPTIMA */   COUNT(*) FROM %s.dbo.sysarticles WHERE pubid = p.pubid) as article_count,
				'N/A' as status
			FROM %s.dbo.syspublications p
			UNION ALL
			SELECT /* SQL_OPTIMA */   
				'Subscription' as type,
				s.dest_db as name,
				'%s' as database_name,
				s.srvname as details,
				0 as article_count,
				CASE s.status WHEN 0 THEN 'Inactive' WHEN 1 THEN 'Subscribed' WHEN 2 THEN 'Active' ELSE 'Unknown' END as status
			FROM %s.dbo.syssubscriptions s
		`, dbNameLit, qb, qb, dbNameLit, qb)

		ctx, cancel := WithQueryTimeout(ctx, 0)
		defer cancel()
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			continue
		}
		for rows.Next() {
			var rType, name, dbName, details, status string
			var articles int
			if err := rows.Scan(&rType, &name, &dbName, &details, &articles, &status); err == nil {
				results = append(results, map[string]interface{}{
					"type":          rType,
					"name":          name,
					"database":      dbName,
					"details":       details,
					"article_count": articles,
					"status":        status,
				})
			}
		}
		rows.Close()
	}

	return results, nil
}
