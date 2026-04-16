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

func HandleHealthReadiness(w http.ResponseWriter, r *http.Request, cfg *config.Config, metricsSvc *service.MetricsService) {
	w.Header().Set("Content-Type", "application/json")

	if len(cfg.Instances) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "unhealthy",
			"reason":  "no database instances configured",
			"version": "1.0.0",
		})
		return
	}

	tsOK := true
	if metricsSvc != nil && os.Getenv("HEALTH_CHECK_TIMESCALE") == "1" {
		tsOK = metricsSvc.TimescalePing(r.Context()) == nil
		if !tsOK {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "unhealthy",
				"reason":  "timescale unavailable",
				"version": "1.0.0",
			})
			return
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"instances": len(cfg.Instances),
		"version":   "1.0.0",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
