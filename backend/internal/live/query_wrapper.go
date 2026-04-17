// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Query wrapper for live query execution with timeout and cancellation support.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package live

import (
	"time"
)

const (
	QueryTimeout = 10 * time.Second
)

type QueryResult struct {
	Success bool                     `json:"success"`
	Data    []map[string]interface{} `json:"data,omitempty"`
	Error   *QueryError              `json:"error,omitempty"`
	Count   int                      `json:"count"`
}

type QueryError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Timeout bool   `json:"timeout"`
}
