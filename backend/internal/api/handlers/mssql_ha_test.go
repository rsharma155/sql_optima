// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for SQL Server HA and log-shipping HTTP handlers (Epic 2.2).
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

// helpers ────────────────────────────────────────────────────────────────────

func newMssqlHandler() *MssqlHandlers {
	cfg := &config.Config{
		Instances: []config.Instance{
			{Name: "test-sql", Type: "sqlserver"},
		},
	}
	return NewMssqlHandlers(&service.MetricsService{}, cfg)
}

// ── LogShipping ──────────────────────────────────────────────────────────────

func TestLogShipping_MissingInstance_Returns400(t *testing.T) {
	h := newMssqlHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/log-shipping", nil)
	rr := httptest.NewRecorder()
	h.LogShipping(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

func TestLogShipping_InvalidInstanceName_Returns400(t *testing.T) {
	h := newMssqlHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/log-shipping?instance=../../etc/passwd", nil)
	rr := httptest.NewRecorder()
	h.LogShipping(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

func TestLogShipping_InstanceNotFound_Returns404(t *testing.T) {
	h := newMssqlHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/log-shipping?instance=unknown-instance", nil)
	rr := httptest.NewRecorder()
	h.LogShipping(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestLogShipping_WrongInstanceType_Returns400(t *testing.T) {
	cfg := &config.Config{
		Instances: []config.Instance{
			{Name: "pg-instance", Type: "postgres"},
		},
	}
	h := NewMssqlHandlers(&service.MetricsService{}, cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/log-shipping?instance=pg-instance", nil)
	rr := httptest.NewRecorder()
	h.LogShipping(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

// ── Jobs ─────────────────────────────────────────────────────────────────────

func TestJobs_MissingInstance_Returns400(t *testing.T) {
	h := newMssqlHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/jobs", nil)
	rr := httptest.NewRecorder()
	h.Jobs(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

func TestJobs_InvalidInstanceName_Returns400(t *testing.T) {
	h := newMssqlHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/jobs?instance=<script>alert(1)</script>", nil)
	rr := httptest.NewRecorder()
	h.Jobs(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

func TestJobs_InstanceNotFound_Returns404(t *testing.T) {
	h := newMssqlHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/jobs?instance=nonexistent", nil)
	rr := httptest.NewRecorder()
	h.Jobs(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

// ── AGHealth ─────────────────────────────────────────────────────────────────

func TestAGHealth_MissingInstance_Returns400(t *testing.T) {
	h := newMssqlHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/ag-health", nil)
	rr := httptest.NewRecorder()
	h.AGHealth(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

func TestAGHealth_WrongInstanceType_Returns400(t *testing.T) {
	cfg := &config.Config{
		Instances: []config.Instance{
			{Name: "pg-replica", Type: "postgres"},
		},
	}
	h := NewMssqlHandlers(&service.MetricsService{}, cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/ag-health?instance=pg-replica", nil)
	rr := httptest.NewRecorder()
	h.AGHealth(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}
