// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Port interfaces (AlertStore, MaintenanceStore, AlertRuleStore) and
//
//	supporting filter/rule types for the alert engine domain.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package alerts

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AlertFilter holds optional filters for listing alerts.
type AlertFilter struct {
	InstanceName string
	Engine       Engine
	Severity     Severity
	Status       Status
	Category     string
	Limit        int
	Offset       int
}

// AlertStore is the port for persisting and querying alert records.
type AlertStore interface {
	// Upsert creates a new alert or bumps the hit count of an existing open
	// alert with the same fingerprint.
	Upsert(ctx context.Context, a Alert) (Alert, error)

	// GetByID fetches a single alert.
	GetByID(ctx context.Context, id uuid.UUID) (Alert, error)

	// List returns alerts matching the given filter.
	List(ctx context.Context, f AlertFilter) ([]Alert, int, error)

	// UpdateStatus persists a status transition and writes a history row.
	UpdateStatus(ctx context.Context, id uuid.UUID, status Status, actor, reason string, at time.Time) error

	// CountOpen returns the number of non-resolved alerts per instance.
	CountOpen(ctx context.Context, instanceName string) (int, error)
}

// MaintenanceStore is the port for maintenance window persistence.
type MaintenanceStore interface {
	Create(ctx context.Context, mw MaintenanceWindow) (MaintenanceWindow, error)
	IsUnderMaintenance(ctx context.Context, instanceName string, engine Engine, at time.Time) (bool, error)
	ListActive(ctx context.Context, at time.Time) ([]MaintenanceWindow, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// AlertRuleStore is the port for alert rule configuration persistence.
type AlertRuleStore interface {
	ListEnabled(ctx context.Context, engine Engine) ([]AlertRule, error)
	GetByName(ctx context.Context, name string) (AlertRule, error)
}

// AlertRule describes a configured alert evaluation rule.
type AlertRule struct {
	ID              uuid.UUID              `json:"id"`
	Name            string                 `json:"name"`
	Engine          string                 `json:"engine"`
	Category        string                 `json:"category"`
	DefaultSeverity Severity               `json:"default_severity"`
	Description     string                 `json:"description"`
	IsEnabled       bool                   `json:"is_enabled"`
	Config          map[string]interface{} `json:"config"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}
