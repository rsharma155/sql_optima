// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Retry SQL Server snapshot queries after transport-level connection loss.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package sqlserver

import (
	"context"
	"database/sql"

	ms "github.com/rsharma155/sql_optima/internal/sqlserver"
)

// ConnRecovery replaces the repository pool after EOF/reset-style errors.
// Implementations should reopen the target instance and return the new *sql.DB.
type ConnRecovery func(ctx context.Context) (*sql.DB, bool)

// SetConnRecovery configures pool refresh on transport errors (optional).
func (r *SQLServerSnapshotRepository) SetConnRecovery(recovery ConnRecovery) {
	r.recovery = recovery
}

func withConnRetry[T any](
	r *SQLServerSnapshotRepository,
	ctx context.Context,
	fn func(db *sql.DB) (T, error),
) (T, error) {
	out, err := fn(r.db)
	if err == nil || !ms.IsMSSQLConnError(err) {
		return out, err
	}
	if r.recovery != nil {
		if newDB, ok := r.recovery(ctx); ok && newDB != nil {
			r.db = newDB
			return fn(r.db)
		}
		return out, err
	}
	return fn(r.db)
}
