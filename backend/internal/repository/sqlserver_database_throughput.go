// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server database throughput statistics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
	"log"
)

type DatabaseThroughputStats struct {
	DatabaseName        string
	UserSeeks           int64
	UserScans           int64
	UserLookups         int64
	UserWrites          int64
	TotalReads          int64
	TotalWrites         int64
	TPS                 float64
	BatchRequestsPerSec float64
	// New I/O throughput metrics
	Reads          int64
	Writes         int64
	BytesRead      int64
	BytesWritten   int64
	ReadLatencyMs  int64
	WriteLatencyMs int64
}

func (c *SqlServerRepository) FetchDatabaseThroughput(instanceName string) ([]DatabaseThroughputStats, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	// Primary Throughput Query using sys.dm_io_virtual_file_stats
	query := `
		/* SQL_OPTIMA */
		SELECT
			DB_NAME(vfs.database_id) AS database_name,
			SUM(vfs.num_of_reads) AS reads,
			SUM(vfs.num_of_writes) AS writes,
			SUM(vfs.num_of_bytes_read) AS bytes_read,
			SUM(vfs.num_of_bytes_written) AS bytes_written,
			SUM(vfs.io_stall_read_ms) AS read_latency_ms,
			SUM(vfs.io_stall_write_ms) AS write_latency_ms
		FROM sys.dm_io_virtual_file_stats(NULL, NULL) AS vfs
		INNER JOIN sys.databases d ON vfs.database_id = d.database_id
		WHERE d.database_id > 4
		  AND d.state_desc = 'ONLINE'
		  AND d.source_database_id IS NULL
		GROUP BY vfs.database_id
		ORDER BY (SUM(vfs.num_of_bytes_read) + SUM(vfs.num_of_bytes_written)) DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[SQLSERVER] FetchDatabaseThroughput Error for %s: %v", instanceName, err)
		return nil, err
	}
	defer rows.Close()

	var results []DatabaseThroughputStats
	for rows.Next() {
		var s DatabaseThroughputStats
		if err := rows.Scan(
			&s.DatabaseName,
			&s.Reads,
			&s.Writes,
			&s.BytesRead,
			&s.BytesWritten,
			&s.ReadLatencyMs,
			&s.WriteLatencyMs,
		); err != nil {
			log.Printf("[SQLSERVER] FetchDatabaseThroughput Scan Error: %v", err)
			continue
		}
		// Map new metrics to total fields for compatibility
		s.TotalReads = s.Reads
		s.TotalWrites = s.Writes
		results = append(results, s)
	}

	// Secondary Query: Transaction Throughput (TPS)
	tpsQuery := `
		/* SQL_OPTIMA */
		SELECT
			DB_NAME(database_id) AS database_name,
			SUM(database_transaction_begin_count) AS tps_count
		FROM sys.dm_tran_database_transactions
		WHERE database_id > 4
		GROUP BY database_id
	`
	tpsRows, tpsErr := db.Query(tpsQuery)
	if tpsErr == nil {
		defer tpsRows.Close()
		tpsMap := make(map[string]float64)
		for tpsRows.Next() {
			var dbName string
			var tps float64
			if err := tpsRows.Scan(&dbName, &tps); err == nil {
				tpsMap[dbName] = tps
			}
		}
		for i := range results {
			if tps, ok := tpsMap[results[i].DatabaseName]; ok {
				results[i].TPS = tps
			}
		}
	}

	// Tertiary Query: Batch Requests (Server Level but matched to DB for compatibility)
	batchQuery := `
		SELECT /* SQL_OPTIMA */   
			DB_NAME(r.database_id) AS database_name,
			COUNT(*) AS batch_count
		FROM sys.dm_exec_requests r
		WHERE r.database_id > 4
		GROUP BY r.database_id
	`

	batchRows, batchErr := db.Query(batchQuery)
	if batchErr == nil {
		defer batchRows.Close()
		batchMap := make(map[string]float64)
		for batchRows.Next() {
			var dbName string
			var bps float64
			if err := batchRows.Scan(&dbName, &bps); err == nil {
				batchMap[dbName] = bps
			}
		}
		for i := range results {
			if bps, ok := batchMap[results[i].DatabaseName]; ok {
				results[i].BatchRequestsPerSec = bps
			}
		}
	}

	return results, rows.Err()
}

/*
If someone says “database throughput,” they usually mean one of these:

Transaction throughput → transactions/sec
I/O throughput → MB/sec read/write
Logical workload throughput → reads/writes per sec
Batch throughput → requests/sec
Per-database activity rate → how busy each DB is

Your earlier query was really measuring cumulative index activity, which is useful, but not ideal for throughput.

If your dashboard is for DBA monitoring, the best practical database throughput metric is:

Per-database I/O throughput + transaction rate

Why?
Because it reflects real workload pressure and scales well across OLTP / reporting / ETL systems.

Option 1 (Recommended): Per-database I/O throughput

This is usually the most meaningful “database throughput” metric.

Query:

SELECT
    DB_NAME(vfs.database_id) AS database_name,
    SUM(vfs.num_of_reads) AS total_reads,
    SUM(vfs.num_of_writes) AS total_writes,
    SUM(vfs.num_of_bytes_read) / 1048576.0 AS mb_read,
    SUM(vfs.num_of_bytes_written) / 1048576.0 AS mb_written,
    SUM(vfs.io_stall_read_ms) AS read_stall_ms,
    SUM(vfs.io_stall_write_ms) AS write_stall_ms
FROM sys.dm_io_virtual_file_stats(NULL, NULL) AS vfs
WHERE vfs.database_id > 4
GROUP BY vfs.database_id
ORDER BY (SUM(vfs.num_of_bytes_read) + SUM(vfs.num_of_bytes_written)) DESC;
To make it true throughput:

Sample every minute and calculate delta.

Example:

mb_read_per_sec =
(current_mb_read - previous_mb_read) / elapsed_seconds

This gives:

MB/sec read
MB/sec write
real storage pressure

This is what many enterprise monitoring tools effectively track.

Option 2: Transaction throughput (TPS)

If you want “business workload throughput.”

SELECT
    DB_NAME(database_id) AS database_name,
    database_transaction_begin_count,
    database_transaction_commit_count,
    database_transaction_rollback_count
FROM sys.dm_tran_database_transactions;

Store snapshots and compute delta:

TPS =
(commit_delta + rollback_delta) / seconds

Good for:

OLTP systems
API/database workload measurement

Less useful for analytics workloads.

Option 3: Requests/sec (server workload)

Good overall throughput indicator.

SELECT cntr_value
FROM sys.dm_os_performance_counters
WHERE counter_name = 'Batch Requests/sec';

This is server-wide, not per DB.

Useful as a top-level dashboard metric.

Option 4: Best per-database practical throughput query

If I were building your TimescaleDB monitoring platform, I’d use this:

SELECT
    GETUTCDATE() AS sample_time,
    DB_NAME(vfs.database_id) AS database_name,
    SUM(vfs.num_of_reads) AS reads,
    SUM(vfs.num_of_writes) AS writes,
    SUM(vfs.num_of_bytes_read) AS bytes_read,
    SUM(vfs.num_of_bytes_written) AS bytes_written,
    SUM(vfs.io_stall_read_ms) AS read_latency_ms,
    SUM(vfs.io_stall_write_ms) AS write_latency_ms
FROM sys.dm_io_virtual_file_stats(NULL, NULL) AS vfs
INNER JOIN sys.databases d
    ON vfs.database_id = d.database_id
WHERE d.database_id > 4
  AND d.state_desc = 'ONLINE'
  AND d.source_database_id IS NULL
GROUP BY vfs.database_id;

Then derive:

Read throughput
(bytes_read_delta / 1048576.0) / elapsed_seconds
Write throughput
(bytes_written_delta / 1048576.0) / elapsed_seconds
IOPS
(reads_delta + writes_delta) / elapsed_seconds
Avg latency
latency_delta / io_delta

That gives you a proper throughput dashboard.

My blunt recommendation for your platform

Track these 4:

Database level
MB/sec read
MB/sec write
IOPS
TPS
Server level
Batch Requests/sec
Page Life Expectancy
Wait stats
CPU %

That’s a far better definition of “database throughput” than index usage stats.

So if you're asking for the ideal query for database throughput, use sys.dm_io_virtual_file_stats() sampled over time.
That’s the closest thing to a real, reliable throughput metric in SQL Server.
*/
