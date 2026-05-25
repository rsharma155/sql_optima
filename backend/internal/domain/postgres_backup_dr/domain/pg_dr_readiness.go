// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Computes Backup & DR readiness pillars (recoverability, PITR, replication,
//          WAL slot safety) by comparing live and historical metrics to DR policy.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/domain/entities"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type ReadinessPillar struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"` // green, amber, red
	Summary     string `json:"summary"`
	TargetLabel string `json:"target_label"`
}

type ReadinessResult struct {
	Overall string            `json:"overall"`
	Pillars []ReadinessPillar `json:"pillars"`
	Policy  entities.DRPolicy `json:"policy"`
}

func worstStatus(a, b string) string {
	rank := map[string]int{"green": 0, "amber": 1, "red": 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func EvaluateReadiness(
	policy entities.DRPolicy,
	kpis map[string]interface{},
	slots []map[string]interface{},
	latestBackup *hot.PostgresBackupRunRow,
) ReadinessResult {
	pillars := make([]ReadinessPillar, 0, 4)
	overall := "green"

	// Recoverability
	recStatus, recSummary := evalRecoverability(policy, latestBackup)
	pillars = append(pillars, ReadinessPillar{
		ID: "recoverability", Title: "Can we restore?",
		Status: recStatus, Summary: recSummary,
		TargetLabel: fmt.Sprintf("Backup within %dh", policy.RPOBackupHours),
	})
	overall = worstStatus(overall, recStatus)

	// Point-in-time (archiving)
	arcStatus, arcSummary := evalArchiving(policy, kpis)
	pillars = append(pillars, ReadinessPillar{
		ID: "point_in_time", Title: "How far back can we rewind?",
		Status: arcStatus, Summary: arcSummary,
		TargetLabel: fmt.Sprintf("Archive age < %dm", policy.RPOArchiveMinutes),
	})
	overall = worstStatus(overall, arcStatus)

	// Availability (replication)
	availStatus, availSummary := evalReplication(policy, kpis)
	pillars = append(pillars, ReadinessPillar{
		ID: "availability", Title: "Are standbys current?",
		Status: availStatus, Summary: availSummary,
		TargetLabel: fmt.Sprintf("Replay lag < %ds", policy.RPOReplaySeconds),
	})
	overall = worstStatus(overall, availStatus)

	// WAL safety (slots)
	slotStatus, slotSummary := evalSlots(policy, slots, kpis)
	pillars = append(pillars, ReadinessPillar{
		ID: "wal_safety", Title: "Will WAL fill the disk?",
		Status: slotStatus, Summary: slotSummary,
		TargetLabel: fmt.Sprintf("Slot retention < %.0f GB", policy.MaxSlotRetentionGB),
	})
	overall = worstStatus(overall, slotStatus)

	return ReadinessResult{Overall: overall, Pillars: pillars, Policy: policy}
}

func evalRecoverability(policy entities.DRPolicy, latest *hot.PostgresBackupRunRow) (string, string) {
	if latest == nil {
		return "amber", "No backup run recorded — configure external backup reporting or log runs via API."
	}
	st := strings.ToLower(strings.TrimSpace(latest.Status))
	if st != "success" && st != "completed" && st != "succeeded" && st != "ok" {
		return "red", fmt.Sprintf("Last backup status is %q (%s).", latest.Status, latest.BackupType)
	}
	age := time.Since(latest.CaptureTimestamp)
	limit := time.Duration(policy.RPOBackupHours) * time.Hour
	if age > limit*2 {
		return "red", fmt.Sprintf("Last successful backup was %s ago (target %dh).", formatAge(age), policy.RPOBackupHours)
	}
	if age > limit {
		return "amber", fmt.Sprintf("Last successful backup was %s ago (target %dh).", formatAge(age), policy.RPOBackupHours)
	}
	return "green", fmt.Sprintf("Last %s backup %s ago (%s).", latest.BackupType, formatAge(age), latest.Tool)
}

func evalArchiving(policy entities.DRPolicy, kpis map[string]interface{}) (string, string) {
	arcPct, _ := kpis["archive_success_percent"].(float64)
	failed, _ := kpis["archive_failed_count"].(float64)
	if failed == 0 {
		if v, ok := kpis["failed_count"].(float64); ok {
			failed = v
		}
	}
	ageStr, _ := kpis["last_archive_age"].(string)
	status := "green"
	summary := "Archiving healthy."
	if arcPct < 99 || failed > 0 {
		status = "red"
		summary = fmt.Sprintf("Archive success %.1f%% with %v failures in range.", arcPct, int(failed))
	} else if arcPct < 100 {
		status = "amber"
		summary = fmt.Sprintf("Archive success %.1f%%.", arcPct)
	}
	if ageStr != "" && ageStr != "N/A" {
		if d, err := time.ParseDuration(strings.ReplaceAll(ageStr, " ", "")); err == nil {
			limit := time.Duration(policy.RPOArchiveMinutes) * time.Minute
			if d > limit*2 {
				status = worstStatus(status, "red")
				summary = fmt.Sprintf("Last archive %s ago (target %dm). %s", formatAge(d), policy.RPOArchiveMinutes, summary)
			} else if d > limit {
				status = worstStatus(status, "amber")
				summary = fmt.Sprintf("Last archive %s ago (target %dm).", formatAge(d), policy.RPOArchiveMinutes)
			}
		} else if strings.Contains(ageStr, "day") || strings.Contains(ageStr, "hour") {
			// Postgres interval text e.g. "00:15:30" or "1 day"
			if strings.Contains(ageStr, "day") || (strings.Contains(ageStr, "hour") && policy.RPOArchiveMinutes < 60) {
				status = worstStatus(status, "amber")
				summary = fmt.Sprintf("Last archive age %s.", ageStr)
			}
		}
	}
	return status, summary
}

func evalReplication(policy entities.DRPolicy, kpis map[string]interface{}) (string, string) {
	maxLag, _ := kpis["replica_max_lag_seconds"].(float64)
	role, _ := kpis["node_role"].(string)
	if role == "standalone" && maxLag == 0 {
		return "green", "Standalone instance — no streaming replicas."
	}
	if role == "replica" {
		return "green", "Viewing a replica — check primary for standby lag."
	}
	if maxLag == 0 {
		return "green", "No replica lag detected in selected window."
	}
	if maxLag > float64(policy.RPOReplaySeconds)*2 {
		return "red", fmt.Sprintf("Max replay lag %.0fs (target %ds).", maxLag, policy.RPOReplaySeconds)
	}
	if maxLag > float64(policy.RPOReplaySeconds) {
		return "amber", fmt.Sprintf("Max replay lag %.0fs (target %ds).", maxLag, policy.RPOReplaySeconds)
	}
	return "green", fmt.Sprintf("Max replay lag %.0fs within target.", maxLag)
}

func evalSlots(policy entities.DRPolicy, slots []map[string]interface{}, kpis map[string]interface{}) (string, string) {
	riskGB, _ := kpis["replication_slots_risk_gb"].(float64)
	inactive := 0
	for _, s := range slots {
		active, _ := s["active"].(bool)
		if !active {
			inactive++
		}
	}
	if len(slots) == 0 && riskGB == 0 {
		return "green", "No replication slots or within limits."
	}
	if riskGB > policy.MaxSlotRetentionGB*2 || inactive > 0 && riskGB > policy.MaxSlotRetentionGB {
		msg := fmt.Sprintf("%.2f GB WAL retained by slots", riskGB)
		if inactive > 0 {
			msg += fmt.Sprintf(" (%d inactive slot(s))", inactive)
		}
		return "red", msg + "."
	}
	if riskGB > policy.MaxSlotRetentionGB {
		return "amber", fmt.Sprintf("%.2f GB WAL retained (target %.0f GB).", riskGB, policy.MaxSlotRetentionGB)
	}
	if inactive > 0 {
		return "amber", fmt.Sprintf("%d inactive replication slot(s) — may retain WAL.", inactive)
	}
	return "green", fmt.Sprintf("Slot retention %.2f GB within target.", riskGB)
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}
