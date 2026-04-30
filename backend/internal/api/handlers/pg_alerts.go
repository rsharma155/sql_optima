// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL alerting and health thresholds.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)


func (h *PostgresHandlers) Alerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
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
	if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
		return
	}

	alerts, err := h.metricsSvc.PgRepo.GetAlerts(instance)
	if err != nil {
		log.Printf("[API] PG alerts error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"alerts": []interface{}{}})
		return
	}
	if alerts == nil {
		alerts = []models.PgAlert{}
	}

	// Enrich with Timescale-backed and host telemetry alerts when available.
	now := time.Now()
	ts := now.Format("2006-01-02 15:04:05")

	// 1) Blocking / incident severity (Timescale-backed)
	if h.metricsSvc.IsTimescaleConnected() {
		if k, kerr := h.metricsSvc.GetPgLocksBlockingKpis(r.Context(), instance); kerr == nil && k != nil {
			victims := k.ActiveBlockingSessions
			idleRisk := k.IdleInTxnRiskCount
			depth := k.ChainDepth
			dur := k.IncidentDurationMins
			score := (victims * 10) + (depth * 5) + (idleRisk * 30) + (dur * 2)

			if victims > 0 {
				alerts = append(alerts, models.PgAlert{
					Severity:   "CRITICAL",
					Metric:     "Blocking Sessions",
					Threshold:  "> 0",
					CurrentVal: fmt.Sprintf("%d (score=%d)", victims, score),
					Timestamp:  ts,
					Status:     "ACTIVE",
				})
			} else if score >= 50 {
				alerts = append(alerts, models.PgAlert{
					Severity:   "WARNING",
					Metric:     "Blocking Incident Severity",
					Threshold:  ">= 50",
					CurrentVal: fmt.Sprintf("%d", score),
					Timestamp:  ts,
					Status:     "LOGGED",
				})
			}
		}
	}

	// 2) CPU / memory thresholds (best-effort, from postgres system stats detail if available)
	if detail, derr := h.metricsSvc.PgRepo.GetSystemStatsDetail(instance); derr == nil && detail != nil {
		// Host CPU
		if detail.CPUUsagePct >= 95 {
			alerts = append(alerts, models.PgAlert{Severity: "CRITICAL", Metric: "Host CPU", Threshold: ">= 95%", CurrentVal: fmt.Sprintf("%.1f%%", detail.CPUUsagePct), Timestamp: ts, Status: "ACTIVE"})
		} else if detail.CPUUsagePct >= 85 {
			alerts = append(alerts, models.PgAlert{Severity: "WARNING", Metric: "Host CPU", Threshold: ">= 85%", CurrentVal: fmt.Sprintf("%.1f%%", detail.CPUUsagePct), Timestamp: ts, Status: "LOGGED"})
		}
		// Memory
		if detail.MemoryUsedPct >= 95 {
			alerts = append(alerts, models.PgAlert{Severity: "CRITICAL", Metric: "Host Memory", Threshold: ">= 95%", CurrentVal: fmt.Sprintf("%.1f%%", detail.MemoryUsedPct), Timestamp: ts, Status: "ACTIVE"})
		} else if detail.MemoryUsedPct >= 85 {
			alerts = append(alerts, models.PgAlert{Severity: "WARNING", Metric: "Host Memory", Threshold: ">= 85%", CurrentVal: fmt.Sprintf("%.1f%%", detail.MemoryUsedPct), Timestamp: ts, Status: "LOGGED"})
		}
	} else if cu, mu, e := h.metricsSvc.PgRepo.GetSystemStats(instance); e == nil {
		if cu >= 95 {
			alerts = append(alerts, models.PgAlert{Severity: "CRITICAL", Metric: "Host CPU", Threshold: ">= 95%", CurrentVal: fmt.Sprintf("%.1f%%", cu), Timestamp: ts, Status: "ACTIVE"})
		} else if cu >= 85 {
			alerts = append(alerts, models.PgAlert{Severity: "WARNING", Metric: "Host CPU", Threshold: ">= 85%", CurrentVal: fmt.Sprintf("%.1f%%", cu), Timestamp: ts, Status: "LOGGED"})
		}
		if mu >= 95 {
			alerts = append(alerts, models.PgAlert{Severity: "CRITICAL", Metric: "Host Memory", Threshold: ">= 95%", CurrentVal: fmt.Sprintf("%.1f%%", mu), Timestamp: ts, Status: "ACTIVE"})
		} else if mu >= 85 {
			alerts = append(alerts, models.PgAlert{Severity: "WARNING", Metric: "Host Memory", Threshold: ">= 85%", CurrentVal: fmt.Sprintf("%.1f%%", mu), Timestamp: ts, Status: "LOGGED"})
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"alerts":   alerts,
	})
}
