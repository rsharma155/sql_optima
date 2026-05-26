// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Define repository ports for alert storage and maintenance windows.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package alerts

import (
	"context"
	"github.com/google/uuid"
	"time"
)

// AlertStore defines the persistence interface for alerts.
type AlertStore interface {
	// Upsert inserts a new alert or updates an existing one (bumping HitCount).
	Upsert(ctx context.Context, a Alert) (Alert, error)

	// GetByID retrieves a single alert by its UUID.
	GetByID(ctx context.Context, id uuid.UUID) (Alert, error)

	// List retrieves alerts filtered by instance, engine, status, or severity.
	List(ctx context.Context, filter AlertFilter) ([]Alert, error)

	// UpdateStatus updates the status of an alert (Acknowledge/Resolve).
	UpdateStatus(ctx context.Context, id uuid.UUID, status Status, actor string, reason string, at time.Time) error

	// PruneResolved removes alerts that have been resolved for more than the given duration.
	PruneResolved(ctx context.Context, olderThan time.Duration) (int64, error)

	// CountOpen returns the number of active alerts for a given server.
	CountOpen(ctx context.Context, serverID uuid.UUID) (int, error)

	// ResolveByFingerprint resolves an open/acknowledged alert with the given fingerprint.
	ResolveByFingerprint(ctx context.Context, fingerprint, actor, reason string, at time.Time) (bool, error)
}

// AlertFilter defines criteria for listing alerts.
type AlertFilter struct {
	ServerID *uuid.UUID `json:"server_id,omitempty"`
	Engine   *Engine    `json:"engine,omitempty"`
	Status   *Status    `json:"status,omitempty"`
	Severity *Severity  `json:"severity,omitempty"`
	Category *string    `json:"category,omitempty"`
	Limit    int        `json:"limit,omitempty"`
}

// MaintenanceStore defines the persistence interface for maintenance windows.
type MaintenanceStore interface {
	// Create registers a new maintenance window.
	Create(ctx context.Context, mw MaintenanceWindow) (MaintenanceWindow, error)

	// IsUnderMaintenance checks if a specific instance/engine is currently suppressed.
	IsUnderMaintenance(ctx context.Context, serverID uuid.UUID, engine Engine, at time.Time) (bool, error)

	// ListActive retrieves all maintenance windows covering the current time.
	ListActive(ctx context.Context, at time.Time) ([]MaintenanceWindow, error)

	// Delete removes a maintenance window.
	Delete(ctx context.Context, id uuid.UUID) error
}
