// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server locks and blocking service logic.
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

func (s *MetricsService) GetSQLServerBlockingKPIs(ctx context.Context, serverID uuid.UUID) (map[string]interface{}, error) {
	return s.tsLogger.GetSQLServerBlockingKPIs(ctx, serverID)
}

func (s *MetricsService) GetSQLServerBlockingTimeline(ctx context.Context, serverID uuid.UUID, start, end time.Time) ([]map[string]interface{}, error) {
	return s.tsLogger.GetSQLServerBlockingTimeline(ctx, serverID, start, end)
}

func (s *MetricsService) GetSQLServerBlockingDetails(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]models.SQLServerBlockingSnapshot, error) {
	return s.tsLogger.GetSQLServerBlockingDetails(ctx, serverID, from, to)
}

func (s *MetricsService) GetSQLServerBlockingLocks(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]models.SQLServerBlockingLock, error) {
	return s.tsLogger.GetSQLServerBlockingLocks(ctx, serverID, from, to)
}

func (s *MetricsService) GetSQLServerTopBlockingQueries(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	return s.tsLogger.GetSQLServerTopBlockingQueries(ctx, serverID, from, to)
}

func (s *MetricsService) GetSQLServerMostBlockedDatabases(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	return s.tsLogger.GetSQLServerMostBlockedDatabases(ctx, serverID, from, to)
}

func (s *MetricsService) GetSQLServerMostBlockedObjects(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	return s.tsLogger.GetSQLServerMostBlockedObjects(ctx, serverID, from, to)
}

func (s *MetricsService) GetSQLServerBlockingRecurrence(ctx context.Context, serverID uuid.UUID, sqlHash, login string) ([]map[string]interface{}, error) {
	return s.tsLogger.GetSQLServerBlockingRecurrence(ctx, serverID, sqlHash, login)
}
