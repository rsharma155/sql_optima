// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Built-in alert rule identifiers used for auto-resolve when conditions clear.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import "github.com/rsharma155/sql_optima/internal/domain/alerts"

type alertRuleKey struct {
	Category string
	RuleName string
}

func builtinRulesForEngine(engine alerts.Engine) []alertRuleKey {
	switch engine {
	case alerts.EngineSQLServer:
		return []alertRuleKey{
			{"Performance", "MSBlockingIncident"},
			{"Job Agent", "MSJobFailed"},
			{"Storage", "MSDiskSpaceLow"},
		}
	case alerts.EnginePostgres:
		return []alertRuleKey{
			{"Performance", "PGBlockingIncident"},
			{"Performance", "PGIdleInTransaction"},
			{"Replication", "PGReplicationLagHigh"},
			{"Backup", "PGBackupNeverRun"},
			{"Backup", "PGBackupStale"},
			{"Storage", "PGDiskSpaceLow"},
		}
	default:
		return nil
	}
}
