// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL deadlock detection and history tracking.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type PgDeadlockTotalRow struct {
	DatabaseName   string `json:"database_name"`
	DeadlocksTotal int64  `json:"deadlocks_total"`
}

func (c *PgRepository) GetDeadlocksTotalByDB(ctx context.Context, instanceName string) ([]PgDeadlockTotalRow, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	q := `
		/* SQL_OPTIMA */ SELECT   datname, deadlocks
		FROM pg_stat_database
		WHERE datname IS NOT NULL AND datname <> ''
		ORDER BY deadlocks DESC, datname
	`
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PgDeadlockTotalRow
	for rows.Next() {
		var r PgDeadlockTotalRow
		var dl sql.NullInt64
		if err := rows.Scan(&r.DatabaseName, &dl); err != nil {
			continue
		}
		if dl.Valid {
			r.DeadlocksTotal = dl.Int64
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
