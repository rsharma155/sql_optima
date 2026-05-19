// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Control Center metrics and health monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// FetchWalBytesTotal returns cumulative wal_bytes from pg_stat_wal (PG 14+).
func (c *PgRepository) FetchWalBytesTotal(ctx context.Context, instanceName string) (uint64, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		slog.Info("[POSTGRES] FetchWalBytesTotal: connection not found for %s, attempting reconnect", "val", instanceName)
		if c.reconnectInstance(ctx, instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var bytes sql.NullInt64
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, `/* SQL_OPTIMA */ SELECT   COALESCE(SUM(wal_bytes), 0) FROM pg_stat_wal`).Scan(&bytes)
	if err != nil || !bytes.Valid || bytes.Int64 == 0 {
		// Fallback to LSN diff from 0/0 (works on PG 10+)
		ctx, cancel = WithQueryTimeout(ctx, 0)
		defer cancel()
		err = db.QueryRowContext(ctx, `
			/* SQL_OPTIMA */
			SELECT  pg_wal_lsn_diff(
				CASE WHEN pg_is_in_recovery() THEN pg_last_wal_replay_lsn() ELSE pg_current_wal_lsn() END,
				'0/0'
			)
		`).Scan(&bytes)
		if err != nil {
			return 0, err
		}
	}
	if !bytes.Valid || bytes.Int64 < 0 {
		return 0, nil
	}
	return uint64(bytes.Int64), nil
}

// FetchWalDirSizeMB returns total size of files in pg_wal directory (MB).
func (c *PgRepository) FetchWalDirSizeMB(ctx context.Context, instanceName string) (float64, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(ctx, instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var mb float64
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, `/* SQL_OPTIMA */ SELECT   COALESCE(SUM(size), 0) / 1024.0 / 1024.0 FROM pg_ls_waldir()`).Scan(&mb)
	return mb, err
}

// FetchActiveWaitingSessions returns counts from pg_stat_activity.
func (c *PgRepository) FetchActiveWaitingSessions(ctx context.Context, instanceName string) (active int, waiting int, err error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(ctx, instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, 0, fmt.Errorf("connection not found")
	}

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err = db.QueryRowContext(ctx, `
	/* SQL_OPTIMA */
		SELECT
			COUNT(*) FILTER (WHERE state = 'active')                 AS active,
			COUNT(*) FILTER (WHERE wait_event IS NOT NULL)           AS waiting
		FROM pg_stat_activity
		WHERE backend_type = 'client backend'
		  AND pid <> pg_backend_pid()
	`).Scan(&active, &waiting)
	return active, waiting, err
}

// FetchSlowQueriesCount returns number of queries in pg_stat_statements with mean_exec_time > thresholdMs.
func (c *PgRepository) FetchSlowQueriesCount(ctx context.Context, instanceName string, thresholdMs float64) (int, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(ctx, instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}
	var cnt int
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, `/* SQL_OPTIMA */ SELECT   COUNT(*) FROM pg_stat_statements WHERE mean_exec_time > $1`, thresholdMs).Scan(&cnt)
	return cnt, err
}

func (c *PgRepository) FetchBlockingSessionsCount(ctx context.Context, instanceName string) (int, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(ctx, instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var cnt int
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, `
		/* SQL_OPTIMA */
		SELECT COUNT(*)
		FROM pg_stat_activity
		WHERE wait_event_type = 'Lock'
		  AND backend_type = 'client backend'
		  AND pid <> pg_backend_pid()
	`).Scan(&cnt)
	return cnt, err
}

func (c *PgRepository) FetchAutovacuumWorkers(ctx context.Context, instanceName string) (int, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(ctx, instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var cnt int
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, `/* SQL_OPTIMA */ SELECT COUNT(*) FROM pg_stat_activity WHERE backend_type = 'autovacuum worker'`).Scan(&cnt)
	return cnt, err
}

func (c *PgRepository) FetchDeadTupleRatioPct(ctx context.Context, instanceName string) (float64, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(ctx, instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var pct float64
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, `
	/* SQL_OPTIMA */
	SELECT
		COALESCE(SUM(n_dead_tup) * 100.0 / NULLIF(SUM(n_live_tup + n_dead_tup), 0), 0) AS dead_tuple_pct
	FROM pg_stat_user_tables
	`).Scan(&pct)
	return pct, err
}

func (c *PgRepository) FetchReplicaLagSec(ctx context.Context, instanceName string) (float64, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(ctx, instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var lag float64
	// Try primary-side check first (max lag of all standbys)
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, `
		SELECT /* SQL_OPTIMA */ 
			COALESCE(MAX(EXTRACT(EPOCH FROM replay_lag)), 0)
		FROM pg_stat_replication
	`).Scan(&lag)

	// If it returns 0 or fails (e.g. column doesn't exist in old PG), try standby-side check
	if err != nil || lag == 0 {
		var isRecovery bool
		ctx, cancel = WithQueryTimeout(ctx, 0)
		defer cancel()
		_ = db.QueryRowContext(ctx, "SELECT pg_is_in_recovery()").Scan(&isRecovery)
		if isRecovery {
			ctx, cancel = WithQueryTimeout(ctx, 0)
			defer cancel()
			_ = db.QueryRowContext(ctx, `
				SELECT /* SQL_OPTIMA */
					COALESCE(EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp())), 0)
			`).Scan(&lag)
		}
	}

	return lag, nil
}

func (c *PgRepository) FetchCacheHitRatioPct(ctx context.Context, instanceName string) (float64, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(ctx, instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var pct float64
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, `
	/* SQL_OPTIMA */ 
		SELECT  
			CASE WHEN (SUM(blks_hit) + SUM(blks_read)) > 0
				THEN (SUM(blks_hit)::float * 100.0) / (SUM(blks_hit) + SUM(blks_read))
				ELSE 0
			END
		FROM pg_stat_database
		WHERE datname IS NOT NULL
		  AND datname NOT LIKE 'template%';
	`).Scan(&pct)
	return pct, err
}

// FetchXactTotal returns the sum of xact_commit and xact_rollback from pg_stat_database.
func (c *PgRepository) FetchXactTotal(ctx context.Context, instanceName string) (uint64, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(ctx, instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var total sql.NullInt64
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, `/* SQL_OPTIMA */ SELECT COALESCE(SUM(xact_commit + xact_rollback), 0) FROM pg_stat_database`).Scan(&total)
	if err != nil {
		return 0, err
	}
	return uint64(total.Int64), nil
}

func retainedWalMBFromBytes(b int64) float64 {
	if b <= 0 {
		return 0
	}
	return float64(b) / 1024.0 / 1024.0
}
