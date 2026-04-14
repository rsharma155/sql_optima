// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Query wrapper for live query execution with timeout and cancellation support.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package live

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
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

func safeQuery(ctx context.Context, db *sql.DB, query string) QueryResult {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	rows, err := db.QueryContext(queryCtx, query)
	if err != nil {
		return handleQueryError(err, queryCtx.Err())
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return QueryResult{
			Success: false,
			Error: &QueryError{
				Code:    "COLUMN_SCAN_ERROR",
				Message: fmt.Sprintf("Failed to read columns: %v", err.Error()),
				Timeout: contextHasTimeout(queryCtx.Err()),
			},
		}
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return QueryResult{
				Success: false,
				Error: &QueryError{
					Code:    "ROW_SCAN_ERROR",
					Message: fmt.Sprintf("Failed to scan row: %v", err.Error()),
					Timeout: contextHasTimeout(queryCtx.Err()),
				},
			}
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			switch v := val.(type) {
			case []byte:
				row[col] = string(v)
			case time.Time:
				row[col] = v.Format(time.RFC3339)
			case nil:
				row[col] = nil
			default:
				row[col] = v
			}
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return QueryResult{
			Success: false,
			Error: &QueryError{
				Code:    "ROWS_ITERATION_ERROR",
				Message: fmt.Sprintf("Error iterating results: %v", err.Error()),
				Timeout: contextHasTimeout(queryCtx.Err()),
			},
		}
	}

	return QueryResult{
		Success: true,
		Data:    results,
		Count:   len(results),
	}
}

func handleQueryError(err error, ctxErr error) QueryResult {
	if contextHasTimeout(ctxErr) {
		return QueryResult{
			Success: false,
			Error: &QueryError{
				Code:    "QUERY_TIMEOUT",
				Message: fmt.Sprintf("Query exceeded %v timeout. Consider simplifying the query or reducing the data scope.", QueryTimeout),
				Timeout: true,
			},
		}
	}

	if err == context.Canceled {
		return QueryResult{
			Success: false,
			Error: &QueryError{
				Code:    "CONTEXT_CANCELLED",
				Message: "Query was cancelled by the client",
				Timeout: false,
			},
		}
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, "microsoft") || strings.Contains(errMsg, "mssql") {
		if strings.Contains(errMsg, "login timeout") || strings.Contains(errMsg, "connection refused") {
			return QueryResult{
				Success: false,
				Error: &QueryError{
					Code:    "CONNECTION_ERROR",
					Message: "Unable to connect to SQL Server. Please check network connectivity and server status.",
					Timeout: false,
				},
			}
		}
		if strings.Contains(errMsg, "permission denied") || strings.Contains(errMsg, "access denied") {
			return QueryResult{
				Success: false,
				Error: &QueryError{
					Code:    "PERMISSION_DENIED",
					Message: "Insufficient permissions to execute this query. Contact your DBA.",
					Timeout: false,
				},
			}
		}
	}

	log.Printf("[LiveQuery] Error executing query: %v", err)
	return QueryResult{
		Success: false,
		Error: &QueryError{
			Code:    "QUERY_ERROR",
			Message: sanitizeErrorMessage(errMsg),
			Timeout: false,
		},
	}
}

func contextHasTimeout(ctxErr error) bool {
	return ctxErr != nil && strings.Contains(ctxErr.Error(), "context deadline exceeded")
}

func sanitizeErrorMessage(msg string) string {
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}
	sensitive := []string{"password", "PASSWORD", "pwd", "PWD", "secret", "SECRET"}
	for _, s := range sensitive {
		if strings.Contains(strings.ToLower(msg), strings.ToLower(s)) {
			return "A database error occurred. Contact your DBA for details."
		}
	}
	return msg
}
