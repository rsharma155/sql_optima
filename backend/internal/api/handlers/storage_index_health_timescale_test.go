// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Test suite for storage index health functionality.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStorageIndexHealthTimescale_IndexUsage_Returns503WhenTimescaleNotConfigured(t *testing.T) {
	h := NewStorageIndexHealthTimescaleHandlers(nil)
	req := httptest.NewRequest("GET", "/api/timescale/storage-index-health/index-usage?engine=sqlserver&instance=x", nil)
	rr := httptest.NewRecorder()

	h.IndexUsage(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

func TestStorageIndexHealthTimescale_IndexUsage_PostgresEngine_Returns503WhenTimescaleNotConfigured(t *testing.T) {
	h := NewStorageIndexHealthTimescaleHandlers(nil)
	req := httptest.NewRequest("GET", "/api/timescale/storage-index-health/index-usage?engine=postgres&instance=pg1", nil)
	rr := httptest.NewRecorder()
	h.IndexUsage(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

func TestStorageIndexHealthTimescale_Dashboard_PostgresEngine_Returns503WhenTimescaleNotConfigured(t *testing.T) {
	h := NewStorageIndexHealthTimescaleHandlers(nil)
	req := httptest.NewRequest("GET", "/api/timescale/storage-index-health/dashboard?engine=postgres&instance=pg1&time_range=24h", nil)
	rr := httptest.NewRecorder()
	h.Dashboard(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

