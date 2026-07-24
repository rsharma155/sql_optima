// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/api/handlers/cold_storage_query.go
// Purpose: Handler for POST /api/cold-storage/query — executes ad-hoc SQL against the
//          Trino coordinator (iceberg catalog) and returns JSON results.
//          Active only when COLD_STORAGE_TRINO_URL is set; returns 503 otherwise.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	// Register the Trino driver for database/sql.
	_ "github.com/trinodb/trino-go-client/trino"

	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/security/sqlsandbox"
)

const (
	trinoQueryTimeout = 60 * time.Second
	trinoMaxRows      = 10_000
	trinoCatalog      = "iceberg"
	trinoSchema       = "default"
)

// ColdQueryHandlers handles ad-hoc Trino queries against the cold storage tier.
type ColdQueryHandlers struct {
	trinoURL string
}

// NewColdQueryHandlers creates a handler using the COLD_STORAGE_TRINO_URL env var.
// Returns nil when the env var is not set — callers must nil-guard before registering routes.
func NewColdQueryHandlers() *ColdQueryHandlers {
	url := os.Getenv("COLD_STORAGE_TRINO_URL")
	if url == "" {
		return nil
	}
	return &ColdQueryHandlers{trinoURL: url}
}

// coldQueryRequest is the POST body accepted by RunQuery.
type coldQueryRequest struct {
	SQL string `json:"sql"`
}

// coldQueryResponse is returned by RunQuery on success.
type coldQueryResponse struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
	RowCount int            `json:"row_count"`
}

// RunQuery executes user-supplied SQL against Trino and returns the results.
//
//	POST /api/cold-storage/query
//	Body: {"sql": "SELECT ..."}
func (h *ColdQueryHandlers) RunQuery(w http.ResponseWriter, r *http.Request) {
	var req coldQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiresponse.WritePlainError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}
	if req.SQL == "" {
		apiresponse.WritePlainError(w, http.StatusBadRequest, "sql field is required", nil)
		return
	}

	if err := sqlsandbox.ValidateReadOnly(sqlsandbox.Options{
		Dialect:  "trino",
		MaxRows:  trinoMaxRows,
		AllowCTE: true,
	}, req.SQL); err != nil {
		apiresponse.WritePlainError(w, http.StatusBadRequest, "sql validation failed: "+err.Error(), nil)
		return
	}

	result, err := h.execTrinoQuery(r.Context(), req.SQL)
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "query failed", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (h *ColdQueryHandlers) execTrinoQuery(ctx context.Context, query string) (*coldQueryResponse, error) {
	dsn := fmt.Sprintf("%s?catalog=%s&schema=%s", h.trinoURL, trinoCatalog, trinoSchema)

	db, err := sql.Open("trino", dsn)
	if err != nil {
		return nil, fmt.Errorf("trino: open: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(ctx, trinoQueryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("trino: query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("trino: columns: %w", err)
	}

	result := &coldQueryResponse{Columns: cols}
	scanBuf := make([]interface{}, len(cols))
	scanPtrs := make([]interface{}, len(cols))
	for i := range scanBuf {
		scanPtrs[i] = &scanBuf[i]
	}

	for rows.Next() {
		if len(result.Rows) >= trinoMaxRows {
			break
		}
		if err := rows.Scan(scanPtrs...); err != nil {
			return nil, fmt.Errorf("trino: scan: %w", err)
		}
		row := make([]interface{}, len(cols))
		copy(row, scanBuf)
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trino: rows: %w", err)
	}

	result.RowCount = len(result.Rows)
	return result, nil
}
