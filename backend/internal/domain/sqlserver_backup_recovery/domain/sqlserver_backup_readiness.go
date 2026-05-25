// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: DR readiness chip computation for SQL Server Backup & Recovery.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package domain

import (
	"fmt"
	"strings"
)

// ComputeReadiness builds readiness chips from posture, policy, and optional job/log-shipping signals.
func ComputeReadiness(
	posture []DatabasePosture,
	policy BackupPolicy,
	failedBackupJobs24h int,
	logShippingBehind int,
	logShippingEnabled bool,
) ReadinessSummary {
	chips := make([]ReadinessChip, 0, 6)
	staleFull := 0
	overdueLog := 0
	for _, p := range posture {
		if !p.FullFreshOK {
			staleFull++
		}
		rm := strings.ToUpper(p.RecoveryModel)
		if (rm == "FULL" || rm == "BULK_LOGGED") && !p.LogFreshOK {
			overdueLog++
		}
	}
	_ = policy

	switch {
	case len(posture) == 0:
		chips = append(chips, ReadinessChip{Label: "Collecting backup posture", Class: "warn"})
	case staleFull == 0:
		chips = append(chips, ReadinessChip{Label: "Full backups within policy", Class: "ok"})
	default:
		chips = append(chips, ReadinessChip{Label: fmt.Sprintf("%d DB(s) overdue full backup", staleFull), Class: "bad"})
	}

	if len(posture) > 0 {
		if overdueLog == 0 {
			chips = append(chips, ReadinessChip{Label: "Log backups within policy", Class: "ok"})
		} else {
			chips = append(chips, ReadinessChip{Label: fmt.Sprintf("%d DB(s) overdue log backup", overdueLog), Class: "bad"})
		}
	}

	if failedBackupJobs24h > 0 {
		chips = append(chips, ReadinessChip{
			Label: fmt.Sprintf("%d failed backup/maintenance job(s) (24h)", failedBackupJobs24h),
			Class: "bad",
		})
	} else if len(posture) > 0 {
		chips = append(chips, ReadinessChip{Label: "No failed backup jobs (24h)", Class: "ok"})
	}

	if logShippingEnabled {
		if logShippingBehind > 0 {
			chips = append(chips, ReadinessChip{
				Label: fmt.Sprintf("%d log shipping pair(s) behind", logShippingBehind),
				Class: "bad",
			})
		} else {
			chips = append(chips, ReadinessChip{Label: "Log shipping within threshold", Class: "ok"})
		}
	}

	overall := "acceptable"
	overallClass := "ok"
	if len(posture) == 0 {
		overall = "collecting"
		overallClass = "warn"
	} else if staleFull > 0 || overdueLog > 0 || failedBackupJobs24h > 0 || logShippingBehind > 0 {
		overall = "needs_attention"
		overallClass = "bad"
	}

	return ReadinessSummary{Overall: overall + ":" + overallClass, Chips: chips}
}

// ApplyPolicyFreshness recomputes full_fresh_ok and log_fresh_ok from ages and policy thresholds.
func ApplyPolicyFreshness(posture []DatabasePosture, policy BackupPolicy) {
	fullMins := policy.RPOFullBackupHours * 60
	logMins := policy.RPOLogBackupMinutes
	for i := range posture {
		p := &posture[i]
		if !p.HasFullBackup {
			p.FullFreshOK = false
		} else {
			p.FullFreshOK = p.MinutesSinceFull <= fullMins
		}
		rm := strings.ToUpper(p.RecoveryModel)
		if rm == "SIMPLE" {
			p.LogFreshOK = true
		} else if !p.HasFullBackup && p.LastLogFinish == nil {
			p.LogFreshOK = false
		} else {
			p.LogFreshOK = p.MinutesSinceLog <= logMins
		}
	}
}
