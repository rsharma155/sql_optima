// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL query execution interface for repository and collector tasks.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"database/sql"
	"time"
)

// Queryer is implemented by *sql.DB and *sql.Conn.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
}

// defaultQueryTimeout is used when no per-instance timeout is configured.
const defaultQueryTimeout = 20 * time.Second

// WithQueryTimeout derives a child context with a deadline for database calls.
func WithQueryTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = defaultQueryTimeout
	}
	return context.WithTimeout(parent, d)
}
