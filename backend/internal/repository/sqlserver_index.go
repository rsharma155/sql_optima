// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server index usage and fragmentation metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
	"strings"
)

func (c *SqlServerRepository) CollectSQLServerIndexUsageMetrics(instanceName, databaseName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := fmt.Sprintf(`
		/* SQL_OPTIMA */ 
		USE [%s];
		SELECT   
			DB_NAME() AS database_name,
			SCHEMA_NAME(t.schema_id) AS schema_name,
			t.name AS table_name,
			i.name AS index_name,
			i.type_desc AS index_type,
			ISNULL(s.user_seeks, 0) AS user_seeks,
			ISNULL(s.user_scans, 0) AS user_scans,
			ISNULL(s.user_lookups, 0) AS user_lookups,
			ISNULL(s.user_updates, 0) AS user_updates,
			(SELECT /* SQL_OPTIMA */   SUM(a.total_pages) * 8 / 1024.0 FROM sys.partitions p WITH (NOLOCK) JOIN sys.allocation_units a WITH (NOLOCK) ON p.partition_id = a.container_id WHERE p.object_id = i.object_id AND p.index_id = i.index_id) AS index_size_mb
		FROM sys.indexes i WITH (NOLOCK)
		INNER JOIN sys.tables t WITH (NOLOCK) ON i.object_id = t.object_id
		LEFT JOIN sys.dm_db_index_usage_stats s WITH (NOLOCK) ON s.object_id = i.object_id AND s.index_id = i.index_id AND s.database_id = DB_ID()
		WHERE t.is_ms_shipped = 0 AND i.name IS NOT NULL
	`, strings.ReplaceAll(databaseName, "]", "]]"))

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var dbn, sn, tn, in, it string
		var seeks, scans, lookups, updates int64
		var sizeMB float64
		if err := rows.Scan(&dbn, &sn, &tn, &in, &it, &seeks, &scans, &lookups, &updates, &sizeMB); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"database_name": dbn, "schema_name": sn, "table_name": tn, "index_name": in, "index_type": it,
			"user_seeks": seeks, "user_scans": scans, "user_lookups": lookups, "user_updates": updates, "index_size_mb": sizeMB,
		})
	}
	return results, nil
}

func (c *SqlServerRepository) CollectSQLServerIndexFragmentationMetrics(instanceName, databaseName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := fmt.Sprintf(`
		/* SQL_OPTIMA */ 	
		USE [%s];
		SELECT   
			DB_NAME() AS database_name,
			SCHEMA_NAME(t.schema_id) AS schema_name,
			t.name AS table_name,
			i.name AS index_name,
			s.avg_fragmentation_in_percent,
			s.page_count
		FROM sys.dm_db_index_physical_stats(DB_ID(), NULL, NULL, NULL, 'LIMITED') s
		INNER JOIN sys.indexes i WITH (NOLOCK) ON s.object_id = i.object_id AND s.index_id = i.index_id
		INNER JOIN sys.tables t WITH (NOLOCK) ON i.object_id = t.object_id
		WHERE t.is_ms_shipped = 0 AND i.name IS NOT NULL AND s.page_count > 100
	`, strings.ReplaceAll(databaseName, "]", "]]"))

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var dbn, sn, tn, in string
		var frag float64
		var pc int64
		if err := rows.Scan(&dbn, &sn, &tn, &in, &frag, &pc); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"database_name": dbn, "schema_name": sn, "table_name": tn, "index_name": in, "avg_fragmentation_pct": frag, "page_count": pc,
		})
	}
	return results, nil
}

func (c *SqlServerRepository) CollectSQLServerTableStructureMetrics(instanceName, databaseName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := fmt.Sprintf(`
		/* SQL_OPTIMA */ 		
		USE [%s];
		SELECT /* SQL_OPTIMA */   
			DB_NAME() AS database_name,
			SCHEMA_NAME(t.schema_id) AS schema_name,
			t.name AS table_name,
			CAST(MAX(CASE WHEN i.index_id = 1 THEN 1 ELSE 0 END) AS BIT) AS has_clustered_index,
			CAST(MAX(CASE WHEN i.is_primary_key = 1 THEN 1 ELSE 0 END) AS BIT) AS has_primary_key
		FROM sys.tables t WITH (NOLOCK)
		INNER JOIN sys.indexes i WITH (NOLOCK) ON t.object_id = i.object_id
		WHERE t.is_ms_shipped = 0
		GROUP BY t.schema_id, t.name
	`, strings.ReplaceAll(databaseName, "]", "]]"))

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var dbn, sn, tn string
		var hasClustered, hasPK bool
		if err := rows.Scan(&dbn, &sn, &tn, &hasClustered, &hasPK); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"database_name": dbn, "schema_name": sn, "table_name": tn, "has_clustered_index": hasClustered, "has_primary_key": hasPK,
		})
	}
	return results, nil
}
