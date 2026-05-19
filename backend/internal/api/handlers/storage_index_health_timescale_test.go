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

func TestStorageIndexHealthTimescale_IndexUsage_Returns400WhenNoInstance(t *testing.T) {
	h := NewStorageIndexHealthTimescaleHandlers(nil, nil)
	req := httptest.NewRequest("GET", "/api/timescale/storage-index-health/index-usage", nil)
	rr := httptest.NewRecorder()

	h.GetIndexUsage(rr, req)
	if rr.Code != http.StatusServiceUnavailable && rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 503 or 400, got %d", rr.Code)
	}
}

func TestStorageIndexHealthTimescale_IndexUsage_PostgresEngine_Returns400WhenNoInstance(t *testing.T) {
	h := NewStorageIndexHealthTimescaleHandlers(nil, nil)
	req := httptest.NewRequest("GET", "/api/timescale/storage-index-health/index-usage?engine=postgres", nil)
	rr := httptest.NewRecorder()
	h.GetIndexUsage(rr, req)
	if rr.Code != http.StatusServiceUnavailable && rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 503 or 400, got %d", rr.Code)
	}
}

func TestStorageIndexHealthTimescale_Dashboard_PostgresEngine_Returns400WhenNoInstance(t *testing.T) {
	h := NewStorageIndexHealthTimescaleHandlers(nil, nil)
	req := httptest.NewRequest("GET", "/api/timescale/storage-index-health/dashboard?engine=postgres", nil)
	rr := httptest.NewRecorder()
	h.GetDashboard(rr, req)
	if rr.Code != http.StatusServiceUnavailable && rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 503 or 400, got %d", rr.Code)
	}
}
