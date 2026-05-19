// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for IntelligenceReportService (In-Process version).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v3"
)

func TestNewIntelligenceReportService(t *testing.T) {
	svc := NewIntelligenceReportService(nil)
	if svc.analysisEngine == nil {
		t.Error("expected analysisEngine to be initialized")
	}
}

func TestGetRawDataSnapshot(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewIntelligenceReportService(mock)
	serverID := uuid.New()
	ctx := context.Background()

	// Mock DB calls
	mock.ExpectQuery(`SELECT avg_cpu_load, memory_usage, free_disk_mb, deadlocks, data_disk_mb, log_disk_mb`).
		WithArgs(serverID).WillReturnRows(pgxmock.NewRows([]string{"a", "b", "c", "d", "e", "f"}).AddRow(45.5, 80.0, 10240.0, 0, 50000.0, 10000.0))
	mock.ExpectQuery(`SELECT sql_process, system_idle, other_process`).
		WithArgs(serverID).WillReturnRows(pgxmock.NewRows([]string{"a", "b", "c"}).AddRow(30.0, 60.0, 10.0))
	mock.ExpectQuery(`SELECT sql_memory_used_mb, ple_seconds, memory_grants_pending`).
		WithArgs(serverID).WillReturnRows(pgxmock.NewRows([]string{"a", "b", "c"}).AddRow(16384.0, 300.0, 0.0))
	mock.ExpectQuery(`SELECT blocking_sessions, tempdb_used_percent, ple, buffer_cache_hit_ratio, batch_requests_per_sec`).
		WithArgs(serverID).WillReturnRows(pgxmock.NewRows([]string{"a", "b", "c", "d", "e"}).AddRow(0.0, 5.0, 300.0, 99.5, 1000.0))
	mock.ExpectQuery(`SELECT SUM\(data_mb\), SUM\(log_mb\), SUM\(free_mb\)`).
		WithArgs(serverID).WillReturnRows(pgxmock.NewRows([]string{"sum", "sum", "sum"}).AddRow(100000.0, 20000.0, 50000.0))
	for i := 0; i < 5; i++ {
		mock.ExpectQuery(`SELECT .+ FROM .+ WHERE server_id = \$1 ORDER BY capture_timestamp DESC LIMIT \$2`).
			WithArgs(serverID, 60).WillReturnRows(pgxmock.NewRows([]string{"val"}).AddRow(10.0).AddRow(20.0))
	}

	raw, err := svc.GetRawDataSnapshot(ctx, serverID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if raw["avg_cpu_load"] != 45.5 {
		t.Errorf("expected 45.5, got %v", raw["avg_cpu_load"])
	}
}

func TestAnalyzeInProcess(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewIntelligenceReportService(mock)
	serverID := uuid.New()
	ctx := context.Background()

	// Mock all DB calls
	mock.ExpectQuery(`SELECT avg_cpu_load`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT sql_process`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT sql_memory_used_mb`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT blocking_sessions`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT SUM\(data_mb\)`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	for i := 0; i < 5; i++ {
		mock.ExpectQuery(`SELECT .+ FROM .+`).WillReturnRows(pgxmock.NewRows([]string{"v"}).AddRow(0))
	}

	result, err := svc.Analyze(ctx, serverID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RunID != serverID.String() {
		t.Errorf("expected RunID to be serverID, got %s", result.RunID)
	}
}

func TestCountPopulated(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load":    45.0,
		"sql_process":     30.0,
		"ple_seconds":     500.0,
		"memory_usage":    70.0,
		"free_disk_mb":    5000.0,
		"delta_data_mb":   100.0,
		"blocking_sessions": 0.0, // Should count
		"tempdb_used_percent": 15.0,
	}

	count := countPopulated(raw)
	if count != 8 {
		t.Errorf("expected 8 populated metrics, got %d", count)
	}

	raw["avg_cpu_load_series"] = []float64{1, 2, 3}
	count = countPopulated(raw)
	if count != 9 {
		t.Errorf("expected 9 populated metrics, got %d", count)
	}
}

func TestGetReportInProcess(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	// Set template dir for test
	os.Setenv("SQLOPTIMA_TEMPLATE_DIR", "../intel/templates")
	defer os.Unsetenv("SQLOPTIMA_TEMPLATE_DIR")

	svc := NewIntelligenceReportService(mock)
	serverID := uuid.New()
	ctx := context.Background()

	// Mock all DB calls for GetReport re-running analysis
	mock.ExpectQuery(`SELECT avg_cpu_load`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT sql_process`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT sql_memory_used_mb`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT blocking_sessions`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT SUM\(data_mb\)`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT worker_thread_exhaustion_warning`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT MAX\(secondary_lag_seconds\)`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT AVG\(read_latency_ms\)`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT failed_jobs_24h`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	mock.ExpectQuery(`SELECT cpu_count`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(0))
	for i := 0; i < 6; i++ {
		mock.ExpectQuery(`SELECT .+ FROM .+`).WillReturnRows(pgxmock.NewRows([]string{"v"}).AddRow(0))
	}
	mock.ExpectQuery(`SELECT capture_timestamp, blocking_sessions`).WillReturnRows(pgxmock.NewRows([]string{"a"}).AddRow(time.Now()))

	content, err := svc.GetReport(ctx, serverID.String(), "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(content) == 0 {
		t.Error("expected non-empty content")
	}
}

func TestAnalyzeInsufficientData(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewIntelligenceReportService(mock)
	serverID := uuid.New()
	ctx := context.Background()

	// Mock queries to return empty/error to simulate insufficient data
	mock.ExpectQuery(`SELECT avg_cpu_load`).WillReturnError(fmt.Errorf("no data"))
	mock.ExpectQuery(`SELECT sql_process`).WillReturnError(fmt.Errorf("no data"))
	mock.ExpectQuery(`SELECT sql_memory_used_mb`).WillReturnError(fmt.Errorf("no data"))
	mock.ExpectQuery(`SELECT blocking_sessions`).WillReturnError(fmt.Errorf("no data"))
	mock.ExpectQuery(`SELECT SUM\(data_mb\)`).WillReturnError(fmt.Errorf("no data"))
	mock.ExpectQuery(`SELECT worker_thread_exhaustion_warning`).WillReturnError(fmt.Errorf("no data"))
	mock.ExpectQuery(`SELECT MAX\(secondary_lag_seconds\)`).WillReturnError(fmt.Errorf("no data"))
	mock.ExpectQuery(`SELECT AVG\(read_latency_ms\)`).WillReturnError(fmt.Errorf("no data"))
	mock.ExpectQuery(`SELECT failed_jobs_24h`).WillReturnError(fmt.Errorf("no data"))
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).WillReturnError(fmt.Errorf("no data"))
	mock.ExpectQuery(`SELECT cpu_count`).WillReturnError(fmt.Errorf("no data"))
	for i := 0; i < 6; i++ {
		mock.ExpectQuery(`SELECT .+ FROM .+`).WillReturnError(fmt.Errorf("no data"))
	}
	mock.ExpectQuery(`SELECT capture_timestamp, blocking_sessions`).WillReturnError(fmt.Errorf("no data"))

	result, err := svc.Analyze(ctx, serverID)
	if err != nil {
		t.Fatalf("Analyze should not return error on missing metrics: %v", err)
	}
	if result.DataStatus != "Insufficient" {
		t.Errorf("expected DataStatus Insufficient, got %s", result.DataStatus)
	}
}
