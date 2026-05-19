// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL pg_stat_statements service layer for trend analysis.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"log/slog"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

func (s *MetricsService) StartPostgresQueryStatsCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "Postgres PGSS", 1*time.Minute)
	slog.Info("[PGSS] Starting background collector (interval: %v)", "val", interval)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Dynamic interval check
				newInterval := s.GetCollectorInterval(ctx, "Postgres PGSS", 1*time.Minute)
				if newInterval != interval && newInterval > 0 {
					slog.Info("[PGSS] Frequency changed from", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}

				if interval > 0 {
					for _, inst := range s.Config.Instances {
						if inst.Type == "postgres" {
							if err := s.CollectPostgresQueryStats(ctx, inst.ServerID); err != nil {
								slog.Error("[PGSS] ERROR: Failed to collect PGSS", "target", inst.Name, "err", err)
							}
						}
					}
				}
			}
		}
	}()
}

func (s *MetricsService) CollectPostgresQueryStats(ctx context.Context, serverID uuid.UUID) error {
	instanceName := ""
	for _, inst := range s.Config.Instances {
		if inst.ServerID == serverID {
			instanceName = inst.Name
			break
		}
	}
	if instanceName == "" {
		return fmt.Errorf("instance not found: %s", serverID)
	}

	db, ok := s.PgRepo.GetConn(instanceName)
	if !ok {
		return fmt.Errorf("no connection for %s", instanceName)
	}

	stats, err := s.PgRepo.FetchQuerySnapshot(ctx, instanceName, db)
	if err != nil {
		return err
	}

	var hotRows []hot.PostgresQueryStatsSnapRow
	for _, st := range stats {
		hotRows = append(hotRows, hot.PostgresQueryStatsSnapRow{
			QueryID:           st.QueryID,
			QueryText:         st.Query,
			DbName:            st.DbName,
			UserName:          st.UserName,
			QueryType:         st.QueryType,
			Calls:             st.Calls,
			TotalTimeMs:       st.TotalTime,
			MeanTimeMs:        st.MeanTime,
			Rows:              st.Rows,
			TempBlksRead:      st.TempBlksRead,
			TempBlksWritten:   st.TempBlksWritten,
			BlkReadTimeMs:     st.BlkReadTime,
			BlkWriteTimeMs:    st.BlkWriteTime,
			SharedBlksHit:     st.SharedBlksHit,
			SharedBlksRead:    st.SharedBlksRead,
			SharedBlksDirtied: st.SharedBlksDirtied,
			SharedBlksWritten: st.SharedBlksWritten,
			WalBytes:          st.WalBytes,
			WalRecords:        st.WalRecords,
			WalFpi:            st.WalFpi,
			TotalPlanTime:     st.TotalPlanTime,
			MeanPlanTime:      st.MeanPlanTime,
			Plans:             st.Plans,
		})
	}

	if err := s.tsLogger.LogPostgresQueryStats(ctx, serverID, hotRows); err != nil {
		return err
	}

	// Compute and store deltas for Query Analysis dashboard
	if err := s.tsLogger.ComputeAndStorePgssDelta1m(ctx, serverID, time.Now().UTC()); err != nil {
		slog.Error("[PGSS] ERROR: Failed to compute deltas", "target", instanceName, "err", err)
	}

	return s.tsLogger.UpsertPgssQueryDim(ctx, serverID, hotRows)
}

func (s *MetricsService) GetPgssWorkloadTrend(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]hot.PgssWorkloadPoint, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetPgssWorkloadTimeSeries(ctx, serverID, from, to)
}

func (s *MetricsService) GetPgssTopQueries(ctx context.Context, serverID uuid.UUID, from, to time.Time, sortBy string, limit int, db, user, app, qType string, hideSys bool) ([]hot.PgssTopQuery, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetPgssTopQueries(ctx, serverID, from, to, sortBy, limit, db, user, app, qType, hideSys)
}

func (s *MetricsService) GetPgssLatencyTrend(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]hot.PgssLatencyPoint, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetPgssLatencyTimeSeries(ctx, serverID, from, to)
}

func (s *MetricsService) GetPgssRegressions(ctx context.Context, serverID uuid.UUID) ([]hot.PgssRegression, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetPgssRegressions(ctx, serverID)
}

func (s *MetricsService) GetPgssDbBreakdown(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]hot.PgssDbBreakdown, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetPgssDbBreakdown(ctx, serverID, from, to)
}

func (s *MetricsService) GetPgssUserBreakdown(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]hot.PgssUserBreakdown, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetPgssUserBreakdown(ctx, serverID, from, to)
}

func (s *MetricsService) GetPgssFilterOptions(ctx context.Context, serverID uuid.UUID, from, to time.Time) (*hot.PgssFilterOptions, error) {
	if s.tsLogger == nil {
		return &hot.PgssFilterOptions{}, nil
	}
	return s.tsLogger.GetPgssFilterOptions(ctx, serverID, from, to)
}

func (s *MetricsService) GetPgssSummary(ctx context.Context, serverID uuid.UUID, from, to time.Time) (*hot.PgssSummary, error) {
	if s.tsLogger == nil {
		return &hot.PgssSummary{}, nil
	}
	return s.tsLogger.GetPgssSummary(ctx, serverID, from, to)
}
