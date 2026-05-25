// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Domain model and default values for per-instance PostgreSQL DR policy
//          (RPO backup/archive/replay and slot retention targets).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package entities

import (
	"time"

	"github.com/google/uuid"
)

type DRPolicy struct {
	ServerID            uuid.UUID `json:"server_id"`
	RPOBackupHours      int       `json:"rpo_backup_hours"`
	RPOArchiveMinutes   int       `json:"rpo_archive_minutes"`
	RPOReplaySeconds    int       `json:"rpo_replay_seconds"`
	MaxSlotRetentionGB  float64   `json:"max_slot_retention_gb"`
	RTOFailoverMinutes  *int      `json:"rto_failover_minutes,omitempty"`
	UpdatedAt           time.Time `json:"updated_at,omitempty"`
	UpdatedBy           string    `json:"updated_by,omitempty"`
}

func DefaultDRPolicy(serverID uuid.UUID) DRPolicy {
	return DRPolicy{
		ServerID:           serverID,
		RPOBackupHours:     24,
		RPOArchiveMinutes:  5,
		RPOReplaySeconds:   60,
		MaxSlotRetentionGB: 10,
	}
}
