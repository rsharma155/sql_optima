// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Extended session state breakdown (5-way) excluding background workers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"fmt"
	"strings"
)

type PgSessionStateFull struct {
	Active           int `json:"active"`
	Idle             int `json:"idle"`
	IdleInTxn        int `json:"idle_in_txn"`
	IdleInTxnAborted int `json:"idle_in_txn_aborted"`
	Waiting          int `json:"waiting"`
	Total            int `json:"total"`
}

// FetchSessionStateFull returns 5-way session breakdown for client backends only.
// Excludes autovacuum, bgwriter, checkpointer, and the monitoring connection itself.
func (c *PgRepository) FetchSessionStateFull(ctx context.Context, instanceName string) (*PgSessionStateFull, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		if c.reconnectInstance(ctx, instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return nil, fmt.Errorf("connection not found after reconnect for %s", instanceName)
			}
		} else {
			return nil, fmt.Errorf("connection not found for %s", instanceName)
		}
	}

	query := `
		/* SQL_OPTIMA */
		SELECT
			COUNT(*) FILTER (WHERE state = 'active'
				AND (wait_event_type IS NULL OR wait_event_type NOT IN ('Lock','IO')))    AS active_cnt,
			COUNT(*) FILTER (WHERE state = 'idle')                                         AS idle_cnt,
			COUNT(*) FILTER (WHERE state = 'idle in transaction')                          AS idle_in_txn_cnt,
			COUNT(*) FILTER (WHERE state = 'idle in transaction (aborted)')                AS idle_in_txn_abort_cnt,
			COUNT(*) FILTER (WHERE state = 'active'
				AND wait_event_type IS NOT NULL)                                            AS waiting_cnt,
			COUNT(*)                                                                        AS total_cnt
		FROM pg_stat_activity
		WHERE backend_type = 'client backend'
		  AND pid <> pg_backend_pid()`

	stats := &PgSessionStateFull{}
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, query).Scan(
		&stats.Active,
		&stats.Idle,
		&stats.IdleInTxn,
		&stats.IdleInTxnAborted,
		&stats.Waiting,
		&stats.Total,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch extended session states: %v", err)
	}

	return stats, nil
}
