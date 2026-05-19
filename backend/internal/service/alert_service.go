// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Alert engine orchestrator – runs evaluators, deduplicates via
//
//	fingerprint, respects maintenance windows, manages acknowledge/resolve.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

// AlertEvaluatorResult is the output from a single evaluator check.
type AlertEvaluatorResult struct {
	RuleName    string
	Category    string
	Severity    alerts.Severity
	Title       string
	Description string
	Evidence    map[string]interface{}
	ServerID    uuid.UUID
	ServerName  string
	Engine      alerts.Engine
}

// AlertEvaluator is the interface each engine-specific evaluator implements.
// Evaluate runs all checks for the given serverID and returns zero or more results.
type AlertEvaluator interface {
	Evaluate(ctx context.Context, serverID uuid.UUID) ([]AlertEvaluatorResult, error)
	Engine() alerts.Engine
}

// AlertService orchestrates alert evaluation, de-duplication, and lifecycle.
type AlertService struct {
	alertStore       alerts.AlertStore
	maintenanceStore alerts.MaintenanceStore
	evaluators       []AlertEvaluator
	notifier         *Notifier
}

func NewAlertService(
	alertStore alerts.AlertStore,
	maintenanceStore alerts.MaintenanceStore,
	evaluators []AlertEvaluator,
	notifier *Notifier,
) *AlertService {
	return &AlertService{
		alertStore:       alertStore,
		maintenanceStore: maintenanceStore,
		evaluators:       evaluators,
		notifier:         notifier,
	}
}

// RunEvaluation executes all evaluators for a given serverID and upserts alerts.
// Returns the number of new/bumped alerts.
func (s *AlertService) RunEvaluation(ctx context.Context, serverID uuid.UUID, engine alerts.Engine) (int, error) {
	now := time.Now().UTC()

	// Check maintenance window
	underMaint, err := s.maintenanceStore.IsUnderMaintenance(ctx, serverID, engine, now)
	if err != nil {
		return 0, err
	}
	if underMaint {
		return 0, nil
	}

	var count int
	for _, ev := range s.evaluators {
		if ev.Engine() != engine {
			continue
		}
		results, err := ev.Evaluate(ctx, serverID)
		if err != nil {
			slog.WarnContext(ctx, "alert evaluator failed",
				"engine", engine,
				"serverID", serverID,
				"error", err,
			)
			continue
		}
		for _, r := range results {
			fp := alerts.Fingerprint(serverID, engine, r.Category, r.RuleName)
			a := alerts.Alert{
				Fingerprint: fp,
				ServerID:    serverID,
				ServerName:  r.ServerName,
				Engine:      engine,
				Severity:    r.Severity,
				Status:      alerts.StatusOpen,
				Category:    r.Category,
				Title:       r.Title,
				Description: &r.Description,
				Evidence:    r.Evidence,
				FirstSeenAt: now,
				LastSeenAt:  now,
				HitCount:    1,
			}
			upserted, err := s.alertStore.Upsert(ctx, a)
			if err != nil {
				continue
			}
			count++

			// Notify — only on first occurrence (HitCount == 1) to avoid
			// flooding on every evaluation tick for a persistent condition.
			if s.notifier != nil && upserted.HitCount == 1 {
				s.notifier.Dispatch(ctx, upserted, "alert.opened")
			}
		}
	}
	return count, nil
}

// Acknowledge transitions an alert to acknowledged state.
func (s *AlertService) Acknowledge(ctx context.Context, id uuid.UUID, actor, reason string) error {
	a, err := s.alertStore.GetByID(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := a.Acknowledge(actor, now); err != nil {
		return err
	}
	return s.alertStore.UpdateStatus(ctx, id, alerts.StatusAcknowledged, actor, reason, now)
}

// Resolve transitions an alert to resolved state.
func (s *AlertService) Resolve(ctx context.Context, id uuid.UUID, actor, reason string) error {
	a, err := s.alertStore.GetByID(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := a.Resolve(actor, now); err != nil {
		return err
	}
	return s.alertStore.UpdateStatus(ctx, id, alerts.StatusResolved, actor, reason, now)
}

func (s *AlertService) List(ctx context.Context, f alerts.AlertFilter) ([]alerts.Alert, error) {
	return s.alertStore.List(ctx, f)
}

func (s *AlertService) GetByID(ctx context.Context, id uuid.UUID) (alerts.Alert, error) {
	return s.alertStore.GetByID(ctx, id)
}

func (s *AlertService) CountOpen(ctx context.Context, serverID uuid.UUID) (int, error) {
	return s.alertStore.CountOpen(ctx, serverID)
}

func (s *AlertService) PruneResolved(ctx context.Context, olderThan time.Duration) (int64, error) {
	return s.alertStore.PruneResolved(ctx, olderThan)
}
