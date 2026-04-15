// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP tests for PostgreSQL CPU dashboard handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

func TestCPUHistoryMissingInstance(t *testing.T) {
	cfg := &config.Config{}
	h := NewPostgresHandlers(&service.MetricsService{}, cfg)
	req := httptest.NewRequest(http.MethodGet, "/cpu/history", nil)
	rr := httptest.NewRecorder()
	h.CPUHistory(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestCPUSaturationMissingInstance(t *testing.T) {
	cfg := &config.Config{}
	h := NewPostgresHandlers(&service.MetricsService{}, cfg)
	req := httptest.NewRequest(http.MethodGet, "/cpu/saturation", nil)
	rr := httptest.NewRecorder()
	h.CPUSaturation(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}
