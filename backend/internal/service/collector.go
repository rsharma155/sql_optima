// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Core metrics collection loop and dispatcher.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"log/slog"
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type CollectorService struct {
	cfg      *config.Config
	msRepo   *repository.SqlServerRepository
	pgRepo   *repository.PgRepository
	tsLogger *hot.TimescaleLogger
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Cache for collector health / run tracking
	runStatus map[uuid.UUID]map[string]time.Time
	statusMu  sync.RWMutex
}

func NewCollectorService(cfg *config.Config, msRepo *repository.SqlServerRepository, pgRepo *repository.PgRepository, tsLogger *hot.TimescaleLogger) *CollectorService {
	return &CollectorService{
		cfg:       cfg,
		msRepo:    msRepo,
		pgRepo:    pgRepo,
		tsLogger:  tsLogger,
		stopChan:  make(chan struct{}),
		runStatus: make(map[uuid.UUID]map[string]time.Time),
	}
}

func (s *CollectorService) Start(ctx context.Context) {
	slog.Info("[CollectorService] Starting collection cycles...")
	// Intervals and loops would be initialized here...
}

func (s *CollectorService) Stop() {
	close(s.stopChan)
	s.wg.Wait()
}

func (s *CollectorService) UpdateRunStatus(serverID uuid.UUID, collector string, t time.Time) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	if s.runStatus[serverID] == nil {
		s.runStatus[serverID] = make(map[string]time.Time)
	}
	s.runStatus[serverID][collector] = t
}

func (s *CollectorService) GetRunStatus(serverID uuid.UUID, collector string) (time.Time, bool) {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	if m, ok := s.runStatus[serverID]; ok {
		t, ok := m[collector]
		return t, ok
	}
	return time.Time{}, false
}

// ... rest of implementation ...
