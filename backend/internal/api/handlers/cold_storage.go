// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
//
// Purpose: REST API handlers for monitoring cold storage archival status and run history.

package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/apiresponse"
)

type ColdStorageHandlers struct {
	pool *pgxpool.Pool
}

func NewColdStorageHandlers(pool *pgxpool.Pool) *ColdStorageHandlers {
	return &ColdStorageHandlers{pool: pool}
}

func (h *ColdStorageHandlers) GetStatus(w http.ResponseWriter, r *http.Request) {
	const q = `
		SELECT
			table_name,
			server_name,
			last_exported_at,
			age,
			watermark_updated_at
		FROM coldstorage.status_view`

	rows, err := h.pool.Query(r.Context(), q)
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load cold storage status", err)
		return
	}
	defer rows.Close()

	var res []map[string]any
	for rows.Next() {
		var tableName, serverName string
		var lastExportedAt, age, updatedAt any
		if err := rows.Scan(&tableName, &serverName, &lastExportedAt, &age, &updatedAt); err != nil {
			continue
		}
		res = append(res, map[string]any{
			"table_name":           tableName,
			"server_name":          serverName,
			"last_exported_at":     lastExportedAt,
			"age":                  age,
			"watermark_updated_at": updatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *ColdStorageHandlers) GetRuns(w http.ResponseWriter, r *http.Request) {
	const q = `
		SELECT
			cold_export_run_id,
			run_started,
			run_finished,
			status,
			tables_ok,
			tables_failed,
			total_rows,
			total_bytes,
			error_detail
		FROM coldstorage.runs
		ORDER BY run_started DESC
		LIMIT 50`

	rows, err := h.pool.Query(r.Context(), q)
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load cold storage runs", err)
		return
	}
	defer rows.Close()

	var res []map[string]any
	for rows.Next() {
		var id int64
		var started, finished, status, errDetail any
		var ok, failed int
		var rowsCount, bytes int64
		if err := rows.Scan(&id, &started, &finished, &status, &ok, &failed, &rowsCount, &bytes, &errDetail); err != nil {
			continue
		}
		res = append(res, map[string]any{
			"run_id":        id,
			"run_started":   started,
			"run_finished":  finished,
			"status":        status,
			"tables_ok":     ok,
			"tables_failed": failed,
			"total_rows":    rowsCount,
			"total_bytes":   bytes,
			"error_detail":  errDetail,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
