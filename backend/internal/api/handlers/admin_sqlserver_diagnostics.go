// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Admin-only SQL Server collector / TimescaleDB diagnostic endpoint.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5"
	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/domain/servers"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/service"
)

type AdminSqlServerDiagnosticsHandlers struct {
	metricsSvc *service.MetricsService
	cfg          *config.Config
}

func NewAdminSqlServerDiagnosticsHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *AdminSqlServerDiagnosticsHandlers {
	return &AdminSqlServerDiagnosticsHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

// GetSqlServerDiagnostics handles GET /api/admin/diagnostics/sqlserver/{instance}
// and GET /api/admin/diagnostics/sqlserver?instance= (or server_id= / server=).
func (h *AdminSqlServerDiagnosticsHandlers) GetSqlServerDiagnostics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	serverID, serverName, ok := h.resolveSqlServerInstance(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "TimescaleDB not connected"})
		return
	}

	repo := repository.NewSqlServerCollectorDiagnosticsRepository(pool)
	metaName, dbType, registryActive, err := repo.GetServerMeta(r.Context(), serverID)
	if err == pgx.ErrNoRows {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server not found in registry"})
		return
	}
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "could not load server metadata", err)
		return
	}
	if strings.ToLower(dbType) != "sqlserver" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "diagnostics are only available for SQL Server instances"})
		return
	}
	if metaName != "" {
		serverName = metaName
	}

	connStatus := h.connectionStatus(serverName)
	from, to := diagnosticTimeWindow(r)

	report, err := repo.GetDiagnostics(r.Context(), serverID, serverName, registryActive, connStatus, from, to)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "could not build diagnostics", err)
		return
	}
	_ = json.NewEncoder(w).Encode(report)
}

func (h *AdminSqlServerDiagnosticsHandlers) resolveSqlServerInstance(r *http.Request) (uuid.UUID, string, bool) {
	vars := mux.Vars(r)
	candidates := []string{vars["instance"]}
	for _, p := range []string{"instance", "server_id", "server"} {
		if q := strings.TrimSpace(r.URL.Query().Get(p)); q != "" {
			candidates = append(candidates, q)
		}
	}

	for _, val := range candidates {
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		if id, err := uuid.Parse(val); err == nil {
			name := h.nameForServerID(id)
			return id, name, true
		}
		if err := validateInstanceName(val); err != nil {
			continue
		}
		if h.cfg != nil {
			up := strings.ToUpper(val)
			for _, inst := range h.cfg.Instances {
				if strings.ToUpper(inst.Name) == up && strings.ToLower(inst.Type) == "sqlserver" {
					return inst.ServerID, inst.Name, true
				}
			}
		}
		if h.metricsSvc != nil && h.metricsSvc.ServerRepo != nil {
			if s, err := h.metricsSvc.ServerRepo.GetByName(r.Context(), val); err == nil && s.DBType == servers.DBSQLServer {
				return s.ID, s.Name, true
			}
		}
	}
	return uuid.Nil, "", false
}

func (h *AdminSqlServerDiagnosticsHandlers) nameForServerID(id uuid.UUID) string {
	if h.cfg != nil {
		for _, inst := range h.cfg.Instances {
			if inst.ServerID == id {
				return inst.Name
			}
		}
	}
	return id.String()
}

func (h *AdminSqlServerDiagnosticsHandlers) connectionStatus(serverName string) string {
	if h.metricsSvc == nil || h.metricsSvc.MsRepo == nil || serverName == "" {
		return ""
	}
	return h.metricsSvc.MsRepo.GetInstanceStatus(serverName)
}

func diagnosticTimeWindow(r *http.Request) (from, to time.Time) {
	to = time.Now().UTC()
	hours := 24
	if raw := strings.TrimSpace(r.URL.Query().Get("hours")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			hours = n
		}
	}
	if hours < 1 {
		hours = 1
	}
	if hours > 168 {
		hours = 168
	}
	from = to.Add(-time.Duration(hours) * time.Hour)
	return from, to
}
