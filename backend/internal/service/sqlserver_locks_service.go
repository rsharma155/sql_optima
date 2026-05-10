// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: sqlserver_locks_service.go
// Purpose: Service methods for SQL Server blocking and deadlock analysis.
// Metadata:
//   - Version: 1.1
//   - Package: service
//   - Domain: Blocking, Deadlocks, Objects, Timeline
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

func (s *MetricsService) GetSQLServerBlockingKPIs(ctx context.Context, instanceID string) (map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetSQLServerBlockingKPIs(ctx, instanceID)
}

func (s *MetricsService) GetSQLServerBlockingTimeline(ctx context.Context, instanceID string, start, end time.Time) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetSQLServerBlockingTimeline(ctx, instanceID, start, end)
}

func (s *MetricsService) GetSQLServerBlockingDetails(ctx context.Context, instanceID string, start, end time.Time) ([]models.SQLServerBlockingSnapshot, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetSQLServerBlockingDetails(ctx, instanceID, start, end)
}

func (s *MetricsService) GetSQLServerBlockingLocks(ctx context.Context, instanceID string, start, end time.Time) ([]models.SQLServerBlockingLock, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetSQLServerBlockingLocks(ctx, instanceID, start, end)
}

func (s *MetricsService) GetSQLServerTopBlockingQueries(ctx context.Context, instanceID string, start, end time.Time) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetSQLServerTopBlockingQueries(ctx, instanceID, start, end)
}

func (s *MetricsService) GetSQLServerMostBlockedDatabases(ctx context.Context, instanceID string, start, end time.Time) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetSQLServerMostBlockedDatabases(ctx, instanceID, start, end)
}

func (s *MetricsService) GetSQLServerMostBlockedObjects(ctx context.Context, instanceID string, start, end time.Time) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetSQLServerMostBlockedObjects(ctx, instanceID, start, end)
}

func (s *MetricsService) GetSQLServerDeadlockHistory(ctx context.Context, instanceID string, start, end time.Time) ([]models.SQLServerDeadlockEvent, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetSQLServerDeadlockHistory(ctx, instanceID, start, end)
}

func (s *MetricsService) GetDeadlockXESessionStatus(instanceName string) (bool, string) {
	db, ok := s.MsRepo.GetConn(instanceName)
	if !ok || db == nil {
		return false, "Connection not found"
	}
	enabled, err := s.MsRepo.IsDeadlockXESessionEnabled(db)
	if err != nil {
		return false, err.Error()
	}
	if !enabled {
		return false, "Extended Event session 'dbamon_deadlocks' is not enabled or user lacks permission to create it. Deadlock details will not be available."
	}
	return true, "Enabled"
}

func (s *MetricsService) GetSQLServerBlockingRecurrence(ctx context.Context, instanceID, sqlHash, loginName string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetSQLServerBlockingRecurrence(ctx, instanceID, sqlHash, loginName)
}
