/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Health Dashboard v2 TimescaleDB storage fetchers for trends.
 * Metadata:
 *   - Feature: SQL Server Health Dashboard V2
 *   - Layer: Infrastructure (Storage)
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */
package hot

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/models"
)

type SQLServerHealthV2Row struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	BatchRequests    float64   `json:"batch_requests"`
	Connections      int       `json:"connections"`
	LoginsPerSec     float64   `json:"logins_per_sec"`
}

// LogSQLServerWaitStatV2 writes a single categorized delta wait row to sqlserver_wait_stats.
func (tl *TimescaleLogger) LogSQLServerWaitStatV2(ctx context.Context, serverID uuid.UUID, category string, waitMs float64) error {
	q := `
        INSERT INTO sqlserver_wait_stats
            (capture_timestamp, server_id, wait_category, wait_time_ms, signal_wait_time_ms, waiting_tasks)
        VALUES ($1, $2, $3, $4, 0, 0)
        ON CONFLICT DO NOTHING
    `
	_, err := tl.pool.Exec(ctx, q, time.Now().UTC(), serverID, category, waitMs)
	return err
}

// LogSQLServerFileIOV2 writes a single file IO delta row to sqlserver_file_io.
// Skips writes when both latency and throughput are zero (no IO activity) or
// when the values are identical to the previous write for this file.
func (tl *TimescaleLogger) LogSQLServerFileIOV2(
	ctx context.Context, serverID uuid.UUID,
	dbName, fileName, fileType string,
	readLatMs, writeLatMs, readBPS, writeBPS float64,
) error {
	// 1. Skip zero-activity rows — they add no information to trend charts.
	if readLatMs == 0 && writeLatMs == 0 && readBPS == 0 && writeBPS == 0 {
		return nil
	}

	// 2. Hash-based deduplication: DMV averages often stay identical between scrapes.
	// Only write if values changed, to keep TimescaleDB lean.
	key := serverID.String() + ":" + dbName + ":" + fileName
	newHash := fileIOHash(readLatMs, writeLatMs, readBPS, writeBPS)

	tl.mu.RLock()
	prevHash, ok := tl.prevFileIOHashKey[key]
	tl.mu.RUnlock()

	if ok && prevHash == newHash {
		return nil // Identical activity — skip write
	}

	// 3. Update cache
	tl.mu.Lock()
	tl.prevFileIOHashKey[key] = newHash
	tl.mu.Unlock()

	q := `
        INSERT INTO sqlserver_file_io
            (capture_timestamp, server_id, database_name, file_name, file_type,
             read_latency_ms, write_latency_ms, read_bytes_per_sec, write_bytes_per_sec)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        ON CONFLICT (server_id, database_name, file_name, capture_timestamp) DO NOTHING
    `
	_, err := tl.pool.Exec(ctx, q, time.Now().UTC(), serverID, dbName, fileName, fileType,
		readLatMs, writeLatMs, readBPS, writeBPS)
	return err
}

func (tl *TimescaleLogger) GetWaitTrendV2(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]models.WaitTrendPoint, error) {
	// Dynamically adjust bucket size based on range duration
	dur := to.Sub(from)
	bucket := "1 minute"
	if dur > 12*time.Hour {
		bucket = "15 minutes"
	} else if dur > 3*time.Hour {
		bucket = "5 minutes"
	}

	// Grouped wait stats query
	q := fmt.Sprintf(`
		SELECT 
			time_bucket('%s', capture_timestamp) AS bucket,
			SUM(CASE WHEN wait_category = 'CPU' THEN wait_time_ms ELSE 0 END) as cpu,
			SUM(CASE WHEN wait_category IN ('IO_DATA', 'IO_LOG') THEN wait_time_ms ELSE 0 END) as io,
			SUM(CASE WHEN wait_category = 'MEMORY' THEN wait_time_ms ELSE 0 END) as memory,
			SUM(CASE WHEN wait_category = 'LOCKING' THEN wait_time_ms ELSE 0 END) as locking,
			SUM(CASE WHEN wait_category = 'PARALLELISM' THEN wait_time_ms ELSE 0 END) as parallel,
			SUM(CASE WHEN wait_category NOT IN ('CPU', 'IO_DATA', 'IO_LOG', 'MEMORY', 'LOCKING', 'PARALLELISM') THEN wait_time_ms ELSE 0 END) as other
		FROM sqlserver_wait_stats
		WHERE server_id = $1
		  AND capture_timestamp BETWEEN $2 AND $3
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucket)
	rows, err := tl.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []models.WaitTrendPoint{}
	for rows.Next() {
		var p models.WaitTrendPoint
		if err := rows.Scan(&p.Timestamp, &p.CPU, &p.IO, &p.Memory, &p.Locking, &p.Parallel, &p.Other); err != nil {
			continue
		}
		results = append(results, p)
	}
	return results, nil
}

func (tl *TimescaleLogger) GetIOLatencyTrendV2(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]models.IOLatencyPoint, error) {
	dur := to.Sub(from)
	bucket := "1 minute"
	if dur > 12*time.Hour {
		bucket = "15 minutes"
	} else if dur > 3*time.Hour {
		bucket = "5 minutes"
	}

	q := fmt.Sprintf(`
		SELECT 
			time_bucket('%s', capture_timestamp) AS bucket,
			AVG(CASE WHEN file_type = 'ROWS' THEN read_latency_ms ELSE NULL END) as read_lat,
			AVG(CASE WHEN file_type = 'ROWS' THEN write_latency_ms ELSE NULL END) as write_lat,
			AVG(CASE WHEN file_type = 'LOG' THEN write_latency_ms ELSE NULL END) as log_lat,
			SUM(read_bytes_per_sec / 1024 / 1024) as read_iops,
			SUM(write_bytes_per_sec / 1024 / 1024) as write_iops
		FROM sqlserver_file_io
		WHERE server_id = $1
		  AND capture_timestamp BETWEEN $2 AND $3
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucket)
	rows, err := tl.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []models.IOLatencyPoint{}
	for rows.Next() {
		var p models.IOLatencyPoint
		var rLat, wLat, lLat *float64
		if err := rows.Scan(&p.Timestamp, &rLat, &wLat, &lLat, &p.ReadIOPS, &p.WriteIOPS); err != nil {
			continue
		}
		if rLat != nil {
			p.DataReadMs = *rLat
		}
		if wLat != nil {
			p.DataWriteMs = *wLat
		}
		if lLat != nil {
			p.LogWriteMs = *lLat
		}
		results = append(results, p)
	}
	return results, nil
}

func (tl *TimescaleLogger) GetThroughputTrendV2(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]models.ThroughputPoint, error) {
	dur := to.Sub(from)
	bucket := "1 minute"
	if dur > 12*time.Hour {
		bucket = "15 minutes"
	} else if dur > 3*time.Hour {
		bucket = "5 minutes"
	}

	q := fmt.Sprintf(`
		WITH br AS (
			SELECT 
				time_bucket('%s', capture_timestamp) AS bucket,
				AVG(value_per_sec) as batch_requests
			FROM sqlserver_perf_counters
			WHERE server_id = $1
			  AND counter_name = 'Batch Requests/sec'
			  AND capture_timestamp BETWEEN $2 AND $3
			GROUP BY bucket
		),
		conns AS (
			SELECT 
				time_bucket('%s', capture_timestamp) AS bucket,
				MAX(active_users) as connections
			FROM sqlserver_metrics
			WHERE server_id = $1
			  AND capture_timestamp BETWEEN $2 AND $3
			GROUP BY bucket
		),
		logins AS (
			SELECT 
				time_bucket('%s', capture_timestamp) AS bucket,
				AVG(logins_per_sec) as logins_per_sec
			FROM sqlserver_health_kpis_v2
			WHERE server_id = $1
			  AND capture_timestamp BETWEEN $2 AND $3
			GROUP BY bucket
		)
		SELECT 
			COALESCE(br.bucket, conns.bucket, logins.bucket) as bucket,
			COALESCE(br.batch_requests, 0) as batch_requests,
			COALESCE(conns.connections, 0) as connections,
			COALESCE(logins.logins_per_sec, 0) as logins_per_sec
		FROM br
		FULL OUTER JOIN conns ON br.bucket = conns.bucket
		FULL OUTER JOIN logins ON COALESCE(br.bucket, conns.bucket) = logins.bucket
		ORDER BY bucket ASC
	`, bucket, bucket, bucket)
	rows, err := tl.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []models.ThroughputPoint{}
	for rows.Next() {
		var p models.ThroughputPoint
		var br *float64
		if err := rows.Scan(&p.Timestamp, &br, &p.Connections, &p.LoginsPerSec); err != nil {
			continue
		}
		if br != nil {
			p.BatchRequests = *br
		}
		results = append(results, p)
	}
	return results, nil
}
