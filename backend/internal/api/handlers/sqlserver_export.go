// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: CSV export handlers for SQL Server Best Practices & Guardrails dashboard.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ExportBestPracticesCSV streams a full "Best Practices & Guardrails" report as a CSV.
// It mirrors exactly what the dashboard page renders: server config checks, database config
// checks, and all eight guardrails categories.
func (h *SqlServerHandlers) ExportBestPracticesCSV(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	bp := h.metricsSvc.GetBestPractices(instance)
	gr := h.metricsSvc.GetGuardrails(instance)

	cw := newCSVResponse(w, "sqlserver_best_practices_guardrails", instance)

	// ── PART 1: Server Configuration ─────────────────────────────────────────
	cw.WriteSection("Server Configuration", []string{
		"Category", "Configuration Name", "Current Value", "Status", "Recommendation", "Remediation SQL",
	})
	for _, c := range bp.ServerConfig {
		msg := c.Message
		if msg == "" {
			msg = "Setting is within best practice guidelines."
		}
		cw.WriteRow([]string{
			c.Category,
			c.ConfigurationName,
			c.CurrentValue,
			c.Status,
			msg,
			c.RemediationSQL,
		})
	}
	cw.WriteRow([]string{})

	// ── PART 2: Database Configuration ───────────────────────────────────────
	cw.WriteSection("Database Configuration", []string{
		"Database Name", "Page Verify", "Auto Shrink", "Auto Close",
		"Target Recovery Time (s)", "Status", "Recommendation",
	})
	for _, db := range bp.DatabaseConfig {
		autoShrink := "OFF"
		if db.AutoShrink {
			autoShrink = "ON"
		}
		autoClose := "OFF"
		if db.AutoClose {
			autoClose = "ON"
		}
		msg := db.Message
		if msg == "" {
			msg = "All database settings are within best practice guidelines."
		}
		cw.WriteRow([]string{
			db.DatabaseName,
			db.PageVerify,
			autoShrink,
			autoClose,
			fmt.Sprintf("%d", db.TargetRecoveryTime),
			db.Status,
			msg,
		})
	}
	cw.WriteRow([]string{})

	// ── PART 3: Guardrails ────────────────────────────────────────────────────
	cw.WriteRow([]string{
		fmt.Sprintf("Guardrails Health Score: %d/100", gr.HealthScore),
		fmt.Sprintf("Health Status: %s", gr.HealthStatus),
	})
	cw.WriteRow([]string{})

	guardrailHeaders := []string{"Category", "Severity", "Item", "Detail", "Value", "Message", "Remediation SQL"}

	// Storage Risks
	if len(gr.StorageRisks) > 0 {
		cw.WriteSection("Storage Risks", guardrailHeaders)
		for _, s := range gr.StorageRisks {
			cw.WriteRow([]string{
				"Storage Risks", s.Severity,
				s.DatabaseName + " / " + s.LogicalName,
				s.PhysicalName,
				fmt.Sprintf("%d MB", s.SizeMB),
				s.Message, s.MitigationSQL,
			})
		}
		cw.WriteRow([]string{})
	}

	// Disk Space
	if len(gr.DiskSpace) > 0 {
		cw.WriteSection("Disk Space", guardrailHeaders)
		for _, d := range gr.DiskSpace {
			cw.WriteRow([]string{
				"Disk Space", d.Severity,
				"Drive " + d.DriveLetter + ":",
				fmt.Sprintf("Free: %d MB / Total: %d MB", d.FreeSpaceMB, d.TotalSizeMB),
				fmt.Sprintf("%.1f%% free", d.FreePercent),
				d.Message, d.MitigationSQL,
			})
		}
		cw.WriteRow([]string{})
	}

	// Transaction Log Health
	if len(gr.LogHealth) > 0 {
		cw.WriteSection("Transaction Log Health", guardrailHeaders)
		for _, l := range gr.LogHealth {
			cw.WriteRow([]string{
				"Transaction Log Health", l.Severity,
				l.DatabaseName,
				fmt.Sprintf("Recovery: %s | Reuse Wait: %s | VLFs: %d", l.RecoveryModel, l.LogReuseWait, l.VLFCount),
				fmt.Sprintf("%.1f MB (%.1f%% used)", l.LogSizeMB, l.LogSpaceUsedPct),
				l.Message, l.MitigationSQL,
			})
		}
		cw.WriteRow([]string{})
	}

	// Log Backups
	if len(gr.LogBackups) > 0 {
		cw.WriteSection("Log Backups", guardrailHeaders)
		for _, lb := range gr.LogBackups {
			cw.WriteRow([]string{
				"Log Backups", lb.Severity,
				lb.DatabaseName,
				fmt.Sprintf("Last backup: %s", lb.LastBackup),
				fmt.Sprintf("%d minutes ago", lb.MinutesAgo),
				lb.Message, lb.MitigationSQL,
			})
		}
		cw.WriteRow([]string{})
	}

	// Long Running Transactions
	if len(gr.LongTxns) > 0 {
		cw.WriteSection("Long Running Transactions", guardrailHeaders)
		for _, t := range gr.LongTxns {
			orphaned := "No"
			if t.IsOrphaned {
				orphaned = "Yes"
			}
			cw.WriteRow([]string{
				"Long Running Transactions", t.Severity,
				fmt.Sprintf("SPID %d / %s", t.SessionID, t.LoginName),
				fmt.Sprintf("DB: %s | Status: %s | Orphaned: %s | Blocking: %d", t.DatabaseName, t.Status, orphaned, t.BlockingSessionID),
				fmt.Sprintf("%d seconds elapsed", t.ElapsedSeconds),
				t.Message, t.MitigationSQL,
			})
		}
		cw.WriteRow([]string{})
	}

	// Autogrowth Risks
	if len(gr.Autogrowth) > 0 {
		cw.WriteSection("Autogrowth Risks", guardrailHeaders)
		for _, ag := range gr.Autogrowth {
			growthType := "Fixed size"
			if ag.IsPercentGrowth {
				growthType = "Percent"
			}
			cw.WriteRow([]string{
				"Autogrowth Risks", ag.Severity,
				ag.DatabaseName + " / " + ag.LogicalName,
				fmt.Sprintf("File: %s | Growth type: %s", ag.FileType, growthType),
				fmt.Sprintf("Growth: %d", ag.Growth),
				ag.Message, ag.MitigationSQL,
			})
		}
		cw.WriteRow([]string{})
	}

	// TempDB
	cw.WriteSection("TempDB Configuration", guardrailHeaders)
	cw.WriteRow([]string{
		"TempDB Configuration", gr.TempDBConfig.Severity,
		"TempDB",
		fmt.Sprintf("File count: %d | Total: %d MB", gr.TempDBConfig.FileCount, gr.TempDBConfig.TotalSizeMB),
		"",
		gr.TempDBConfig.Message, gr.TempDBConfig.MitigationSQL,
	})
	cw.WriteRow([]string{})

	// Resource Governor
	cw.WriteSection("Resource Governor", guardrailHeaders)
	rgState := "Disabled"
	if gr.ResourceGov.IsEnabled {
		rgState = "Enabled"
	}
	cw.WriteRow([]string{
		"Resource Governor", gr.ResourceGov.Severity,
		"Resource Governor",
		rgState, "",
		gr.ResourceGov.Message, gr.ResourceGov.MitigationSQL,
	})

	cw.Flush()
}

// ExportGuardrailsCSV streams only the guardrails health assessment as a standalone CSV.
func (h *SqlServerHandlers) ExportGuardrailsCSV(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	data := h.metricsSvc.GetGuardrails(instance)
	cw := newCSVResponse(w, "sqlserver_guardrails", instance)

	cw.WriteRow([]string{
		fmt.Sprintf("Overall Health Score: %d/100", data.HealthScore),
		fmt.Sprintf("Health Status: %s", data.HealthStatus),
	})
	cw.WriteRow([]string{})

	headers := []string{"Category", "Severity", "Item", "Detail", "Value", "Message", "Remediation SQL"}

	cw.WriteSection("Storage Risks", headers)
	for _, s := range data.StorageRisks {
		cw.WriteRow([]string{"Storage Risks", s.Severity, s.DatabaseName + " / " + s.LogicalName, s.PhysicalName, fmt.Sprintf("%d MB", s.SizeMB), s.Message, s.MitigationSQL})
	}
	cw.WriteRow([]string{})

	cw.WriteSection("Disk Space", headers)
	for _, d := range data.DiskSpace {
		cw.WriteRow([]string{"Disk Space", d.Severity, "Drive " + d.DriveLetter + ":", fmt.Sprintf("Free: %d MB / Total: %d MB", d.FreeSpaceMB, d.TotalSizeMB), fmt.Sprintf("%.1f%% free", d.FreePercent), d.Message, d.MitigationSQL})
	}
	cw.WriteRow([]string{})

	cw.WriteSection("Transaction Log Health", headers)
	for _, l := range data.LogHealth {
		cw.WriteRow([]string{"Transaction Log Health", l.Severity, l.DatabaseName, fmt.Sprintf("Recovery: %s | Reuse Wait: %s | VLFs: %d", l.RecoveryModel, l.LogReuseWait, l.VLFCount), fmt.Sprintf("%.1f MB (%.1f%% used)", l.LogSizeMB, l.LogSpaceUsedPct), l.Message, l.MitigationSQL})
	}
	cw.WriteRow([]string{})

	cw.WriteSection("Log Backups", headers)
	for _, lb := range data.LogBackups {
		cw.WriteRow([]string{"Log Backups", lb.Severity, lb.DatabaseName, fmt.Sprintf("Last backup: %s", lb.LastBackup), fmt.Sprintf("%d minutes ago", lb.MinutesAgo), lb.Message, lb.MitigationSQL})
	}
	cw.WriteRow([]string{})

	cw.WriteSection("Long Running Transactions", headers)
	for _, t := range data.LongTxns {
		orphaned := "No"
		if t.IsOrphaned {
			orphaned = "Yes"
		}
		cw.WriteRow([]string{"Long Running Transactions", t.Severity, fmt.Sprintf("SPID %d / %s", t.SessionID, t.LoginName), fmt.Sprintf("DB: %s | Orphaned: %s", t.DatabaseName, orphaned), fmt.Sprintf("%d seconds elapsed", t.ElapsedSeconds), t.Message, t.MitigationSQL})
	}
	cw.WriteRow([]string{})

	cw.WriteSection("Autogrowth Risks", headers)
	for _, ag := range data.Autogrowth {
		growthType := "Fixed size"
		if ag.IsPercentGrowth {
			growthType = "Percent"
		}
		cw.WriteRow([]string{"Autogrowth Risks", ag.Severity, ag.DatabaseName + " / " + ag.LogicalName, fmt.Sprintf("File: %s | Growth type: %s", ag.FileType, growthType), fmt.Sprintf("Growth: %d", ag.Growth), ag.Message, ag.MitigationSQL})
	}
	cw.WriteRow([]string{})

	cw.WriteSection("TempDB Configuration", headers)
	cw.WriteRow([]string{"TempDB Configuration", data.TempDBConfig.Severity, "TempDB", fmt.Sprintf("File count: %d | Total: %d MB", data.TempDBConfig.FileCount, data.TempDBConfig.TotalSizeMB), "", data.TempDBConfig.Message, data.TempDBConfig.MitigationSQL})
	cw.WriteRow([]string{})

	cw.WriteSection("Resource Governor", headers)
	rgState := "Disabled"
	if data.ResourceGov.IsEnabled {
		rgState = "Enabled"
	}
	cw.WriteRow([]string{"Resource Governor", data.ResourceGov.Severity, "Resource Governor", rgState, "", data.ResourceGov.Message, data.ResourceGov.MitigationSQL})

	cw.Flush()
}
