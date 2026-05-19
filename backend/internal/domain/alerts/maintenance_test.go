// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for MaintenanceWindow validation and active-check logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package alerts

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMaintenanceWindow_IsActive(t *testing.T) {
	mw := MaintenanceWindow{
		StartTime: time.Date(2026, 4, 16, 2, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 16, 4, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"before window", time.Date(2026, 4, 16, 1, 0, 0, 0, time.UTC), false},
		{"at start", time.Date(2026, 4, 16, 2, 0, 0, 0, time.UTC), true},
		{"during window", time.Date(2026, 4, 16, 3, 0, 0, 0, time.UTC), true},
		{"at end (exclusive)", time.Date(2026, 4, 16, 4, 0, 0, 0, time.UTC), false},
		{"after window", time.Date(2026, 4, 16, 5, 0, 0, 0, time.UTC), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mw.IsActive(tt.now); got != tt.want {
				t.Errorf("IsActive(%v) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

func TestMaintenanceWindow_Validate(t *testing.T) {
	serverID := uuid.New()
	base := MaintenanceWindow{
		ServerID:  serverID,
		Engine:    EnginePostgres,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(2 * time.Hour),
	}

	t.Run("valid", func(t *testing.T) {
		mw := base
		if err := mw.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing instance", func(t *testing.T) {
		mw := base
		mw.ServerID = uuid.Nil
		if err := mw.Validate(); err != ErrMissingInstanceName {
			t.Errorf("expected ErrMissingInstanceName, got %v", err)
		}
	})

	t.Run("invalid engine", func(t *testing.T) {
		mw := base
		mw.Engine = Engine("mysql")
		if err := mw.Validate(); err != ErrInvalidEngine {
			t.Errorf("expected ErrInvalidEngine, got %v", err)
		}
	})

	t.Run("ends before starts", func(t *testing.T) {
		mw := base
		mw.EndTime = mw.StartTime.Add(-1 * time.Hour)
		if err := mw.Validate(); err == nil {
			t.Error("expected error for invalid range")
		}
	})
}
