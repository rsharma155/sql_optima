// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Core alert aggregate – value objects (Severity, Status, Engine) and
//
//	Alert entity with lifecycle methods (Acknowledge, Resolve, BumpHitCount).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package alerts

import (
	"time"

	"github.com/google/uuid"
)

// Severity represents the importance level of an alert.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

func (s Severity) Valid() bool {
	switch s {
	case SeverityInfo, SeverityWarning, SeverityCritical:
		return true
	}
	return false
}

// Weight returns a numeric weight for sorting (higher = more severe).
func (s Severity) Weight() int {
	switch s {
	case SeverityCritical:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	}
	return 0
}

// Status represents the lifecycle state of an alert.
type Status string

const (
	StatusOpen         Status = "open"
	StatusAcknowledged Status = "acknowledged"
	StatusResolved     Status = "resolved"
)

func (s Status) Valid() bool {
	switch s {
	case StatusOpen, StatusAcknowledged, StatusResolved:
		return true
	}
	return false
}

// Engine identifies the database engine that originated an alert.
type Engine string

const (
	EnginePostgres  Engine = "postgres"
	EngineSQLServer Engine = "sqlserver"
)

func (e Engine) Valid() bool {
	switch e {
	case EnginePostgres, EngineSQLServer:
		return true
	}
	return false
}

// Alert is the core domain entity representing a detected issue.
type Alert struct {
	ID             uuid.UUID              `json:"id"`
	Fingerprint    string                 `json:"fingerprint"`
	ServerID       *uuid.UUID             `json:"server_id,omitempty"`
	InstanceName   string                 `json:"instance_name"`
	Engine         Engine                 `json:"engine"`
	Severity       Severity               `json:"severity"`
	Status         Status                 `json:"status"`
	Category       string                 `json:"category"`
	Title          string                 `json:"title"`
	Description    *string                `json:"description,omitempty"`
	Evidence       map[string]interface{} `json:"evidence,omitempty"`
	FirstSeenAt    time.Time              `json:"first_seen_at"`
	LastSeenAt     time.Time              `json:"last_seen_at"`
	HitCount       int                    `json:"hit_count"`
	AcknowledgedBy *string                `json:"acknowledged_by,omitempty"`
	AcknowledgedAt *time.Time             `json:"acknowledged_at,omitempty"`
	ResolvedBy     *string                `json:"resolved_by,omitempty"`
	ResolvedAt     *time.Time             `json:"resolved_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// Acknowledge transitions the alert to acknowledged state.
func (a *Alert) Acknowledge(actor string, at time.Time) error {
	if a.Status == StatusResolved {
		return ErrAlertAlreadyResolved
	}
	a.Status = StatusAcknowledged
	a.AcknowledgedBy = &actor
	a.AcknowledgedAt = &at
	a.UpdatedAt = at
	return nil
}

// Resolve transitions the alert to resolved state.
func (a *Alert) Resolve(actor string, at time.Time) error {
	if a.Status == StatusResolved {
		return ErrAlertAlreadyResolved
	}
	a.Status = StatusResolved
	a.ResolvedBy = &actor
	a.ResolvedAt = &at
	a.UpdatedAt = at
	return nil
}

// BumpHitCount increments the de-duplication counter and updates last-seen.
func (a *Alert) BumpHitCount(at time.Time) {
	a.HitCount++
	a.LastSeenAt = at
	a.UpdatedAt = at
}

// IsOpen returns true when the alert still requires attention.
func (a *Alert) IsOpen() bool {
	return a.Status == StatusOpen || a.Status == StatusAcknowledged
}
