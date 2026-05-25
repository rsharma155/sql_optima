// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Provides health check endpoints for liveness and readiness probes. Validates query configuration, database instances, and optional TimescaleDB connectivity.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Version   string `json:"version,omitempty"`
}

func HandleHealthLiveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

type depStatus struct {
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

func HandleHealthReadiness(w http.ResponseWriter, r *http.Request, cfg *config.Config, metricsSvc *service.MetricsService) {
	w.Header().Set("Content-Type", "application/json")

	if len(cfg.Instances) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "unhealthy",
			"reason":  "no database instances configured",
		})
		return
	}

	deps := map[string]depStatus{}
	overallOK := true

	// — TimescaleDB probe ——————————————————————————————————————————————————
	tsStatus := depStatus{Status: "not_configured"}
	if metricsSvc != nil && metricsSvc.IsTimescaleConnected() {
		start := time.Now()
		if err := metricsSvc.TimescalePing(r.Context()); err != nil {
			tsStatus = depStatus{Status: "unreachable", Error: err.Error()}
			overallOK = false
		} else {
			tsStatus = depStatus{Status: "ok", LatencyMs: time.Since(start).Milliseconds()}
		}
	}
	deps["timescaledb"] = tsStatus

	// — Vault probe ————————————————————————————————————————————————————————
	vaultAddr := os.Getenv("VAULT_ADDR")
	vaultStatus := depStatus{Status: "not_configured"}
	if vaultAddr != "" {
		addr := strings.TrimRight(vaultAddr, "/") + "/v1/sys/health"
		client := &http.Client{Timeout: 3 * time.Second}
		start := time.Now()
		resp, err := client.Get(addr) //nolint:noctx
		if err != nil {
			vaultStatus = depStatus{Status: "unreachable", Error: err.Error()}
		} else {
			resp.Body.Close()
			latency := time.Since(start).Milliseconds()
			// Vault /v1/sys/health returns 200 (initialized+unsealed), 429 (standby), 472/473 (DR)
			if resp.StatusCode == 200 || resp.StatusCode == 429 {
				vaultStatus = depStatus{Status: "ok", LatencyMs: latency}
			} else {
				vaultStatus = depStatus{Status: "degraded", LatencyMs: latency}
			}
		}
	}
	deps["vault"] = vaultStatus

	overallStatus := "ok"
	if !overallOK {
		overallStatus = "degraded"
	}

	httpCode := http.StatusOK
	if !overallOK {
		httpCode = http.StatusServiceUnavailable
	}

	w.WriteHeader(httpCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       overallStatus,
		"dependencies": deps,
		"instances":    len(cfg.Instances),
		"timestamp":    time.Now().Format(time.RFC3339),
	})
}
