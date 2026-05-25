// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
//
// Purpose: Shared CSV response writers for PostgreSQL and SQL Server best-practices export handlers.

package handlers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"

	"github.com/rsharma155/sql_optima/internal/models"
)

func writeBestPracticesCSV(w http.ResponseWriter, filename string, result models.BestPracticesResult) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"scope", "name", "category", "current_value", "default_value", "status", "message", "remediation_sql"})
	for _, c := range result.ServerConfig {
		_ = cw.Write([]string{
			"server",
			c.ConfigurationName,
			c.Category,
			c.CurrentValue,
			c.DefaultValue,
			c.Status,
			c.Message,
			c.RemediationSQL,
		})
	}
	for _, c := range result.DatabaseConfig {
		_ = cw.Write([]string{
			"database",
			c.DatabaseName,
			"",
			fmt.Sprintf("page_verify=%s auto_shrink=%v auto_close=%v target_recovery=%d",
				c.PageVerify, c.AutoShrink, c.AutoClose, c.TargetRecoveryTime),
			"",
			c.Status,
			c.Message,
			"",
		})
	}
	cw.Flush()
}

func writeGuardrailsCSV(w http.ResponseWriter, filename string, result models.GuardrailsResult) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"section", "severity", "database", "detail", "message"})
	for _, r := range result.StorageRisks {
		_ = cw.Write([]string{"storage_risk", r.Severity, r.DatabaseName, r.Path, r.PhysicalName})
	}
	for _, d := range result.DiskSpace {
		_ = cw.Write([]string{"disk_space", d.Severity, d.DriveLetter,
			fmt.Sprintf("free_mb=%d total_mb=%d", d.FreeSpaceMB, d.TotalSizeMB), d.Message})
	}
	for _, l := range result.LogHealth {
		_ = cw.Write([]string{"log_health", l.Severity, l.DatabaseName,
			fmt.Sprintf("vlf_count=%d", l.VLFCount), l.Message})
	}
	for _, t := range result.LongTxns {
		_ = cw.Write([]string{"long_transaction", t.Severity, t.DatabaseName,
			fmt.Sprintf("elapsed_sec=%d session_id=%d", t.ElapsedSeconds, t.SessionID), t.Message})
	}
	for _, s := range result.Summary {
		_ = cw.Write([]string{"summary", s.Severity, "",
			fmt.Sprintf("%s critical=%d warning=%d count=%d", s.Category, s.Critical, s.Warning, s.Count), ""})
	}
	_ = cw.Write([]string{"meta", "", "", "health_score", fmt.Sprintf("%d (%s)", result.HealthScore, result.HealthStatus)})
	cw.Flush()
}

func writePgRuleChecksCSV(w http.ResponseWriter, filename string, data map[string]interface{}) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"rule_id", "rule_name", "category", "severity", "status", "current_value", "recommended_value", "description"})

	writeCheckRow := func(row map[string]interface{}) {
		ruleID := row["rule_id"]
		if ruleID == nil {
			ruleID = row["id"]
		}
		ruleName := row["rule_name"]
		if ruleName == nil {
			ruleName = row["name"]
		}
		rec := row["recommended_value"]
		if rec == nil {
			rec = row["recommendation"]
		}
		_ = cw.Write([]string{
			fmt.Sprint(ruleID),
			fmt.Sprint(ruleName),
			fmt.Sprint(row["category"]),
			fmt.Sprint(row["severity"]),
			fmt.Sprint(row["status"]),
			fmt.Sprint(row["current_value"]),
			fmt.Sprint(rec),
			fmt.Sprint(row["description"]),
		})
	}
	switch checks := data["checks"].(type) {
	case []map[string]interface{}:
		for _, row := range checks {
			writeCheckRow(row)
		}
	case []interface{}:
		for _, item := range checks {
			if row, ok := item.(map[string]interface{}); ok {
				writeCheckRow(row)
			}
		}
	}
	cw.Flush()
}

func bestPracticesFromAny(v interface{}) (models.BestPracticesResult, bool) {
	switch t := v.(type) {
	case models.BestPracticesResult:
		return t, true
	case *models.BestPracticesResult:
		if t != nil {
			return *t, true
		}
	}
	return models.BestPracticesResult{}, false
}

func guardrailsFromAny(v interface{}) (models.GuardrailsResult, bool) {
	switch t := v.(type) {
	case models.GuardrailsResult:
		return t, true
	case *models.GuardrailsResult:
		if t != nil {
			return *t, true
		}
	}
	return models.GuardrailsResult{}, false
}

func csvFilename(prefix, instance string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, instance)
	if safe == "" {
		safe = "instance"
	}
	return fmt.Sprintf("%s_%s.csv", prefix, safe)
}
