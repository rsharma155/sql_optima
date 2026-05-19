/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * File: sqlserver_locks_test.go
 * Purpose: Tests for SQL Server blocking and deadlock handlers.
 * Metadata:
 *   - Type: Backend Test
 *   - Coverage: Time range parsing, Handler structure
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseTimeRange(t *testing.T) {

	t.Run("Default Range", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/test", nil)
		from, to := ParseTimeRange(req.URL.Query().Get("from"), req.URL.Query().Get("to"))

		now := time.Now().UTC()
		if to.After(now) {
			t.Errorf("expected 'to' time to be now or earlier, got %v", to)
		}
		expectedFrom := to.Add(-1 * time.Hour)
		if !from.Equal(expectedFrom) {
			t.Errorf("expected 'from' to be 1 hour before 'to', got %v vs %v", from, expectedFrom)
		}
	})

	t.Run("Custom Range", func(t *testing.T) {
		fromStr := "2026-05-01T10:00:00Z"
		toStr := "2026-05-01T12:00:00Z"
		req, _ := http.NewRequest("GET", "/api/test?from="+fromStr+"&to="+toStr, nil)
		from, to := ParseTimeRange(req.URL.Query().Get("from"), req.URL.Query().Get("to"))

		expectedFrom, _ := time.Parse(time.RFC3339, fromStr)
		expectedTo, _ := time.Parse(time.RFC3339, toStr)

		if !from.Equal(expectedFrom.UTC()) {
			t.Errorf("expected %v, got %v", expectedFrom, from)
		}
		if !to.Equal(expectedTo.UTC()) {
			t.Errorf("expected %v, got %v", expectedTo, to)
		}
	})

	t.Run("Invalid Range Fallback", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/test?from=invalid&to=invalid", nil)
		from, to := ParseTimeRange(req.URL.Query().Get("from"), req.URL.Query().Get("to"))

		if from.IsZero() || to.IsZero() {
			t.Error("expected non-zero times for invalid input fallback")
		}
	})
}

func TestBlockingKPIs_MissingInstance(t *testing.T) {
	h := &SqlServerHandlers{}
	req, _ := http.NewRequest("GET", "/api/sqlserver/blocking/kpis", nil)
	rr := httptest.NewRecorder()

	h.BlockingKPIs(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}
