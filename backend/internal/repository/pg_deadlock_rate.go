// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Aggregate deadlock counter for rate-per-minute computation via delta tracking.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
	"strings"
)

// FetchDeadlocksTotalAllDBs returns sum of deadlocks across all non-template client databases.
// Counter is monotonically increasing (resets on server restart). Used by CC collector
// to compute deadlocks_per_min via TimescaleLogger.ComputePgDeadlockRate().
func (c *PgRepository) FetchDeadlocksTotalAllDBs(instanceName string) (int64, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return 0, fmt.Errorf("connection not found after reconnect for %s", instanceName)
			}
		} else {
			return 0, fmt.Errorf("connection not found for %s", instanceName)
		}
	}

	query := `
		/* SQL_OPTIMA */
		SELECT COALESCE(SUM(deadlocks), 0)
		FROM pg_stat_database
		WHERE datname IS NOT NULL
		  AND datname NOT LIKE 'template%'
		  AND datname <> ''`

	var total int64
	err := db.QueryRow(query).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch aggregate deadlocks: %v", err)
	}

	return total, nil
}
