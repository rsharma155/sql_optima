// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server connection error identification.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package sqlserver

import (
	"io"
	"strings"
)

// IsMSSQLConnError returns true for transport-level errors that indicate the
// connection to SQL Server was lost (EOF, reset, broken pipe, etc.).
// go-mssqldb often surfaces these as io.EOF rather than driver.ErrBadConn,
// so database/sql's built-in retry doesn't trigger; callers must reconnect.
func IsMSSQLConnError(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "forcibly closed") ||
		strings.Contains(msg, "connection refused")
}
