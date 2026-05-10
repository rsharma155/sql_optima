// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for the pg_stat_statements service-layer methods.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"testing"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
)

// newBareService returns a MetricsService with no repositories or storage
// configured — equivalent to running without TimescaleDB.
func newBareService() *MetricsService {
	return &MetricsService{
		dashboardCache:   make(map[string]models.DashboardMetrics),
		pgDashboardCache: make(map[string]models.PgCoreDashboardCache),
	}
}

// ── nil-tsLogger branch tests ─────────────────────────────────

func TestGetPgssWorkload_NilTsLogger(t *testing.T) {
	s := newBareService()
	pts, err := s.GetPgssWorkload(context.Background(), "any", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pts != nil {
		t.Fatalf("expected nil points with nil tsLogger, got %d", len(pts))
	}
}

func TestGetPgssLatency_NilTsLogger(t *testing.T) {
	s := newBareService()
	pts, err := s.GetPgssLatency(context.Background(), "any", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pts != nil {
		t.Fatalf("expected nil points with nil tsLogger, got %d", len(pts))
	}
}

func TestGetPgssTopQueries_NilTsLogger(t *testing.T) {
	s := newBareService()
	q, err := s.GetPgssTopQueries(context.Background(), "any", time.Now().Add(-time.Hour), time.Now(), "total_time", 50, "", "", "", "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q != nil {
		t.Fatalf("expected nil queries with nil tsLogger, got %d", len(q))
	}
}

func TestGetPgssRegressions_NilTsLogger(t *testing.T) {
	s := newBareService()
	r, err := s.GetPgssRegressions(context.Background(), "any")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r != nil {
		t.Fatalf("expected nil regressions with nil tsLogger, got %d", len(r))
	}
}

// ── GetPgssStatus nil-PgRepo branch ───────────────────────────

func TestGetPgssStatus_NilPgRepo(t *testing.T) {
	s := newBareService()
	resp := s.GetPgssStatus(context.Background(), "pg-test")
	if resp.Instance != "pg-test" {
		t.Fatalf("expected instance=pg-test, got %q", resp.Instance)
	}
	if resp.Ready {
		t.Fatal("expected ready=false with nil PgRepo")
	}
	if resp.Message != "instance connection not available" {
		t.Fatalf("expected connection-unavailable message, got %q", resp.Message)
	}
}

// ── GetPgssStatus with PgRepo but no connection for instance ──

func TestGetPgssStatus_NoConnection(t *testing.T) {
	s := newBareService()
	s.PgRepo = repository.NewPgRepository(&config.Config{})
	resp := s.GetPgssStatus(context.Background(), "unknown-host")
	if resp.Ready {
		t.Fatal("expected ready=false for missing connection")
	}
	if resp.Message != "instance connection not available" {
		t.Fatalf("expected connection-unavailable message, got %q", resp.Message)
	}
}
