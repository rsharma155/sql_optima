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
	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/domain"
	"testing"
	"time"
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
		GetSqlServerWorkloadSummary(ctx context.Context, serverID uuid.UUID, from, to time.Time) (*domain.SqlServerWorkloadSummary, error)
		GetSqlServerWorkloadTrends(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]domain.SqlServerWorkloadTrendPoint, error)
		GetSqlServerWorkloadTopOffenders(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]domain.SqlServerWorkloadTopQuery, error)
		GetSqlServerWorkloadAppLoadTimeline(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error)
		GetSqlServerWorkloadLoginLoadTimeline(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error)
		GetSqlServerWorkloadTopApps(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]map[string]interface{}, error)
		GetSqlServerWorkloadTopLogins(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]map[string]interface{}, error)
	} = (*TimescaleLogger)(nil)
}
