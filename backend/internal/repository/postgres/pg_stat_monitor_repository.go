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

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/repository"
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
	WalMB           float64
	BlocksRead      int64
	BlocksHit       int64
}

type PgStatMonitorRepository struct {
}

func NewPgStatMonitorRepository() *PgStatMonitorRepository {
	return &PgStatMonitorRepository{}
}

func (r *PgStatMonitorRepository) CheckExtension(ctx context.Context, db *sql.DB) (bool, error) {
	var exists bool
	ctx, cancel := repository.WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_monitor')").Scan(&exists)
	return exists, err
}

func (r *PgStatMonitorRepository) GetLastCompletedBucket(ctx context.Context, db *sql.DB) (int64, error) {
	var bucket int64
	query := "SELECT COALESCE(max(bucket)-1, 0) AS bucket FROM pg_stat_monitor"
	ctx, cancel := repository.WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, query).Scan(&bucket)
	return bucket, err
}

func (r *PgStatMonitorRepository) GetLastCollectedBucket(ctx context.Context, tsDB *sql.DB, instanceName string) (int64, error) {
	var bucket int64
	// In the future, we should probably query by server_id (UUID), but instanceName is still used in some state tables.
	// For now, let's stick to what the schema actually has or what we refactored.
	query := "SELECT last_bucket_collected FROM pg_collector_bucket_state WHERE server_id = $1"
	ctx, cancel := repository.WithQueryTimeout(ctx, 0)
	defer cancel()
	err := tsDB.QueryRowContext(ctx, query, instanceName).Scan(&bucket)
	if err == sql.ErrNoRows {
		return -1, nil
	}
	return bucket, err
}

func (r *PgStatMonitorRepository) UpdateLastCollectedBucket(ctx context.Context, tsDB *sql.DB, instanceName string, bucket int64) error {
	query := `
		INSERT INTO pg_collector_bucket_state (server_id, last_bucket_collected, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (server_id) DO UPDATE 
		SET last_bucket_collected = EXCLUDED.last_bucket_collected,
		    updated_at = now();
	`
	ctx, cancel := repository.WithQueryTimeout(ctx, 0)
	defer cancel()
	_, err := tsDB.ExecContext(ctx, query, instanceName, bucket)
	return err
}

func (r *PgStatMonitorRepository) UpdateInstanceMetadata(ctx context.Context, serverID uuid.UUID, hasMonitor bool) error {
	// We don't have a direct pg_instance table in 01_timescale_schema.sql,
	// this might be a placeholder or we use optima_servers.
	return nil
}

func (r *PgStatMonitorRepository) FetchBucketMetrics(ctx context.Context, db *sql.DB, bucket int64) ([]PgStatMonitorRow, error) {
	query := `
		SELECT
			bucket_start_time,
			bucket_start_time AS bucket_end_time,
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
	tctx, cancel := repository.WithQueryTimeout(ctx, 0)
	rows, err := db.QueryContext(tctx, query, bucket)
	if err != nil {
		cancel() // cancel if first query fails to allow retry
		if strings.Contains(err.Error(), "column \"bucket_start_time\" does not exist") {
			query2 := `
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
			tctx2, cancel2 := repository.WithQueryTimeout(ctx, 0)
			defer cancel2()
			rows, err = db.QueryContext(tctx2, query2, bucket)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		defer cancel()
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
	tctx, cancel := repository.WithQueryTimeout(ctx, 0)
	err := db.QueryRowContext(tctx, query, bucket).Scan(
		&agg.BucketStartTime,
		&agg.Calls,
		&agg.TotalExecMs,
		&agg.WalBytes,
		&agg.BlocksRead,
		&agg.BlocksHit,
	)
	if err != nil {
		cancel()
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if strings.Contains(err.Error(), "column \"bucket_start_time\" does not exist") {
			query2 := `
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
			tctx2, cancel2 := repository.WithQueryTimeout(ctx, 0)
			defer cancel2()
			err = db.QueryRowContext(tctx2, query2, bucket).Scan(
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
	} else {
		defer cancel()
	}
	agg.WalMB = agg.WalBytes / 1024 / 1024
	return &agg, nil
}

func (r *PgStatMonitorRepository) LogBucketMetrics(ctx context.Context, tsDB *sql.DB, instanceName string, rows []PgStatMonitorRow) error {
	if len(rows) == 0 {
		return nil
	}

	ctx, cancel := repository.WithQueryTimeout(ctx, 0)
	defer cancel()

	tx, err := tsDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()


	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO pg_query_bucket_metrics (
			bucket_start, bucket_end, server_id, dbid, userid, queryid, query,
			application_name, client_ip, calls, total_exec_time, mean_exec_time,
			min_exec_time, max_exec_time, stddev_exec_time, rows,
			shared_blks_hit, shared_blks_read, temp_blks_written, wal_bytes
		) VALUES ($1, $2, (SELECT id FROM optima_servers WHERE name = $3 LIMIT 1), $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, row := range rows {
		_, err := stmt.ExecContext(ctx,
			row.BucketStartTime, row.BucketEndTime, instanceName, row.Dbid, row.Userid, row.Queryid, row.Query,
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
