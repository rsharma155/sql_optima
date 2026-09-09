// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Structured cold-history query endpoint using federation allowlist (safer than free SQL).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/storage/cold/federation"
)

type coldHistoryRequest struct {
	Table    string `json:"table"`
	ServerID string `json:"server_id"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// RunHistoryQuery executes an allowlisted Iceberg history SELECT via Trino.
//
//	POST /api/cold-storage/history
//	Body: {"table":"sqlserver_cpu_history","server_id":"<uuid>","from":"<rfc3339>","to":"<rfc3339>"}
func (h *ColdQueryHandlers) RunHistoryQuery(w http.ResponseWriter, r *http.Request) {
	if h == nil {
		apiresponse.WriteJSONError(w, http.StatusServiceUnavailable, "cold query not configured", nil)
		return
	}

	var req coldHistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	from, to, err := parseColdHistoryWindow(req.From, req.To)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}

	hotDays := coldHotRetentionDays()
	needsCold := federation.NeedsColdLookback(from, to, hotDays)
	sqlText, err := federation.BuildIcebergHistorySQL(req.Table, req.ServerID, from, to)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}

	result, err := h.execTrinoQuery(r.Context(), sqlText)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "cold history query failed", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"columns":              result.Columns,
		"rows":                 result.Rows,
		"row_count":            result.RowCount,
		"needs_cold_lookback":  needsCold,
		"hot_retention_days":   hotDays,
		"table":                federation.SanitizeIcebergTableName(req.Table),
		"source":               "trino",
	})
}

func parseColdHistoryWindow(fromS, toS string) (time.Time, time.Time, error) {
	if fromS == "" || toS == "" {
		return time.Time{}, time.Time{}, errColdHistory("from and to are required (RFC3339)")
	}
	from, err := time.Parse(time.RFC3339, fromS)
	if err != nil {
		return time.Time{}, time.Time{}, errColdHistory("invalid from timestamp")
	}
	to, err := time.Parse(time.RFC3339, toS)
	if err != nil {
		return time.Time{}, time.Time{}, errColdHistory("invalid to timestamp")
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, errColdHistory("from must be before to")
	}
	return from.UTC(), to.UTC(), nil
}

type coldHistoryError string

func (e coldHistoryError) Error() string { return string(e) }

func errColdHistory(msg string) error { return coldHistoryError(msg) }

func coldHotRetentionDays() int {
	if v := os.Getenv("COLD_STORAGE_HOT_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 90
}
