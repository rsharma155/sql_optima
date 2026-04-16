// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for the background alert evaluation loop.
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
	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

func TestEngineForInstanceType(t *testing.T) {
	tests := []struct {
		typ    string
		want   alerts.Engine
		wantOK bool
	}{
		{"sqlserver", alerts.EngineSQLServer, true},
		{"postgres", alerts.EnginePostgres, true},
		{"mysql", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := engineForInstanceType(tt.typ)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("engineForInstanceType(%q) = (%q, %v), want (%q, %v)", tt.typ, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestRunOnce_EvaluatesAllInstances(t *testing.T) {
	store := newMockAlertStore()
	maintStore := &mockMaintenanceStore{}
	ev := &mockEvaluator{
		engine: alerts.EngineSQLServer,
		results: []AlertEvaluatorResult{
			{
				RuleName: "mssql_blocking",
				Category: "blocking",
				Severity: alerts.SeverityCritical,
				Title:    "Blocking detected",
			},
		},
	}
	svc := NewAlertService(store, maintStore, []AlertEvaluator{ev})
	cfg := &config.Config{
		Instances: []config.Instance{
			{Name: "db-01", Type: "sqlserver"},
			{Name: "db-02", Type: "sqlserver"},
			{Name: "pg-01", Type: "postgres"}, // no evaluator matches
		},
	}

	runOnce(context.Background(), cfg, svc)

	// Should have alerts from db-01 and db-02 (both sqlserver)
	store.mu.Lock()
	count := len(store.alerts)
	store.mu.Unlock()

	if count != 2 {
		t.Errorf("expected 2 alerts (one per sqlserver instance), got %d", count)
	}
}

func TestStartAlertEvaluationLoop_NilPool(t *testing.T) {
	// Should return immediately without panic when pool is nil
	store := newMockAlertStore()
	svc := NewAlertService(store, &mockMaintenanceStore{}, nil)
	cfg := &config.Config{
		Instances: []config.Instance{{Name: "test-db", Type: "sqlserver"}},
	}
	// nil pool → returns immediately
	StartAlertEvaluationLoop(context.Background(), nil, cfg, svc, time.Second)
}

func TestStartAlertEvaluationLoop_NilSvc(t *testing.T) {
	// Should return immediately without panic
	StartAlertEvaluationLoop(context.Background(), nil, &config.Config{}, nil, time.Second)
}

func TestStartAlertEvaluationLoop_NilCfg(t *testing.T) {
	store := newMockAlertStore()
	svc := NewAlertService(store, &mockMaintenanceStore{}, nil)
	StartAlertEvaluationLoop(context.Background(), nil, nil, svc, time.Second)
}
