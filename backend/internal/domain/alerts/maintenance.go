// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Define maintenance windows to suppress alerting during
//
//	known patching or deployment activities.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package alerts

import (
	"errors"
	"github.com/google/uuid"
	"time"
)

// MaintenanceWindow defines a period where alerts for a specific instance
// and engine should be suppressed or downgraded.
type MaintenanceWindow struct {
	ID          uuid.UUID `json:"id"`
	ServerID    uuid.UUID `json:"server_id"`
	Engine      Engine    `json:"engine"`
	Category    *string   `json:"category,omitempty"` // optional: only suppress specific category
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Description string    `json:"description"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// IsActive returns true if the given time falls within the window.
func (mw *MaintenanceWindow) IsActive(t time.Time) bool {
	return (t.Equal(mw.StartTime) || t.After(mw.StartTime)) && t.Before(mw.EndTime)
}

// Validate ensures the maintenance window has required fields and a valid range.
func (mw *MaintenanceWindow) Validate() error {
	if mw.ServerID == uuid.Nil {
		return ErrMissingInstanceName // Keeping name for compatibility or could change to ErrMissingServerID
	}
	if !mw.Engine.Valid() {
		return ErrInvalidEngine
	}
	if !mw.EndTime.After(mw.StartTime) {
		return errors.New("end time must be after start time")
	}
	return nil
}
