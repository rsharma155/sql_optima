// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
//
// Purpose: OS-level metrics ingestion from the os_collector agent (admin JWT required).

package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/service"
)

type OSMetricsHandlers struct {
	metricsSvc *service.MetricsService
}

func NewOSMetricsHandlers(svc *service.MetricsService) *OSMetricsHandlers {
	return &OSMetricsHandlers{metricsSvc: svc}
}

// ReceiveMetrics accepts either the os_collector flat payload or legacy {server_id, metrics}.
func (h *OSMetricsHandlers) ReceiveMetrics(w http.ResponseWriter, r *http.Request) {
	if h.metricsSvc == nil || !h.metricsSvc.IsOSMetricsIngestEnabled(r.Context()) {
		apiresponse.WriteJSONError(w, http.StatusForbidden,
			"OS metrics ingest is disabled. Enable it from the OS Collector panel in the UI (Admin/DBA) or set OS_METRICS_INGEST_ENABLED=1.", nil)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "invalid JSON", err)
		return
	}

	// os_collector agent payload (instance_name + host telemetry)
	if _, ok := probe["instance_name"]; ok {
		var p service.OSCollectorPayload
		if err := json.Unmarshal(body, &p); err != nil {
			apiresponse.WriteJSONError(w, http.StatusBadRequest, "invalid os collector payload", err)
			return
		}
		if err := h.metricsSvc.IngestOSCollectorPayload(r.Context(), &p); err != nil {
			apiresponse.WriteJSONError(w, http.StatusBadRequest, "failed to ingest os metrics", err, "instance", p.InstanceName)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Legacy API shape
	var legacy struct {
		ServerID uuid.UUID              `json:"server_id"`
		Metrics  map[string]interface{} `json:"metrics"`
	}
	if err := json.Unmarshal(body, &legacy); err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}
	if legacy.ServerID == uuid.Nil || legacy.Metrics == nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "server_id and metrics are required", nil)
		return
	}
	if err := h.metricsSvc.SaveOSMetrics(r.Context(), legacy.ServerID, uuid.Nil, legacy.Metrics); err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to save os metrics", err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *OSMetricsHandlers) SaveMetrics(w http.ResponseWriter, r *http.Request) {
	h.ReceiveMetrics(w, r)
}
