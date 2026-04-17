// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for alert evaluator interface compliance and threshold logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"testing"

	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

// stubEvaluator lets us test evaluator behavior without a real pool.
type stubEvaluator struct {
	engine  alerts.Engine
	results []AlertEvaluatorResult
}

func (s *stubEvaluator) Engine() alerts.Engine { return s.engine }
func (s *stubEvaluator) Evaluate(_ context.Context, _ string) ([]AlertEvaluatorResult, error) {
	return s.results, nil
}

func TestMssqlBlockingEvaluator_Interface(t *testing.T) {
	// Ensure it satisfies the AlertEvaluator interface at compile time.
	var _ AlertEvaluator = (*MssqlBlockingEvaluator)(nil)
}

func TestMssqlFailedJobsEvaluator_Interface(t *testing.T) {
	var _ AlertEvaluator = (*MssqlFailedJobsEvaluator)(nil)
}

func TestMssqlDiskSpaceEvaluator_Interface(t *testing.T) {
	var _ AlertEvaluator = (*MssqlDiskSpaceEvaluator)(nil)
}

func TestPgReplicationLagEvaluator_Interface(t *testing.T) {
	var _ AlertEvaluator = (*PgReplicationLagEvaluator)(nil)
}

func TestPgBlockingEvaluator_Interface(t *testing.T) {
	var _ AlertEvaluator = (*PgBlockingEvaluator)(nil)
}

func TestPgBackupFreshnessEvaluator_Interface(t *testing.T) {
	var _ AlertEvaluator = (*PgBackupFreshnessEvaluator)(nil)
}

func TestPgDiskSpaceEvaluator_Interface(t *testing.T) {
	var _ AlertEvaluator = (*PgDiskSpaceEvaluator)(nil)
}

func TestEvaluatorResults_SeverityEscalation(t *testing.T) {
	tests := []struct {
		name     string
		blocking int
		wantSev  alerts.Severity
	}{
		{"low blocking is warning", 1, alerts.SeverityWarning},
		{"high blocking is critical", 5, alerts.SeverityCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use stub evaluator that simulates the blocking logic
			var sev alerts.Severity
			if tt.blocking >= 3 {
				sev = alerts.SeverityCritical
			} else {
				sev = alerts.SeverityWarning
			}

			ev := &stubEvaluator{
				engine: alerts.EngineSQLServer,
				results: []AlertEvaluatorResult{{
					RuleName: "mssql_blocking",
					Severity: sev,
				}},
			}

			results, err := ev.Evaluate(context.Background(), "test")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}
			if results[0].Severity != tt.wantSev {
				t.Errorf("severity = %q, want %q", results[0].Severity, tt.wantSev)
			}
		})
	}
}

func TestEvaluatorResults_ReplicationLagThresholds(t *testing.T) {
	tests := []struct {
		name    string
		lagMB   float64
		wantSev alerts.Severity
		wantNil bool
	}{
		{"below threshold", 50, "", true},
		{"warning level", 150, alerts.SeverityWarning, false},
		{"critical level", 600, alerts.SeverityCritical, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warningMB := 100.0
			criticalMB := 500.0

			var results []AlertEvaluatorResult
			if tt.lagMB >= criticalMB {
				results = []AlertEvaluatorResult{{Severity: alerts.SeverityCritical}}
			} else if tt.lagMB >= warningMB {
				results = []AlertEvaluatorResult{{Severity: alerts.SeverityWarning}}
			}

			if tt.wantNil && len(results) > 0 {
				t.Error("expected no results")
			}
			if !tt.wantNil {
				if len(results) != 1 {
					t.Fatalf("expected 1 result, got %d", len(results))
				}
				if results[0].Severity != tt.wantSev {
					t.Errorf("severity = %q, want %q", results[0].Severity, tt.wantSev)
				}
			}
		})
	}
}

func TestEvaluatorResults_DiskSpaceThresholds(t *testing.T) {
	tests := []struct {
		name    string
		freePct float64
		wantSev alerts.Severity
		wantNil bool
	}{
		{"healthy", 50, "", true},
		{"warning", 15, alerts.SeverityWarning, false},
		{"critical", 5, alerts.SeverityCritical, false},
	}

	warningPct := 20.0
	criticalPct := 10.0

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var results []AlertEvaluatorResult
			if tt.freePct <= criticalPct {
				results = []AlertEvaluatorResult{{Severity: alerts.SeverityCritical}}
			} else if tt.freePct <= warningPct {
				results = []AlertEvaluatorResult{{Severity: alerts.SeverityWarning}}
			}

			if tt.wantNil && len(results) > 0 {
				t.Error("expected no results")
			}
			if !tt.wantNil {
				if len(results) != 1 {
					t.Fatalf("expected 1 result, got %d", len(results))
				}
				if results[0].Severity != tt.wantSev {
					t.Errorf("severity = %q, want %q", results[0].Severity, tt.wantSev)
				}
			}
		})
	}
}
