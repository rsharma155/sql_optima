// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server query analysis and watched query service logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/models"
)

func (s *MetricsService) GetWatchedQueryStats(ctx context.Context, serverID uuid.UUID, dbName, sqlHash string) (*models.SqlServerWatchedQuerySnapshot, error) {
	return s.MsRepo.FetchWatchedQueryStats(ctx, serverID.String(), dbName, sqlHash)
}

func (s *MetricsService) GetSqlServerQueryWaitStats(ctx context.Context, serverID uuid.UUID, dbName, sqlHash string, from, to time.Time) (interface{}, error) {
	stats, err := s.MsRepo.FetchQueryWaitStats(ctx, serverID.String(), dbName, sqlHash)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"wait_stats": stats}, nil
}

func (s *MetricsService) GetSqlServerQueryPlans(ctx context.Context, serverID uuid.UUID, dbName, sqlHash string, from, to time.Time) (interface{}, error) {
	plans, err := s.MsRepo.FetchQueryPlans(ctx, serverID.String(), dbName, sqlHash)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"plans": plans}, nil
}

func (s *MetricsService) GetSQLServerQueryTrend(ctx context.Context, serverID uuid.UUID, sqlHash string, from, to time.Time) (map[string]interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSQLServerQueryTrend(ctx, serverID, sqlHash, from, to)
}
