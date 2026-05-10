/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Health Dashboard v2 TimescaleDB storage fetchers for trends.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */
package hot

import (
	"context"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

func (tl *TimescaleLogger) GetWaitTrendV2(ctx context.Context, instanceName string, from, to time.Time) ([]models.WaitTrendPoint, error) {
	// Grouped wait stats query
	q := `
		SELECT 
			time_bucket('1 minute', capture_timestamp) AS bucket,
			SUM(CASE WHEN wait_type = 'SOS_SCHEDULER_YIELD' THEN wait_time_ms_total ELSE 0 END) as cpu,
			SUM(CASE WHEN wait_type LIKE 'PAGEIOLATCH_%' OR wait_type = 'WRITELOG' THEN wait_time_ms_total ELSE 0 END) as io,
			SUM(CASE WHEN wait_type = 'RESOURCE_SEMAPHORE' THEN wait_time_ms_total ELSE 0 END) as memory,
			SUM(CASE WHEN wait_type LIKE 'LCK_%' THEN wait_time_ms_total ELSE 0 END) as locking,
			SUM(CASE WHEN wait_type IN ('CXPACKET', 'CXCONSUMER') THEN wait_time_ms_total ELSE 0 END) as parallel,
			SUM(CASE WHEN wait_type NOT LIKE 'LCK_%' AND wait_type NOT LIKE 'PAGEIOLATCH_%' AND wait_type NOT IN ('SOS_SCHEDULER_YIELD', 'WRITELOG', 'RESOURCE_SEMAPHORE', 'CXPACKET', 'CXCONSUMER') THEN wait_time_ms_total ELSE 0 END) as other
		FROM sqlserver_wait_history
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND capture_timestamp BETWEEN $2 AND $3
		GROUP BY bucket
		ORDER BY bucket ASC
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, from, to)
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

func (tl *TimescaleLogger) GetIOLatencyTrendV2(ctx context.Context, instanceName string, from, to time.Time) ([]models.IOLatencyPoint, error) {
	q := `
		SELECT 
			time_bucket('1 minute', capture_timestamp) AS bucket,
			AVG(CASE WHEN file_type = 'ROWS' THEN read_latency_ms ELSE NULL END) as read_lat,
			AVG(CASE WHEN file_type = 'ROWS' THEN write_latency_ms ELSE NULL END) as write_lat,
			AVG(CASE WHEN file_type = 'LOG' THEN write_latency_ms ELSE NULL END) as log_lat,
			SUM(num_of_reads) as read_iops,
			SUM(num_of_writes) as write_iops
		FROM sqlserver_disk_history
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND capture_timestamp BETWEEN $2 AND $3
		GROUP BY bucket
		ORDER BY bucket ASC
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, from, to)
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
		if rLat != nil { p.DataReadMs = *rLat }
		if wLat != nil { p.DataWriteMs = *wLat }
		if lLat != nil { p.LogWriteMs = *lLat }
		results = append(results, p)
	}
	return results, nil
}

func (tl *TimescaleLogger) GetThroughputTrendV2(ctx context.Context, instanceName string, from, to time.Time) ([]models.ThroughputPoint, error) {
	q := `
		SELECT 
			time_bucket('1 minute', capture_timestamp) AS bucket,
			AVG(batch_requests) as batch_requests,
			MAX(active_users) as connections,
			0.0 as logins_per_sec
		FROM sqlserver_metrics
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND capture_timestamp BETWEEN $2 AND $3
		GROUP BY bucket
		ORDER BY bucket ASC
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, from, to)
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
		if br != nil { p.BatchRequests = *br }
		results = append(results, p)
	}
	return results, nil
}
