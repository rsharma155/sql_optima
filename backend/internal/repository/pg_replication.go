// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL replication monitoring including lag, slot status, and streaming health.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"log/slog"
	"context"
	"database/sql"
)

// CollectPgReplication fetches PostgreSQL replication stats
func (c *PgRepository) CollectPgReplication(ctx context.Context, db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		/* SQL_OPTIMA */ SELECT   
			pid,
			usesysid,
			usename,
			application_name,
			client_addr,
			backend_start,
			backend_xmin,
			state,
			sent_lsn,
			write_lsn,
			flush_lsn,
			replay_lsn,
			COALESCE(write_lag, INTERVAL '0') AS write_lag,
			COALESCE(flush_lag, INTERVAL '0') AS flush_lag,
			COALESCE(replay_lag, INTERVAL '0') AS replay_lag
		FROM pg_stat_replication
		ORDER BY state, backend_start DESC
	`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		slog.Error("[PostgreSQL] Replication Query Error", "err", err)
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var resultsMap = make(map[string]interface{})
		columns, _ := rows.Columns()
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err == nil {
			for i, col := range columns {
				resultsMap[col] = values[i]
			}
			results = append(results, resultsMap)
		}
	}
	return results, nil
}

// CollectPgReplicationLag fetches the worst-case replication lag in MB across all standbys.
// Uses WAL LSN delta (sent vs replay) which is the correct byte-based lag measure.
func (c *PgRepository) CollectPgReplicationLag(ctx context.Context, db *sql.DB) (float64, string, error) {
	query := `
		/* SQL_OPTIMA */
		SELECT
			COALESCE(MAX(pg_wal_lsn_diff(sent_lsn, replay_lsn)), 0) / 1024.0 / 1024.0 AS lag_mb,
			COALESCE((SELECT state FROM pg_stat_replication ORDER BY pg_wal_lsn_diff(sent_lsn, replay_lsn) DESC LIMIT 1), 'none') AS state
		FROM pg_stat_replication
	`

	var lagMB float64
	var state string
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, query).Scan(&lagMB, &state)
	if err != nil {
		slog.Error("[PostgreSQL] Replication Lag Query Error", "err", err)
		return 0, "none", err
	}
	return lagMB, state, nil
}
