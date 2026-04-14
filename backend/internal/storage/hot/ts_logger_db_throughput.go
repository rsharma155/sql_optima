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

	"github.com/jackc/pgx/v5"
)

func (tl *TimescaleLogger) LogDatabaseThroughput(ctx context.Context, instanceName string, dbStats []DatabaseThroughputRow) error {
	if len(dbStats) == 0 {
		return nil
	}

	batch := &pgx.Batch{}

	for _, r := range dbStats {
		batch.Queue(`
			INSERT INTO sqlserver_database_throughput (
				capture_timestamp, server_instance_name, database_name,
				user_seeks, user_scans, user_lookups, user_writes,
				total_reads, total_writes, tps, batch_requests_per_sec
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			r.CaptureTimestamp, r.ServerInstanceName, r.DatabaseName,
			r.UserSeeks, r.UserScans, r.UserLookups, r.UserWrites,
			r.TotalReads, r.TotalWrites, r.TPS, r.BatchRequestsPerSec,
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

func (tl *TimescaleLogger) LogDatabaseThroughputFromMap(ctx context.Context, instanceName string, dbStats []map[string]interface{}) error {
	if len(dbStats) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	now := time.Now().UTC()

	for _, r := range dbStats {
		batch.Queue(`
			INSERT INTO sqlserver_database_throughput (
				capture_timestamp, server_instance_name, database_name,
				user_seeks, user_scans, user_lookups, user_writes,
				total_reads, total_writes, tps, batch_requests_per_sec
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			now, instanceName,
			getStr(r, "database_name"),
			getInt64FromMap(r, "user_seeks"),
			getInt64FromMap(r, "user_scans"),
			getInt64FromMap(r, "user_lookups"),
			getInt64FromMap(r, "user_writes"),
			getInt64FromMap(r, "total_reads"),
			getInt64FromMap(r, "total_writes"),
			getFloat64(r, "tps"),
			getFloat64(r, "batch_requests_per_sec"),
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

func (tl *TimescaleLogger) GetDatabaseThroughputSummary(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT 
			database_name,
			AVG(tps) AS avg_tps,
			AVG(batch_requests_per_sec) AS avg_batch_requests,
			SUM(total_reads) AS total_reads,
			SUM(total_writes) AS total_writes,
			MAX(tps) AS max_tps,
			COUNT(*) AS sample_count
		FROM sqlserver_database_throughput
		WHERE server_instance_name = $1
		  AND capture_timestamp >= NOW() - INTERVAL '1 hour'
		GROUP BY database_name
		ORDER BY AVG(tps) DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var dbName string
		var avgTps, avgBatch, totalReads, totalWrites, maxTps float64
		var sampleCount int

		if err := rows.Scan(&dbName, &avgTps, &avgBatch, &totalReads, &totalWrites, &maxTps, &sampleCount); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"database_name":      dbName,
			"avg_tps":            avgTps,
			"avg_batch_requests": avgBatch,
			"total_reads":        int64(totalReads),
			"total_writes":       int64(totalWrites),
			"max_tps":            maxTps,
			"sample_count":       sampleCount,
		})
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetDatabaseThroughputTimeRange(ctx context.Context, instanceName string, start, end time.Time) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			capture_timestamp,
			database_name,
			tps,
			batch_requests_per_sec,
			total_reads,
			total_writes
		FROM sqlserver_database_throughput
		WHERE server_instance_name = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, start, end)
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

// GetBatchRequestsTrend returns a 1-minute bucketed time series of batch requests/sec
// summed across all databases for the instance.
func (tl *TimescaleLogger) GetBatchRequestsTrend(ctx context.Context, instanceName string, minutes int) ([]map[string]interface{}, error) {
	if minutes <= 0 {
		minutes = 60
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := `
		SELECT time_bucket('1 minute', capture_timestamp) AS bucket,
		       SUM(batch_requests_per_sec) AS batch_requests_per_sec
		FROM sqlserver_database_throughput
		WHERE server_instance_name = $1
		  AND capture_timestamp >= NOW() - ($2::int * INTERVAL '1 minute')
		GROUP BY bucket
		ORDER BY bucket ASC
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, minutes)
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
