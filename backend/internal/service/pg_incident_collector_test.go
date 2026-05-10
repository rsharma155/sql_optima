// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TDD tests for PostgreSQL incident building logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"testing"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
)

func TestBuildIncidentRows_BlockingEvent(t *testing.T) {
	now := time.Now()
	locks := []repository.PGTimescaleLockInternal{
		{PID: 101, WaitDurationMs: 500, BlockedBy: 202, DatabaseName: "db1", QueryText: "UPDATE..."},
	}
	
	rows := buildIncidentRows("inst1", now, locks, nil, 0)
	
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].IncidentType != "blocking" {
		t.Fatalf("expected blocking type, got %s", rows[0].IncidentType)
	}
}

func TestBuildIncidentRows_LongQueryOver5Min_SetsCritical(t *testing.T) {
	now := time.Now()
	queries := []models.PgSession{
		{PID: 303, DurationMs: 300001, UserName: "u1", Database: "db1", Query: "SELECT..."},
	}
	
	rows := buildIncidentRows("inst1", now, nil, queries, 0)
	
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Severity != "critical" {
		t.Fatalf("expected critical, got %s", rows[0].Severity)
	}
}

func TestBuildIncidentRows_LongQueryUnder5Min_SetsWarning(t *testing.T) {
	now := time.Now()
	queries := []models.PgSession{
		{PID: 303, DurationMs: 10000, UserName: "u1", Database: "db1", Query: "SELECT..."},
	}
	
	rows := buildIncidentRows("inst1", now, nil, queries, 0)
	
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Severity != "warning" {
		t.Fatalf("expected warning, got %s", rows[0].Severity)
	}
}

func TestBuildIncidentRows_NoIncidents_ReturnsEmptySlice(t *testing.T) {
	rows := buildIncidentRows("inst1", time.Now(), nil, nil, 0)
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func TestBuildIncidentRows_DeadlockAlwaysCritical(t *testing.T) {
	now := time.Now()
	rows := buildIncidentRows("inst1", now, nil, nil, 5.0)
	
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].IncidentType != "deadlock" {
		t.Fatalf("expected deadlock type, got %s", rows[0].IncidentType)
	}
	if rows[0].Severity != "critical" {
		t.Fatalf("expected critical, got %s", rows[0].Severity)
	}
}
