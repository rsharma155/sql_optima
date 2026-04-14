// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL query execution utility for collector tasks.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"context"
	"database/sql"
)

// Queryer is implemented by *sql.DB and *sql.Conn.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
}
