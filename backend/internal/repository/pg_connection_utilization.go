// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL connection utilization — used vs. max_connections saturation.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
	"math"
	"strings"
)

type PgConnectionUtilization struct {
	MaxConnections   int     `json:"max_connections"`
	UsedConnections  int     `json:"used_connections"`
	UsagePct         float64 `json:"usage_pct"`
	ReservedForSuper int     `json:"reserved_for_superuser"`
}

// FetchConnectionUtilization returns current used/max connections and saturation %.
// A single query combines pg_settings and pg_stat_activity to avoid two round trips.
func (c *PgRepository) FetchConnectionUtilization(instanceName string) (*PgConnectionUtilization, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		if c.reconnectInstance(instanceName) {
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
			(SELECT setting::int FROM pg_settings WHERE name = 'max_connections')                 AS max_conn,
			(SELECT setting::int FROM pg_settings WHERE name = 'superuser_reserved_connections')  AS reserved,
			COUNT(*) FILTER (WHERE pid <> pg_backend_pid() AND backend_type = 'client backend')   AS used_conn
		FROM pg_stat_activity`

	util := &PgConnectionUtilization{}
	err := db.QueryRow(query).Scan(&util.MaxConnections, &util.ReservedForSuper, &util.UsedConnections)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch connection utilization: %v", err)
	}

	if util.MaxConnections > 0 {
		util.UsagePct = (float64(util.UsedConnections) * 100.0) / float64(util.MaxConnections)
		// Clamp to [0, 100]
		util.UsagePct = math.Max(0, math.Min(100, util.UsagePct))
	} else {
		util.UsagePct = 0
	}

	return util, nil
}
