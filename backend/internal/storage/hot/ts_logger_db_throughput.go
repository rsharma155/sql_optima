// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Database throughput metrics logger for transaction rates.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (tl *TimescaleLogger) LogDatabaseThroughput(ctx context.Context, serverID uuid.UUID, dbStats []DatabaseThroughputRow) error {
	if len(dbStats) == 0 {
		return nil
	}

	batch := &pgx.Batch{}

	for _, r := range dbStats {
		batch.Queue(`
			INSERT INTO sqlserver_database_throughput (
				capture_timestamp, server_id, database_name,
				user_seeks, user_scans, user_lookups, user_writes,
				total_reads, total_writes, tps, batch_requests_per_sec,
				reads, writes, bytes_read, bytes_written,
				read_latency_ms, write_latency_ms
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
			r.CaptureTimestamp, serverID, r.DatabaseName,
			r.UserSeeks, r.UserScans, r.UserLookups, r.UserWrites,
			r.TotalReads, r.TotalWrites, r.TPS, r.BatchRequestsPerSec,
			r.Reads, r.Writes, r.BytesRead, r.BytesWritten,
			r.ReadLatencyMs, r.WriteLatencyMs,
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(dbStats); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("DB throughput batch insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogDatabaseThroughputFromMap(ctx context.Context, serverID uuid.UUID, dbStats []map[string]interface{}) error {
	if len(dbStats) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	now := time.Now().UTC()

	for _, r := range dbStats {
		batch.Queue(`
			INSERT INTO sqlserver_database_throughput (
				capture_timestamp, server_id, database_name,
				user_seeks, user_scans, user_lookups, user_writes,
				total_reads, total_writes, tps, batch_requests_per_sec,
				reads, writes, bytes_read, bytes_written,
				read_latency_ms, write_latency_ms
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
			now, serverID,
			getStr(r, "database_name"),
			getInt64FromMap(r, "user_seeks"),
			getInt64FromMap(r, "user_scans"),
			getInt64FromMap(r, "user_lookups"),
			getInt64FromMap(r, "user_writes"),
			getInt64FromMap(r, "total_reads"),
			getInt64FromMap(r, "total_writes"),
			getFloat64(r, "tps"),
			getFloat64(r, "batch_requests_per_sec"),
			getInt64FromMap(r, "reads"),
			getInt64FromMap(r, "writes"),
			getInt64FromMap(r, "bytes_read"),
			getInt64FromMap(r, "bytes_written"),
			getInt64FromMap(r, "read_latency_ms"),
			getInt64FromMap(r, "write_latency_ms"),
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(dbStats); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("DB throughput batch insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetDatabaseThroughputSummary(ctx context.Context, serverID uuid.UUID, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		WITH raw_stats AS (
			SELECT 
				database_name,
				capture_timestamp,
				tps AS cumulative_tps,
				batch_requests_per_sec AS cumulative_batch,
				total_reads,
				total_writes,
				LAG(tps) OVER (PARTITION BY database_name ORDER BY capture_timestamp) as prev_tps,
				LAG(total_reads) OVER (PARTITION BY database_name ORDER BY capture_timestamp) as prev_reads,
				LAG(total_writes) OVER (PARTITION BY database_name ORDER BY capture_timestamp) as prev_writes,
				EXTRACT(EPOCH FROM (capture_timestamp - LAG(capture_timestamp) OVER (PARTITION BY database_name ORDER BY capture_timestamp))) as seconds_diff
			FROM sqlserver_database_throughput
			WHERE server_id = $1
			  AND capture_timestamp >= NOW() - INTERVAL '24 hours'
		),
		deltas AS (
			SELECT 
				database_name,
				capture_timestamp,
				CASE WHEN seconds_diff > 0 AND cumulative_tps >= prev_tps THEN (cumulative_tps - prev_tps) / seconds_diff ELSE 0 END as tps_rate,
				CASE WHEN seconds_diff > 0 AND total_reads >= prev_reads THEN (total_reads - prev_reads) / seconds_diff ELSE 0 END as reads_rate,
				CASE WHEN seconds_diff > 0 AND total_writes >= prev_writes THEN (total_writes - prev_writes) / seconds_diff ELSE 0 END as writes_rate,
				cumulative_batch as batch_rate
			FROM raw_stats
			WHERE prev_tps IS NOT NULL
		)
		SELECT 
			database_name,
			AVG(tps_rate) AS avg_tps,
			AVG(batch_rate) AS avg_batch_requests,
			SUM(reads_rate) AS total_reads_rate,
			SUM(writes_rate) AS total_writes_rate,
			MAX(tps_rate) AS max_tps,
			COUNT(*) AS sample_count
		FROM deltas
		WHERE capture_timestamp >= NOW() - INTERVAL '1 hour'
		GROUP BY database_name
		ORDER BY AVG(tps_rate) DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var dbName string
		var avgTps, avgBatch, totalReadsRate, totalWritesRate, maxTps float64
		var sampleCount int

		if err := rows.Scan(&dbName, &avgTps, &avgBatch, &totalReadsRate, &totalWritesRate, &maxTps, &sampleCount); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"database_name":      dbName,
			"avg_tps":            avgTps,
			"tps":                avgTps,
			"avg_batch_requests": avgBatch,
			"total_reads":        int64(totalReadsRate),
			"total_writes":       int64(totalWritesRate),
			"max_tps":            maxTps,
			"sample_count":       sampleCount,
		})
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetDatabaseThroughputTimeRange(ctx context.Context, serverID uuid.UUID, start, end time.Time) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			capture_timestamp,
			database_name,
			tps,
			batch_requests_per_sec,
			total_reads,
			total_writes
		FROM sqlserver_database_throughput
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
	`

	rows, err := tl.pool.Query(ctx, query, serverID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var dbName string
		var tps, batchRequests float64
		var totalReads, totalWrites int64

		if err := rows.Scan(&ts, &dbName, &tps, &batchRequests, &totalReads, &totalWrites); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"timestamp":              ts,
			"database_name":          dbName,
			"tps":                    tps,
			"batch_requests_per_sec": batchRequests,
			"total_reads":            totalReads,
			"total_writes":           totalWrites,
		})
	}
	return results, rows.Err()
}

// GetBatchRequestsTrend returns a 1-minute bucketed time series of batch requests/sec or a specific range.
func (tl *TimescaleLogger) GetBatchRequestsTrend(ctx context.Context, serverID uuid.UUID, minutes int, from, to string) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var start, end time.Time
	var err error
	useRange := false
	if from != "" && to != "" {
		start, end, err = parseTimeRangeRFC3339(from, to)
		if err == nil {
			useRange = true
		}
	}

	var q string
	var rows pgx.Rows
	if useRange {
		q = `
			SELECT time_bucket('1 minute', capture_timestamp) AS bucket,
			       AVG(batch_requests_per_sec) AS batch_requests_per_sec
			FROM sqlserver_risk_health
			WHERE server_id = $1
			  AND capture_timestamp >= $2 AND capture_timestamp <= $3
			GROUP BY bucket
			ORDER BY bucket ASC
		`
		rows, err = tl.pool.Query(ctx, q, serverID, start, end)
	} else {
		if minutes <= 0 {
			minutes = 60
		}
		q = `
			SELECT time_bucket('1 minute', capture_timestamp) AS bucket,
			       AVG(batch_requests_per_sec) AS batch_requests_per_sec
			FROM sqlserver_risk_health
			WHERE server_id = $1
			  AND capture_timestamp >= NOW() - ($2::int * INTERVAL '1 minute')
			GROUP BY bucket
			ORDER BY bucket ASC
		`
		rows, err = tl.pool.Query(ctx, q, serverID, minutes)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, minutes)
	for rows.Next() {
		var ts time.Time
		var v float64
		if err := rows.Scan(&ts, &v); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"timestamp":              ts,
			"batch_requests_per_sec": v,
		})
	}
	return out, rows.Err()
}
