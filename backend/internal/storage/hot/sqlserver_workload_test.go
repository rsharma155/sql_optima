// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for SQL Server Workload storage-layer models and interfaces.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/domain"
)

func TestSqlServerWorkloadSummaryStructExists(t *testing.T) {
	var _ domain.SqlServerWorkloadSummary
}

func TestSqlServerWorkloadTrendPointStructExists(t *testing.T) {
	var _ domain.SqlServerWorkloadTrendPoint
}

func TestSqlServerWorkloadTopQueryStructExists(t *testing.T) {
	var _ domain.SqlServerWorkloadTopQuery
}

func TestTimescaleLogger_WorkloadInterface(t *testing.T) {
	// This test ensures the TimescaleLogger implements the expected workload methods.
	var _ interface {
		GetSqlServerWorkloadSummary(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) (*domain.SqlServerWorkloadSummary, error)
		GetSqlServerWorkloadTrends(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) ([]domain.SqlServerWorkloadTrendPoint, error)
		GetSqlServerWorkloadTopOffenders(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int, filter domain.WorkloadQueryFilter) ([]domain.SqlServerWorkloadTopQuery, error)
		GetSqlServerWorkloadAppLoadTimeline(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) ([]map[string]interface{}, error)
		GetSqlServerWorkloadLoginLoadTimeline(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) ([]map[string]interface{}, error)
		GetSqlServerWorkloadTopApps(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int, filter domain.WorkloadQueryFilter) ([]map[string]interface{}, error)
		GetSqlServerWorkloadTopLogins(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int, filter domain.WorkloadQueryFilter) ([]map[string]interface{}, error)
	} = (*TimescaleLogger)(nil)
}

func TestWorkloadMetricsFilterSQL(t *testing.T) {
	excluded := workloadMetricsFilterSQL(domain.WorkloadQueryFilter{ExcludeSystem: true})
	if excluded == "" || !strings.Contains(excluded, "sys.dm_") || !strings.Contains(excluded, "%/* SQL_OPTIMA%") {
		t.Fatalf("expected user-workload filter: %s", excluded)
	}
	if !strings.Contains(excluded, "statement_text") {
		t.Fatalf("expected system noise on statement_text: %s", excluded)
	}
	if !strings.Contains(excluded, "distribution") {
		t.Fatalf("expected distribution exclusion in workload scope: %s", excluded)
	}
	if !strings.Contains(excluded, "is_user_workload") {
		t.Fatalf("exclude_system=true should filter on is_user_workload: %s", excluded)
	}
	if strings.Contains(excluded, "NOT LIKE 'SET %'") {
		t.Fatalf("filter must not drop CRUD batches: %s", excluded)
	}
	unchecked := workloadMetricsFilterSQL(domain.WorkloadQueryFilter{ExcludeSystem: false})
	if !strings.Contains(unchecked, "%/* SQL_OPTIMA%") || !strings.Contains(unchecked, "query_text_raw") {
		t.Fatalf("unchecked should exclude collector SQL on query_text_raw: %s", unchecked)
	}
	if strings.Contains(unchecked, "is_user_workload") {
		t.Fatal("unchecked should not gate on is_user_workload")
	}
}

func TestWorkloadUsesDatabaseScope(t *testing.T) {
	if !workloadUsesDatabaseScope(domain.WorkloadQueryFilter{Database: "mydb"}) {
		t.Fatal("expected database scope")
	}
	if workloadUsesDatabaseScope(domain.WorkloadQueryFilter{Database: "all"}) {
		t.Fatal("all should not scope")
	}
}

// Ensures LIKE '%sys.dm_%' in the noise filter is not passed through fmt.Sprintf.
func TestWorkloadTrendsQueryWithFilterAvoidsSprintfPercent(t *testing.T) {
	bucketSize := "1 minute"
	filter := domain.WorkloadQueryFilter{ExcludeSystem: true}
	query := fmt.Sprintf(`
		SELECT time_bucket('%s', capture_timestamp) AS bucket
		FROM sqlserver_query_metrics_v2
		WHERE server_id = $1 AND database_name = $4
	`, bucketSize) + workloadMetricsFilterSQL(filter)
	if !containsAll(query, "1 minute", "%sys.dm_%", "%/* SQL_OPTIMA%", "query_text_raw") {
		t.Fatalf("query missing expected fragments: %s", query)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
