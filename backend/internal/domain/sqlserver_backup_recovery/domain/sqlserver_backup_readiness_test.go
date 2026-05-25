// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for SQL Server backup readiness computation.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestApplyPolicyFreshness_FullAndLog(t *testing.T) {
	policy := DefaultBackupPolicy(uuid.Nil)
	policy.RPOFullBackupHours = 24
	policy.RPOLogBackupMinutes = 15

	posture := []DatabasePosture{
		{DatabaseName: "db1", RecoveryModel: "FULL", HasFullBackup: true, MinutesSinceFull: 60, MinutesSinceLog: 10},
		{DatabaseName: "db2", RecoveryModel: "FULL", HasFullBackup: true, MinutesSinceFull: 2000, MinutesSinceLog: 20},
		{DatabaseName: "db3", RecoveryModel: "SIMPLE", MinutesSinceLog: 9999},
	}
	ApplyPolicyFreshness(posture, policy)

	if !posture[0].FullFreshOK || !posture[0].LogFreshOK {
		t.Fatalf("db1 expected fresh: %+v", posture[0])
	}
	if posture[1].FullFreshOK || posture[1].LogFreshOK {
		t.Fatalf("db2 expected stale: %+v", posture[1])
	}
	if !posture[2].LogFreshOK {
		t.Fatalf("SIMPLE recovery should not require log backup: %+v", posture[2])
	}
}

func TestComputeReadiness_NeedsAttention(t *testing.T) {
	posture := []DatabasePosture{
		{DatabaseName: "a", RecoveryModel: "FULL", FullFreshOK: false, LogFreshOK: true},
	}
	sum := ComputeReadiness(posture, DefaultBackupPolicy(uuid.Nil), 0, 0, false)
	if sum.Overall != "needs_attention:bad" {
		t.Fatalf("overall=%q", sum.Overall)
	}
	if len(sum.Chips) < 2 {
		t.Fatalf("expected chips, got %d", len(sum.Chips))
	}
}

func TestComputeReadiness_Collecting(t *testing.T) {
	sum := ComputeReadiness(nil, DefaultBackupPolicy(uuid.Nil), 0, 0, false)
	if sum.Overall != "collecting:warn" {
		t.Fatalf("overall=%q", sum.Overall)
	}
}
