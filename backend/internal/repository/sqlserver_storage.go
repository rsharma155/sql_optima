// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server storage and database size metrics.
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

type TableSizeStats struct {
	SchemaName string
	TableName  string
	RowCount   int64
	TotalMB    float64
	DataMB     float64
	IndexMB    float64
}

func (c *SqlServerRepository) CollectSQLServerStorageMetrics(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		/* SQL_OPTIMA */ SELECT   
			DB_NAME(database_id) AS database_name,
			SUM(CASE WHEN type_desc = 'ROWS' THEN size * 8 / 1024.0 ELSE 0 END) AS data_size_mb,
			SUM(CASE WHEN type_desc = 'LOG' THEN size * 8 / 1024.0 ELSE 0 END) AS log_size_mb,
			SUM(size * 8 / 1024.0) AS total_size_mb
		FROM sys.master_files WITH (NOLOCK)
		WHERE database_id > 4 AND state = 0
		  AND LOWER(DB_NAME(database_id)) <> N'distribution'
		GROUP BY database_id
	`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var dbName string
		var dataMB, logMB, totalMB float64
		if err := rows.Scan(&dbName, &dataMB, &logMB, &totalMB); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"database_name": dbName,
			"data_size_mb":  dataMB,
			"log_size_mb":   logMB,
			"total_size_mb": totalMB,
		})
	}
	return results, nil
}

func (c *SqlServerRepository) FetchTableSizeStats(ctx context.Context, instanceName, databaseName string) ([]TableSizeStats, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := fmt.Sprintf(`
		/* SQL_OPTIMA */ 
		USE [%s];
		SELECT /* SQL_OPTIMA */   
			SCHEMA_NAME(t.schema_id) AS schema_name,
			t.name AS table_name,
			p.rows AS row_count,
			SUM(a.total_pages) * 8 / 1024.0 AS total_mb,
			SUM(a.data_pages) * 8 / 1024.0 AS data_mb,
			(SUM(a.used_pages) - SUM(a.data_pages)) * 8 / 1024.0 AS index_mb
		FROM sys.tables t WITH (NOLOCK)
		INNER JOIN sys.indexes i WITH (NOLOCK) ON t.object_id = i.object_id
		INNER JOIN sys.partitions p WITH (NOLOCK) ON i.object_id = p.object_id AND i.index_id = p.index_id
		INNER JOIN sys.allocation_units a WITH (NOLOCK) ON p.partition_id = a.container_id
		WHERE t.is_ms_shipped = 0
		GROUP BY t.schema_id, t.name, p.rows
		ORDER BY total_mb DESC
	`, strings.ReplaceAll(databaseName, "]", "]]"))

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TableSizeStats
	for rows.Next() {
		var st TableSizeStats
		if err := rows.Scan(&st.SchemaName, &st.TableName, &st.RowCount, &st.TotalMB, &st.DataMB, &st.IndexMB); err != nil {
			continue
		}
		results = append(results, st)
	}
	return results, nil
}
