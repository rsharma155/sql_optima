// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server Query Store statistics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"
)

type QueryStoreStats struct {
	DatabaseName      string
	QueryHash         string
	QueryText         string
	PlanID            int64
	IntervalID        int64
	Executions        int64
	AvgDurationMs     float64
	AvgCpuMs          float64
	AvgLogicalReads   float64
	TotalCpuMs        float64
	LastExecutionTime time.Time
}

func (c *SqlServerRepository) FetchQueryStoreStats(instanceName string) ([]QueryStoreStats, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	dbNames, err := c.listUserDatabaseNamesForQueryStore(db)
	if err != nil || len(dbNames) == 0 {
		log.Printf("[SQLSERVER] FetchQueryStoreStats: no user DBs with Query Store or list error for %s: %v — trying current database only", instanceName, err)
		return c.fetchQueryStoreStatsSingleDB(db, "")
	}

	var merged []QueryStoreStats
	for _, dbn := range dbNames {
		qb := sqlServerQuoteBracket(dbn)
		query := queryStoreStatsSelectSQL(qb)
		rows, err := db.Query(query)
		if err != nil {
			log.Printf("[SQLSERVER] FetchQueryStoreStats query in %s: %v", dbn, err)
			continue
		}
		for rows.Next() {
			var qs QueryStoreStats
			qs.DatabaseName = dbn
			var queryText sql.NullString
			var qh int64
			if err := rows.Scan(&qh, &queryText, &qs.PlanID, &qs.IntervalID, &qs.Executions,
				&qs.AvgDurationMs, &qs.AvgCpuMs, &qs.AvgLogicalReads, &qs.TotalCpuMs, &qs.LastExecutionTime); err != nil {
				log.Printf("[SQLSERVER] FetchQueryStoreStats Scan Error: %v", err)
				continue
			}
			qs.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
			if queryText.Valid {
				qs.QueryText = queryText.String
			} else {
				qs.QueryText = "N/A"
			}
			merged = append(merged, qs)
		}
		rows.Close()
	}

	if len(merged) == 0 {
		return c.fetchQueryStoreStatsSingleDB(db, "")
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].TotalCpuMs > merged[j].TotalCpuMs
	})
	if len(merged) > 200 {
		merged = merged[:200]
	}
	return merged, nil
}

func queryStoreStatsSelectSQL(dbPrefix string) string {
	return fmt.Sprintf(`
		/* SQL_OPTIMA */ 
		SELECT   TOP 100
			CAST(q.query_hash AS BIGINT) AS query_hash,
			qt.query_sql_text AS query_text,
			p.plan_id,
			rs.runtime_stats_interval_id,
			ISNULL(rs.count_executions, 0) AS executions,
			ISNULL(rs.avg_duration, 0) / 1000.0 AS avg_duration_ms,
			ISNULL(rs.avg_cpu_time, 0) / 1000.0 AS avg_cpu_ms,
			ISNULL(rs.avg_logical_io_reads, 0) AS avg_logical_reads,
			(ISNULL(rs.avg_cpu_time, 0) * ISNULL(rs.count_executions, 1)) / 1000.0 AS total_cpu_ms,
			rs.last_execution_time
		FROM %s.sys.query_store_query q
		INNER JOIN %s.sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
		INNER JOIN %s.sys.query_store_plan p ON q.query_id = p.query_id
		INNER JOIN %s.sys.query_store_runtime_stats rs ON p.plan_id = rs.plan_id
		WHERE q.is_internal_query = 0
		  AND ISNULL(rs.count_executions, 0) > 0
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%%sql_optima%%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%%sys.dm_%%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%%sys.query_store%%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%%sp_server_diagnostics%%'
		  AND rs.last_execution_time > DATEADD(minute, -60, GETUTCDATE())
		  AND (
			  rs.avg_cpu_time > 1000 
			  OR rs.avg_logical_io_reads > 50 
			  OR rs.count_executions > 5
		  )
		  AND q.query_id NOT IN (
			  SELECT query_id 
			  FROM %s.sys.query_store_query 
			  WHERE last_bind_duration = 0
		  )
		ORDER BY (ISNULL(rs.avg_cpu_time, 0) * ISNULL(rs.count_executions, 0)) DESC
	`, dbPrefix, dbPrefix, dbPrefix, dbPrefix, dbPrefix)
}

func (c *SqlServerRepository) listUserDatabaseNamesForQueryStore(db *sql.DB) ([]string, error) {
	q := `/* SQL_OPTIMA */  
		SELECT  d.name
		FROM sys.databases d WITH (NOLOCK)
		WHERE d.database_id > 4
		  AND d.state = 0
		  AND d.is_query_store_on = 1
		ORDER BY d.name`
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
		names = append(names, n)
	}
	return names, rows.Err()
}

func (c *SqlServerRepository) fetchQueryStoreStatsSingleDB(db *sql.DB, labelDB string) ([]QueryStoreStats, error) {
	query := `
		/* SQL_OPTIMA */ SELECT   TOP 100
			CAST(q.query_hash AS BIGINT) AS query_hash,
			qt.query_sql_text AS query_text,
			p.plan_id,
			rs.runtime_stats_interval_id,
			ISNULL(rs.count_executions, 0) AS executions,
			ISNULL(rs.avg_duration, 0) / 1000.0 AS avg_duration_ms,
			ISNULL(rs.avg_cpu_time, 0) / 1000.0 AS avg_cpu_ms,
			ISNULL(rs.avg_logical_io_reads, 0) AS avg_logical_reads,
			(ISNULL(rs.avg_cpu_time, 0) * ISNULL(rs.count_executions, 1)) / 1000.0 AS total_cpu_ms,
			rs.last_execution_time
		FROM sys.query_store_query q
		INNER JOIN sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
		INNER JOIN sys.query_store_plan p ON q.query_id = p.query_id
		INNER JOIN sys.query_store_runtime_stats rs ON p.plan_id = rs.plan_id
		WHERE q.is_internal_query = 0
		  AND ISNULL(rs.count_executions, 0) > 0
		  AND qt.query_sql_text NOT LIKE '%/* SQL_OPTIMA */%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%sql_optima%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%sqloptima%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%sys.dm_%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%sys.query_store%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%sp_server_diagnostics%'
		  AND rs.last_execution_time > DATEADD(minute, -60, GETUTCDATE())
		  AND (
			  rs.avg_cpu_time > 1000 
			  OR rs.avg_logical_io_reads > 50 
			  OR rs.count_executions > 5
		  )
		  AND q.query_id NOT IN (
			  SELECT  query_id 
			  FROM sys.query_store_query 
			  WHERE last_bind_duration = 0
		  )
		ORDER BY (ISNULL(rs.avg_cpu_time, 0) * ISNULL(rs.count_executions, 0)) DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []QueryStoreStats
	for rows.Next() {
		var qs QueryStoreStats
		qs.DatabaseName = labelDB
		if labelDB == "" {
			qs.DatabaseName = "default"
		}
		var queryText sql.NullString
		var qh int64
		if err := rows.Scan(&qh, &queryText, &qs.PlanID, &qs.IntervalID, &qs.Executions,
			&qs.AvgDurationMs, &qs.AvgCpuMs, &qs.AvgLogicalReads, &qs.TotalCpuMs, &qs.LastExecutionTime); err != nil {
			continue
		}
		qs.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		if queryText.Valid {
			qs.QueryText = queryText.String
		} else {
			qs.QueryText = "N/A"
		}
		results = append(results, qs)
	}
	return results, rows.Err()
}

func (c *SqlServerRepository) FetchQueryStoreSQLText(instanceName, databaseName, queryHash string) (string, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return "", fmt.Errorf("no connection for instance: %s", instanceName)
	}
	databaseName = strings.TrimSpace(databaseName)
	if databaseName == "" {
		return "", fmt.Errorf("database is required")
	}
	queryHash = strings.TrimSpace(queryHash)
	if queryHash == "" {
		return "", fmt.Errorf("query_hash is required")
	}

	var qhBytes []byte
	// Strip any UI suffixes like :1
	hashPart := strings.Split(queryHash, ":")[0]
	hashPart = strings.TrimPrefix(hashPart, "0x")
	
	if b, err := hex.DecodeString(hashPart); err == nil {
		qhBytes = b
	} else {
		// Fallback to uint64 if hex decode fails (though binary(8) preferred)
		if h, err := strconv.ParseUint(hashPart, 16, 64); err == nil {
			var buf [8]byte
			binary.BigEndian.PutUint64(buf[:], h)
			qhBytes = buf[:]
		} else {
			return "", fmt.Errorf("invalid query_hash format: %s", queryHash)
		}
	}

	dbPrefix := sqlServerQuoteBracket(databaseName)
	q := fmt.Sprintf(`
		/* SQL_OPTIMA */ SELECT   TOP 1
			qt.query_sql_text
		FROM %s.sys.query_store_query q
		INNER JOIN %s.sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
		INNER JOIN %s.sys.query_store_plan p ON q.query_id = p.query_id
		INNER JOIN %s.sys.query_store_runtime_stats rs ON p.plan_id = rs.plan_id
		WHERE q.is_internal_query = 0
		  AND q.query_hash = @p1
		ORDER BY rs.last_execution_time DESC;
	`, dbPrefix, dbPrefix, dbPrefix, dbPrefix)

	var sqlText sql.NullString
	if err := db.QueryRow(q, qhBytes).Scan(&sqlText); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	if !sqlText.Valid {
		return "", nil
	}
	return sqlText.String, nil
}
