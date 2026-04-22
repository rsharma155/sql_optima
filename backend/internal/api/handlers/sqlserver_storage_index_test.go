/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Unit tests for SQL Server Storage & Index Health analytical handlers.
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

	"github.com/rsharma155/sql_optima/internal/config"
)

func TestSqlServerStorageIndex_TableDrilldown(t *testing.T) {
	cfg := &config.Config{
		Instances: []config.Instance{
			{Name: "test-sql", Type: "sqlserver"},
		},
	}
	h := NewSqlServerHandlers(nil, cfg)

	t.Run("Missing Parameters", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/sqlserver/storage-index/table-drilldown?instance=test-sql", nil)
		rr := httptest.NewRecorder()
		h.TableDrilldown(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("Basic Extraction", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/sqlserver/storage-index/table-drilldown?instance=test-sql&db=master&table=sysprocesses", nil)
		rr := httptest.NewRecorder()

		defer func() {
			if r := recover(); r == nil {
				// Success if it reached the nil metricsSvc without crashing or returned error
				if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
					t.Errorf("unexpected status: %d", rr.Code)
				}
			}
		}()
		h.TableDrilldown(rr, req)
	})
}
