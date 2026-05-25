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

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
)

// newBareService returns a MetricsService with no repositories or storage
// configured — equivalent to running without TimescaleDB.
func newBareService() *MetricsService {
	return &MetricsService{
		dashboardCache:   make(map[uuid.UUID]models.DashboardMetrics),
		pgDashboardCache: make(map[uuid.UUID]models.PgCoreDashboardCache),
	}
}

// ── nil-tsLogger branch tests ─────────────────────────────────

func TestGetPgssWorkload_NilTsLogger(t *testing.T) {
	s := newBareService()
	pts, err := s.GetPgssWorkloadTrend(context.Background(), uuid.New(), time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pts != nil {
		t.Fatalf("expected nil points with nil tsLogger, got %d", len(pts))
	}
}

func TestGetPgssLatency_NilTsLogger(t *testing.T) {
	s := newBareService()
	pts, err := s.GetPgssLatencyTrend(context.Background(), uuid.New(), time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pts != nil {
		t.Fatalf("expected nil points with nil tsLogger, got %d", len(pts))
	}
}

func TestGetPgssTopQueries_NilTsLogger(t *testing.T) {
	s := newBareService()
	q, err := s.GetPgssTopQueries(context.Background(), uuid.New(), time.Now().Add(-time.Hour), time.Now(), "total_time", 50, "", "", "", "", true, []string{"postgres"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q != nil {
		t.Fatalf("expected nil queries with nil tsLogger, got %d", len(q))
	}
}

func TestGetPgssRegressions_NilTsLogger(t *testing.T) {
	s := newBareService()
	r, err := s.GetPgssRegressions(context.Background(), uuid.New())
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
	id := uuid.New()
	resp, _ := s.GetPgssStatus(context.Background(), id)
	// GetPgssStatus returns interface{} which is a map
	m := resp.(map[string]interface{})
	if m["enabled"] == true {
		t.Fatal("expected enabled=false with nil PgRepo")
	}
}

// ── GetPgssStatus with PgRepo but no connection for serverID ──

func TestGetPgssStatus_NoConnection(t *testing.T) {
	s := newBareService()
	s.PgRepo = repository.NewPgRepository(context.Background(), &config.Config{})
	resp, _ := s.GetPgssStatus(context.Background(), uuid.New())
	m := resp.(map[string]interface{})
	if m["enabled"] == true {
		t.Fatal("expected enabled=false for missing connection")
	}
}
