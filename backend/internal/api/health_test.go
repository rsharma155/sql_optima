// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Test suite for health check functionality, verifying instance status and query loading.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleHealthLiveness(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	HandleHealthLiveness(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}
