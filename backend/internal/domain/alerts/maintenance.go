// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: MaintenanceWindow entity with validation and active-check logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package alerts

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// MaintenanceWindow represents a scheduled period during which alerts
// for a given instance are suppressed.
type MaintenanceWindow struct {
	ID           uuid.UUID `json:"id"`
	InstanceName string    `json:"instance_name"`
	Engine       Engine    `json:"engine"`
	Reason       string    `json:"reason,omitempty"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	CreatedBy    string    `json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// IsActive returns true when the current time falls within the window.
func (mw *MaintenanceWindow) IsActive(now time.Time) bool {
	return !now.Before(mw.StartsAt) && now.Before(mw.EndsAt)
}

// Validate checks required fields and range constraint.
func (mw *MaintenanceWindow) Validate() error {
	if mw.InstanceName == "" {
		return ErrMissingInstanceName
	}
	if !mw.Engine.Valid() {
		return ErrInvalidEngine
	}
	if !mw.EndsAt.After(mw.StartsAt) {
		return errors.New("ends_at must be after starts_at")
	}
	return nil
}
