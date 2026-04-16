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
	"time"

	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

// AlertEvaluatorResult is the output from a single evaluator check.
type AlertEvaluatorResult struct {
	RuleName     string
	Category     string
	Severity     alerts.Severity
	Title        string
	Description  string
	Evidence     map[string]interface{}
	InstanceName string
	Engine       alerts.Engine
}

// AlertEvaluator is the interface each engine-specific evaluator implements.
// Evaluate runs all checks for the given instance and returns zero or more results.
type AlertEvaluator interface {
	Evaluate(ctx context.Context, instanceName string) ([]AlertEvaluatorResult, error)
	Engine() alerts.Engine
}

// AlertService orchestrates alert evaluation, de-duplication, and lifecycle.
type AlertService struct {
	alertStore       alerts.AlertStore
	maintenanceStore alerts.MaintenanceStore
	evaluators       []AlertEvaluator
}

func NewAlertService(
	alertStore alerts.AlertStore,
	maintenanceStore alerts.MaintenanceStore,
	evaluators []AlertEvaluator,
) *AlertService {
	return &AlertService{
		alertStore:       alertStore,
		maintenanceStore: maintenanceStore,
		evaluators:       evaluators,
	}
}

// RunEvaluation executes all evaluators for a given instance and upserts alerts.
// Returns the number of new/bumped alerts.
func (s *AlertService) RunEvaluation(ctx context.Context, instanceName string, engine alerts.Engine) (int, error) {
	now := time.Now().UTC()

	// Check maintenance window
	underMaint, err := s.maintenanceStore.IsUnderMaintenance(ctx, instanceName, engine, now)
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
		results, err := ev.Evaluate(ctx, instanceName)
		if err != nil {
			// Log but continue with other evaluators
			continue
		}
		for _, r := range results {
			fp := alerts.Fingerprint(instanceName, engine, r.Category, r.RuleName)
			a := alerts.Alert{
				Fingerprint:  fp,
				InstanceName: instanceName,
				Engine:       engine,
				Severity:     r.Severity,
				Status:       alerts.StatusOpen,
				Category:     r.Category,
				Title:        r.Title,
				Description:  &r.Description,
				Evidence:     r.Evidence,
				FirstSeenAt:  now,
				LastSeenAt:   now,
				HitCount:     1,
			}
			if _, err := s.alertStore.Upsert(ctx, a); err != nil {
				continue
			}
			count++
		}
	}
	return count, nil
}

// Acknowledge transitions an alert to acknowledged state.
func (s *AlertService) Acknowledge(ctx context.Context, id string, actor, reason string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return err
	}
	a, err := s.alertStore.GetByID(ctx, uid)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := a.Acknowledge(actor, now); err != nil {
		return err
	}
	return s.alertStore.UpdateStatus(ctx, uid, alerts.StatusAcknowledged, actor, reason, now)
}

// Resolve transitions an alert to resolved state.
func (s *AlertService) Resolve(ctx context.Context, id string, actor, reason string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return err
	}
	a, err := s.alertStore.GetByID(ctx, uid)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := a.Resolve(actor, now); err != nil {
		return err
	}
	return s.alertStore.UpdateStatus(ctx, uid, alerts.StatusResolved, actor, reason, now)
}
