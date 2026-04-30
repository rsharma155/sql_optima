// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository for interacting with pg_stat_monitor and associated state tables.
//
// Metadata:
//   Type: Repository
//   Package: postgres
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type PgStatMonitorRow struct {
	BucketStartTime time.Time
	BucketEndTime   time.Time
	Dbid            uint32
	Userid          uint32
	Queryid         int64
	Query           string
	ApplicationName string
	ClientIP        string
	Calls           int64
	TotalExecTime   float64
	MeanExecTime    float64
	MinExecTime     float64
	MaxExecTime     float64
	StdDevExecTime  float64
	Rows            int64
	SharedBlksHit   int64
	SharedBlksRead  int64
	TempBlksWritten int64
	WalBytes        float64
}

type PgStatMonitorAgg struct {
	BucketStartTime time.Time
	Calls           int64
	TotalExecMs     float64
	WalBytes        float64
	BlocksRead      int64
	BlocksHit       int64
}

type PgStatMonitorRepository struct {
	// Any dependencies like a shared DB pool can go here if needed,
	// but usually we pass the connection from the collector.
}

func NewPgStatMonitorRepository() *PgStatMonitorRepository {
	return &PgStatMonitorRepository{}
}

func (r *PgStatMonitorRepository) GetLastCompletedBucket(ctx context.Context, db *sql.DB) (int64, error) {
	var bucket int64
	// We take max(bucket)-1 because the current max bucket is usually still being populated.
	query := "SELECT COALESCE(max(bucket)-1, 0) AS bucket FROM pg_stat_monitor"
	err := db.QueryRowContext(ctx, query).Scan(&bucket)
	return bucket, err
}

func (r *PgStatMonitorRepository) GetLastCollectedBucket(ctx context.Context, tsDB *sql.DB, instanceID string) (int64, error) {
	var bucket int64
	query := "SELECT last_bucket_collected FROM pg_collector_bucket_state WHERE instance_id = $1"
	err := tsDB.QueryRowContext(ctx, query, instanceID).Scan(&bucket)
	if err == sql.ErrNoRows {
		return -1, nil // Never collected
	}
	return bucket, err
}

func (r *PgStatMonitorRepository) UpdateLastCollectedBucket(ctx context.Context, tsDB *sql.DB, instanceID string, bucket int64) error {
	query := `
		INSERT INTO pg_collector_bucket_state (instance_id, last_bucket_collected, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (instance_id) DO UPDATE 
		SET last_bucket_collected = EXCLUDED.last_bucket_collected,
		    updated_at = now();
	`
	_, err := tsDB.ExecContext(ctx, query, instanceID, bucket)
	return err
}

func (r *PgStatMonitorRepository) UpdateInstanceMetadata(ctx context.Context, tsDB *sql.DB, instanceID string, source string) error {
	query := `
		INSERT INTO pg_instance (instance_id, query_stats_source, last_detected_at)
		VALUES ($1, $2, now())
		ON CONFLICT (instance_id) DO UPDATE 
		SET query_stats_source = EXCLUDED.query_stats_source,
		    last_detected_at = now();
	`
	_, err := tsDB.ExecContext(ctx, query, instanceID, source)
	return err
}

func (r *PgStatMonitorRepository) FetchBucketMetrics(ctx context.Context, db *sql.DB, bucket int64) ([]PgStatMonitorRow, error) {
	query := `
	/* SQL_OPTIMA */
		SELECT
			bucket_start_time,
			bucket_start_time AS bucket_end_time, -- pg_stat_monitor 1.x doesn't have bucket_end_time
			dbid,
			userid,
			queryid,
			query,
			COALESCE(application_name, ''),
			COALESCE(client_ip::text, ''),
			calls,
			total_exec_time,
			mean_exec_time,
			min_exec_time,
			max_exec_time,
			stddev_exec_time,
			rows,
			shared_blks_hit,
			shared_blks_read,
			temp_blks_written,
			wal_bytes
		FROM pg_stat_monitor
		WHERE bucket = $1
	`
	rows, err := db.QueryContext(ctx, query, bucket)
	if err != nil {
		// Fallback for pg_stat_monitor 2.0+ where columns were renamed
		if strings.Contains(err.Error(), "column \"bucket_start_time\" does not exist") {
			query2 := `
			/* SQL_OPTIMA */
				SELECT
					bucket_start,
					bucket_end,
					dbid,
					userid,
					queryid,
					query,
					COALESCE(application_name, ''),
					COALESCE(client_ip::text, ''),
					calls,
					total_exec_time,
					mean_exec_time,
					min_exec_time,
					max_exec_time,
					stddev_exec_time,
					rows,
					shared_blks_hit,
					shared_blks_read,
					temp_blks_written,
					wal_bytes
				FROM pg_stat_monitor
				WHERE bucket = $1
			`
			rows, err = db.QueryContext(ctx, query2, bucket)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	defer rows.Close()

	var results []PgStatMonitorRow
	for rows.Next() {
		var res PgStatMonitorRow
		err := rows.Scan(
			&res.BucketStartTime,
			&res.BucketEndTime,
			&res.Dbid,
			&res.Userid,
			&res.Queryid,
			&res.Query,
			&res.ApplicationName,
			&res.ClientIP,
			&res.Calls,
			&res.TotalExecTime,
			&res.MeanExecTime,
			&res.MinExecTime,
			&res.MaxExecTime,
			&res.StdDevExecTime,
			&res.Rows,
			&res.SharedBlksHit,
			&res.SharedBlksRead,
			&res.TempBlksWritten,
			&res.WalBytes,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, rows.Err()
}

func (r *PgStatMonitorRepository) FetchAggregatedMetrics(ctx context.Context, db *sql.DB, bucket int64) (*PgStatMonitorAgg, error) {
	query := `
	/* SQL_OPTIMA */
		SELECT
			bucket_start_time,
			SUM(calls) AS calls,
			SUM(total_exec_time) AS total_exec_ms,
			SUM(wal_bytes) AS wal_bytes,
			SUM(shared_blks_read) AS blocks_read,
			SUM(shared_blks_hit) AS blocks_hit
		FROM pg_stat_monitor
		WHERE bucket = $1
		GROUP BY bucket_start_time
	`
	var agg PgStatMonitorAgg
	err := db.QueryRowContext(ctx, query, bucket).Scan(
		&agg.BucketStartTime,
		&agg.Calls,
		&agg.TotalExecMs,
		&agg.WalBytes,
		&agg.BlocksRead,
		&agg.BlocksHit,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		// Fallback for pg_stat_monitor 2.0+
		if strings.Contains(err.Error(), "column \"bucket_start_time\" does not exist") {
			query2 := `
			/* SQL_OPTIMA */
				SELECT
					bucket_start,
					SUM(calls) AS calls,
					SUM(total_exec_time) AS total_exec_ms,
					SUM(wal_bytes) AS wal_bytes,
					SUM(shared_blks_read) AS blocks_read,
					SUM(shared_blks_hit) AS blocks_hit
				FROM pg_stat_monitor
				WHERE bucket = $1
				GROUP BY bucket_start
			`
			err = db.QueryRowContext(ctx, query2, bucket).Scan(
				&agg.BucketStartTime,
				&agg.Calls,
				&agg.TotalExecMs,
				&agg.WalBytes,
				&agg.BlocksRead,
				&agg.BlocksHit,
			)
			if err != nil {
				if err == sql.ErrNoRows {
					return nil, nil
				}
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return &agg, nil
}

func (r *PgStatMonitorRepository) LogBucketMetrics(ctx context.Context, tsDB *sql.DB, instanceID string, rows []PgStatMonitorRow) error {
	if len(rows) == 0 {
		return nil
	}

	// Using a transaction for bulk insert
	tx, err := tsDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO pg_query_bucket_metrics (
			bucket_start, bucket_end, instance_id, dbid, userid, queryid, query,
			application_name, client_ip, calls, total_exec_time, mean_exec_time,
			min_exec_time, max_exec_time, stddev_exec_time, rows,
			shared_blks_hit, shared_blks_read, temp_blks_written, wal_bytes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, row := range rows {
		_, err := stmt.ExecContext(ctx,
			row.BucketStartTime, row.BucketEndTime, instanceID, row.Dbid, row.Userid, row.Queryid, row.Query,
			row.ApplicationName, row.ClientIP, row.Calls, row.TotalExecTime, row.MeanExecTime,
			row.MinExecTime, row.MaxExecTime, row.StdDevExecTime, row.Rows,
			row.SharedBlksHit, row.SharedBlksRead, row.TempBlksWritten, row.WalBytes,
		)
		if err != nil {
			return fmt.Errorf("failed to insert bucket metric: %w", err)
		}
	}

	return tx.Commit()
}
