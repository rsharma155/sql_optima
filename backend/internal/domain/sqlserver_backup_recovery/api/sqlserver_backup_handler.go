// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for SQL Server Backup & Recovery dashboard and policy APIs.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/api/handlers"
	backupdomain "github.com/rsharma155/sql_optima/internal/domain/sqlserver_backup_recovery/domain"
	"github.com/rsharma155/sql_optima/internal/domain/sqlserver_backup_recovery/domain/repositories"
	hadomain "github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/domain"
	ha_repo "github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/repository"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/service"
)

type SQLServerBackupHandler struct {
	metricsSvc *service.MetricsService
	haRepo     *ha_repo.HAReplicationRepository
}

func NewSQLServerBackupHandler(svc *service.MetricsService) *SQLServerBackupHandler {
	h := &SQLServerBackupHandler{metricsSvc: svc}
	if pool := svc.GetTimescaleDBPool(); pool != nil {
		h.haRepo = ha_repo.NewHAReplicationRepository(pool)
	}
	return h
}

func (h *SQLServerBackupHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	serverID, instanceName, ok := h.resolveInstance(r)
	if !ok {
		return
	}
	from, to := parseTimeRange(r)
	ctx := r.Context()

	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "TimescaleDB unavailable"})
		return
	}

	policyRepo := repositories.NewSQLServerBackupPolicyRepository(pool)
	policy, _ := policyRepo.Get(ctx, serverID)

	var posture []backupdomain.DatabasePosture
	var history []backupdomain.BackupHistoryRow
	var historyTrend []map[string]interface{}
	compressionDefault := false

	tsLogger := h.metricsSvc.GetTimescaleDBLogger()
	dashRepo := repositories.NewSQLServerBackupDashboardRepository(tsLogger)
	if maps, err := dashRepo.GetPostureMaps(ctx, serverID); err == nil && len(maps) > 0 {
		posture = repositories.MapsToPosture(maps)
		if len(posture) > 0 {
			compressionDefault = posture[0].BackupCompressionDefault
		}
	}
	if maps, err := dashRepo.GetHistoryMaps(ctx, serverID, from, to, 200); err == nil {
		history = repositories.MapsToHistory(maps)
	}
	historyTrend, _ = dashRepo.GetHistoryTrend(ctx, serverID, from, to)

	if len(posture) == 0 && h.metricsSvc.MsRepo != nil && instanceName != "" {
		if live, comp, err := h.metricsSvc.MsRepo.FetchBackupPostureLive(ctx, instanceName); err == nil && len(live) > 0 {
			posture = live
			compressionDefault = comp
		}
	}
	if len(history) == 0 && h.metricsSvc.MsRepo != nil && instanceName != "" {
		if liveHist, err := h.metricsSvc.MsRepo.FetchBackupHistoryLive(ctx, instanceName, 200); err == nil {
			history = liveHist
		}
	}

	backupdomain.ApplyPolicyFreshness(posture, policy)

	fromS, toS := from.Format(time.RFC3339), to.Format(time.RFC3339)
	failedJobs := 0
	failures, _ := h.metricsSvc.GetSQLServerJobFailures(ctx, serverID, fromS, toS, 100)
	failedJobs = filterFailedBackupJobs(failures)

	logRows, _ := h.metricsSvc.GetSQLServerLogShippingHealth(ctx, serverID)
	logBehind, logEnabled := logShippingSignals(logRows)

	readiness := backupdomain.ComputeReadiness(posture, policy, failedJobs, logBehind, logEnabled)
	overall, _ := splitOverall(readiness.Overall)

	haCtx := map[string]interface{}{}
	if h.haRepo != nil {
		if feat, err := h.haRepo.GetFeatureDetection(ctx, serverID, ""); err == nil {
			haCtx["ag_enabled"] = feat.AGEnabled
			haCtx["log_shipping_enabled"] = feat.LogShippingEnabled
			haCtx["replication_enabled"] = feat.ReplicationEnabled
			haCtx["ha_enabled"] = feat.HAEnabled
		}
		if cov, err := h.haRepo.GetDatabaseCoverage(ctx, serverID); err == nil {
			mergeCoverage(&posture, cov)
			haCtx["coverage"] = cov
		}
	}

	payload := backupdomain.DashboardPayload{
		Readiness: backupdomain.ReadinessSummary{
			Overall: overall,
			Chips:   readiness.Chips,
		},
		KPIs:           dashRepo.BuildKPIs(posture, history, failedJobs),
		Posture:        posture,
		History:        history,
		HistoryTrend:   historyTrend,
		BackupJobs:     loadBackupJobs(ctx, h, serverID, fromS, toS),
		LogShipping:    logRows,
		HAContext:      haCtx,
		InstanceConfig: map[string]interface{}{"backup_compression_default": compressionDefault},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Data-Source", "timescale")
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *SQLServerBackupHandler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	serverID, _, ok := h.resolveInstance(r)
	if !ok {
		return
	}
	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	p, err := repositories.NewSQLServerBackupPolicyRepository(pool).Get(r.Context(), serverID)
	if err != nil {
		p = backupdomain.DefaultBackupPolicy(serverID)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (h *SQLServerBackupHandler) PutPolicy(w http.ResponseWriter, r *http.Request) {
	serverID, _, ok := h.resolveInstance(r)
	if !ok {
		return
	}
	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	var body backupdomain.BackupPolicy
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	body.ServerID = serverID
	if body.RPOFullBackupHours <= 0 {
		body.RPOFullBackupHours = 24
	}
	if body.RPOLogBackupMinutes <= 0 {
		body.RPOLogBackupMinutes = 15
	}
	actor := "system"
	if c := middleware.GetAuthClaims(r); c != nil && c.Username != "" {
		actor = c.Username
	}
	repo := repositories.NewSQLServerBackupPolicyRepository(pool)
	if err := repo.Upsert(r.Context(), body, actor); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	p, _ := repo.Get(r.Context(), serverID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (h *SQLServerBackupHandler) resolveInstance(r *http.Request) (uuid.UUID, string, bool) {
	serverID, ok := handlers.ParseServerID(r, h.metricsSvc.Config)
	if !ok {
		return uuid.Nil, "", false
	}
	inst := r.URL.Query().Get("instance")
	if inst != "" {
		return serverID, inst, true
	}
	for _, i := range h.metricsSvc.Config.Instances {
		if i.ServerID == serverID {
			return serverID, i.Name, true
		}
	}
	return serverID, inst, true
}

func parseTimeRange(r *http.Request) (time.Time, time.Time) {
	to := time.Now().UTC()
	from := to.Add(-24 * time.Hour)
	if f := r.URL.Query().Get("from"); f != "" {
		if t, err := time.Parse(time.RFC3339, f); err == nil {
			from = t
		}
	}
	if t := r.URL.Query().Get("to"); t != "" {
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			to = parsed
		}
	}
	return from, to
}

func splitOverall(s string) (string, string) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return s, "ok"
}

func logShippingSignals(rows []map[string]interface{}) (behind int, enabled bool) {
	if len(rows) > 0 {
		enabled = true
	}
	for _, row := range rows {
		delay := int(numVal(row, "restore_delay_minutes"))
		thresh := int(numVal(row, "restore_threshold_minutes"))
		if thresh > 0 && delay > thresh {
			behind++
		}
	}
	return behind, enabled
}

func numVal(m map[string]interface{}, k string) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}

func filterFailedBackupJobs(failures []map[string]interface{}) int {
	n := 0
	for _, f := range failures {
		name := strings.ToLower(stringVal(f, "job_name"))
		if strings.Contains(name, "backup") || strings.Contains(name, "maintenance") {
			n++
		}
	}
	return n
}

func stringVal(m map[string]interface{}, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

func loadBackupJobs(ctx context.Context, h *SQLServerBackupHandler, serverID uuid.UUID, from, to string) map[string]interface{} {
	jobs, _ := h.metricsSvc.GetSQLServerJobDetails(ctx, serverID, from, to)
	failures, _ := h.metricsSvc.GetSQLServerJobFailures(ctx, serverID, from, to, 50)
	metrics, _ := h.metricsSvc.GetSQLServerJobMetrics(ctx, serverID, from, to, 1)

	filteredJobs := filterBackupCategoryJobs(jobs)
	filteredFails := filterBackupCategoryFailures(failures)

	summary := map[string]interface{}{
		"total_jobs": 0, "enabled_jobs": 0, "failed_jobs_24h": 0,
	}
	if len(metrics) > 0 {
		m := metrics[0]
		summary["total_jobs"] = m["total_jobs"]
		summary["enabled_jobs"] = m["enabled_jobs"]
		summary["failed_jobs_24h"] = m["failed_jobs_24h"]
	}
	return map[string]interface{}{
		"summary":  summary,
		"jobs":     filteredJobs,
		"failures": filteredFails,
	}
}

func filterBackupCategoryJobs(jobs []map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0)
	for _, j := range jobs {
		cat := strings.ToLower(stringVal(j, "job_category"))
		name := strings.ToLower(stringVal(j, "job_name"))
		if strings.Contains(cat, "backup") || strings.Contains(cat, "maintenance") ||
			strings.Contains(name, "backup") {
			out = append(out, j)
		}
	}
	return out
}

func filterBackupCategoryFailures(failures []map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0)
	for _, f := range failures {
		name := strings.ToLower(stringVal(f, "job_name"))
		if strings.Contains(name, "backup") || strings.Contains(name, "maintenance") {
			out = append(out, f)
		}
	}
	return out
}

func mergeCoverage(posture *[]backupdomain.DatabasePosture, cov []hadomain.DatabaseCoverage) {
	byName := make(map[string]hadomain.DatabaseCoverage, len(cov))
	for _, c := range cov {
		byName[c.DatabaseName] = c
	}
	for i := range *posture {
		if c, ok := byName[(*posture)[i].DatabaseName]; ok {
			(*posture)[i].InHA = c.InHA
			(*posture)[i].ProtectionLevel = string(c.ProtectionLevel)
		}
	}
}
