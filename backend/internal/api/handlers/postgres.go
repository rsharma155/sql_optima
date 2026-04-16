// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL dashboard including replication, sessions, locks, backups, vacuum progress, and configuration drift.
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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type PostgresHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewPostgresHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *PostgresHandlers {
	return &PostgresHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

func (h *PostgresHandlers) DBObservation(w http.ResponseWriter, r *http.Request) {
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

	metrics := h.metricsSvc.GetPostgresDBObservationMetrics(instance)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (h *PostgresHandlers) Overview(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.metricsSvc.GetPostgresOverview(instance))
}

func (h *PostgresHandlers) ServerInfo(w http.ResponseWriter, r *http.Request) {
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

	version, uptime, err := h.metricsSvc.PgRepo.GetServerInfo(instance)
	if err != nil {
		log.Printf("[API] PG server-info error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"version": version,
		"uptime":  uptime,
	})
}

func (h *PostgresHandlers) SystemStats(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	detail, err := h.metricsSvc.PgRepo.GetSystemStatsDetail(instance)
	if err != nil || detail == nil {
		log.Printf("[API] PG system-stats error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"cpu_usage":    0,
			"memory_usage": 0,
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"cpu_usage":              detail.CPUUsagePct,
		"memory_usage":           detail.MemoryUsedPct,
		"total_memory_bytes":     detail.TotalMemoryBytes,
		"available_memory_bytes": detail.AvailableMemoryBytes,
		"shared_buffers_bytes":   detail.SharedBuffersBytes,
	})
}

func (h *PostgresHandlers) SystemStatsHistory(w http.ResponseWriter, r *http.Request) {
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
	limit := 120
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1440 {
			limit = n
		}
	}

	rows, err := h.metricsSvc.GetTimescalePostgresSystemStats(instance, limit)
	if err != nil {
		log.Printf("[API] PG system-stats history error for %s: %v", instance, err)
		rows = []hot.PostgresSystemStatsRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"stats":    rows,
	})
}

func (h *PostgresHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	database := r.URL.Query().Get("database")
	if database == "" {
		database = "all"
	}

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Allow dashboard for any instance, even if not in config (for newly added servers)
	// if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
	// 	w.WriteHeader(http.StatusNotFound)
	// 	json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
	// 	return
	// }

	// if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
	// 	w.WriteHeader(http.StatusBadRequest)
	// 	json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
	// 	return
	// }

	thr := h.metricsSvc.GetCachedPgThroughputDashboard(instance, database)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"labels":        thr.Labels,
		"tps":           thr.Tps,
		"cache_hit_pct": thr.CacheHitPct,
	})
}

func (h *PostgresHandlers) BGWriter(w http.ResponseWriter, r *http.Request) {
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

	stats, err := h.metricsSvc.GetPostgresCheckpointSummary(r.Context(), instance, 50)
	if err != nil {
		log.Printf("[API] PG bgwriter error for %s: %v", instance, err)
		stats = []map[string]interface{}{}
	}
	if stats == nil {
		stats = []map[string]interface{}{}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"stats":    stats,
	})
}

func (h *PostgresHandlers) Archiver(w http.ResponseWriter, r *http.Request) {
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

	stats, err := h.metricsSvc.GetPostgresArchiveSummary(r.Context(), instance, 50)
	if err != nil {
		log.Printf("[API] PG archiver error for %s: %v", instance, err)
		stats = []map[string]interface{}{}
	}
	if stats == nil {
		stats = []map[string]interface{}{}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"stats":    stats,
	})
}

func (h *PostgresHandlers) WaitEventsHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid instance"})
		return
	}
	limit := 400
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "rows": []interface{}{}, "source": "none"})
		return
	}
	rows, err := h.metricsSvc.GetPostgresWaitEventsHistory(r.Context(), instance, limit)
	if err != nil {
		log.Printf("[API] PG waits/history error for %s: %v", instance, err)
		rows = []hot.PostgresWaitEventRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"rows":     rows,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) DbIOHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid instance"})
		return
	}
	limit := 800
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 10000 {
			limit = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "rows": []interface{}{}, "source": "none"})
		return
	}
	rows, err := h.metricsSvc.GetPostgresDbIOHistory(r.Context(), instance, limit)
	if err != nil {
		log.Printf("[API] PG io/history error for %s: %v", instance, err)
		rows = []hot.PostgresDbIORow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"rows":     rows,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) SettingsDrift(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid instance"})
		return
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "changes": []interface{}{}, "source": "none"})
		return
	}

	latestTs, prevTs, latest, prev, err := h.metricsSvc.GetPostgresSettingsSnapshotLatestTwo(r.Context(), instance)
	if err != nil {
		log.Printf("[API] PG settings/drift error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "changes": []interface{}{}, "source": "timescale"})
		return
	}

	prevMap := map[string]hot.PostgresSettingSnapshotRow{}
	for _, r := range prev {
		prevMap[r.Name] = r
	}
	type change struct {
		Name      string `json:"name"`
		OldValue  string `json:"old_value"`
		NewValue  string `json:"new_value"`
		Unit      string `json:"unit"`
		OldSource string `json:"old_source"`
		NewSource string `json:"new_source"`
	}
	var changes []change
	for _, r := range latest {
		p, ok := prevMap[r.Name]
		if !ok {
			continue
		}
		if p.Setting != r.Setting || p.Source != r.Source || p.Unit != r.Unit {
			changes = append(changes, change{
				Name:      r.Name,
				OldValue:  p.Setting,
				NewValue:  r.Setting,
				Unit:      r.Unit,
				OldSource: p.Source,
				NewSource: r.Source,
			})
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":           instance,
		"latest_timestamp":   latestTs,
		"previous_timestamp": prevTs,
		"changes":            changes,
		"source":             "timescale",
	})
}

func (h *PostgresHandlers) Databases(w http.ResponseWriter, r *http.Request) {
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

	databases, err := h.metricsSvc.PgRepo.GetDatabases(instance)
	if err != nil {
		log.Printf("[API] PG databases error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"databases": []string{}})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":  instance,
		"databases": databases,
	})
}

// ControlCenter returns the latest derived DBA-first metrics snapshot from TimescaleDB.
func (h *PostgresHandlers) ControlCenter(w http.ResponseWriter, r *http.Request) {
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

	row, err := h.metricsSvc.GetLatestPostgresControlCenterStats(r.Context(), instance)
	if err != nil {
		log.Printf("[API] PG control-center error for %s: %v", instance, err)
		row = nil
	}
	if h.metricsSvc.IsTimescaleConnected() {
		w.Header().Set("X-Data-Source", "timescale")
	} else {
		w.Header().Set("X-Data-Source", "live_cache_fallback")
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"stats":    row,
	})
}

func (h *PostgresHandlers) ControlCenterHistory(w http.ResponseWriter, r *http.Request) {
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
	limit := 60
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 720 {
			limit = n
		}
	}
	hist, err := h.metricsSvc.GetPostgresControlCenterHistory(r.Context(), instance, limit)
	if err != nil {
		log.Printf("[API] PG control-center history error for %s: %v", instance, err)
		hist = &hot.PostgresControlCenterHistory{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history":  hist,
	})
}

func (h *PostgresHandlers) ReplicationLagHistory(w http.ResponseWriter, r *http.Request) {
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
	limit := 120
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1440 {
			limit = n
		}
	}
	series, err := h.metricsSvc.GetPostgresReplicationLagDetail(r.Context(), instance, limit)
	if err != nil {
		log.Printf("[API] PG replication lag history error for %s: %v", instance, err)
		series = map[string]hot.PostgresReplicationLagSeries{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"series":   series,
	})
}

func (h *PostgresHandlers) ReplicationSlots(w http.ResponseWriter, r *http.Request) {
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

	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}

	// Prefer Timescale history; fall back to live pg_replication_slots if Timescale unavailable.
	if h.metricsSvc.IsTimescaleConnected() {
		rows, err := h.metricsSvc.GetTimescalePostgresReplicationSlots(instance, limit)
		if err != nil {
			log.Printf("[API] PG replication-slots error for %s: %v", instance, err)
			rows = []hot.PostgresReplicationSlotRow{}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"slots":    rows,
			"source":   "timescale",
		})
		return
	}

	live, err := h.metricsSvc.PgRepo.GetReplicationSlotStats(instance)
	if err != nil {
		log.Printf("[API] PG replication-slots live error for %s: %v", instance, err)
		live = []repository.PgReplicationSlotStat{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"slots":    live,
		"source":   "live",
	})
}

func (h *PostgresHandlers) Disk(w http.ResponseWriter, r *http.Request) {
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

	rows, err := h.metricsSvc.GetTimescalePostgresDiskStats(instance, 200)
	if err != nil {
		log.Printf("[API] PG disk error for %s: %v", instance, err)
		rows = []hot.PostgresDiskStatRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"stats":    rows,
		"source":   "timescale",
	})
}

type backupReportRequest struct {
	Instance         string                 `json:"instance"`
	Tool             string                 `json:"tool"`
	BackupType       string                 `json:"backup_type"`
	Status           string                 `json:"status"`
	StartedAt        *time.Time             `json:"started_at,omitempty"`
	FinishedAt       *time.Time             `json:"finished_at,omitempty"`
	DurationSeconds  int64                  `json:"duration_seconds"`
	WalArchivedUntil *time.Time             `json:"wal_archived_until,omitempty"`
	Repo             string                 `json:"repo,omitempty"`
	SizeBytes        int64                  `json:"size_bytes"`
	ErrorMessage     string                 `json:"error_message,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

func (h *PostgresHandlers) BackupReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req backupReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid json body"})
		return
	}
	instance := strings.TrimSpace(req.Instance)
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not connected"})
		return
	}

	row := hot.PostgresBackupRunRow{
		ServerInstanceName: instance,
		Tool:               req.Tool,
		BackupType:         req.BackupType,
		Status:             req.Status,
		StartedAt:          req.StartedAt,
		FinishedAt:         req.FinishedAt,
		DurationSeconds:    req.DurationSeconds,
		WalArchivedUntil:   req.WalArchivedUntil,
		Repo:               req.Repo,
		SizeBytes:          req.SizeBytes,
		ErrorMessage:       req.ErrorMessage,
		Metadata:           req.Metadata,
	}
	if err := h.metricsSvc.LogPostgresBackupRun(r.Context(), row); err != nil {
		log.Printf("[API] PG backup report insert error for %s: %v", instance, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to store backup report"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *PostgresHandlers) BackupLatest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	row, err := h.metricsSvc.GetLatestPostgresBackupRun(r.Context(), instance)
	if err != nil {
		log.Printf("[API] PG backup latest error for %s: %v", instance, err)
		row = nil
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"latest":   row,
	})
}

func (h *PostgresHandlers) BackupHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	rows, err := h.metricsSvc.GetPostgresBackupRunHistory(r.Context(), instance, limit)
	if err != nil {
		log.Printf("[API] PG backup history error for %s: %v", instance, err)
		rows = []hot.PostgresBackupRunRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history":  rows,
	})
}

type pgLogReportRequest struct {
	Instance string `json:"instance"`
	Events   []struct {
		CaptureTimestamp *time.Time             `json:"capture_timestamp,omitempty"`
		Severity         string                 `json:"severity"`
		SQLState         string                 `json:"sqlstate,omitempty"`
		Message          string                 `json:"message"`
		UserName         string                 `json:"user_name,omitempty"`
		DatabaseName     string                 `json:"database_name,omitempty"`
		ApplicationName  string                 `json:"application_name,omitempty"`
		ClientAddr       string                 `json:"client_addr,omitempty"`
		PID              int64                  `json:"pid,omitempty"`
		Context          string                 `json:"context,omitempty"`
		Detail           string                 `json:"detail,omitempty"`
		Hint             string                 `json:"hint,omitempty"`
		Raw              map[string]interface{} `json:"raw,omitempty"`
	} `json:"events"`
}

func (h *PostgresHandlers) LogsReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not connected"})
		return
	}

	var req pgLogReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid json body"})
		return
	}
	instance := strings.TrimSpace(req.Instance)
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
		return
	}
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	rows := make([]hot.PostgresLogEventRow, 0, len(req.Events))
	for _, e := range req.Events {
		if strings.TrimSpace(e.Message) == "" {
			continue
		}
		ts := time.Now().UTC()
		if e.CaptureTimestamp != nil {
			ts = e.CaptureTimestamp.UTC()
		}
		rows = append(rows, hot.PostgresLogEventRow{
			CaptureTimestamp:   ts,
			ServerInstanceName: instance,
			Severity:           e.Severity,
			SQLState:           e.SQLState,
			Message:            e.Message,
			UserName:           e.UserName,
			DatabaseName:       e.DatabaseName,
			ApplicationName:    e.ApplicationName,
			ClientAddr:         e.ClientAddr,
			PID:                e.PID,
			Context:            e.Context,
			Detail:             e.Detail,
			Hint:               e.Hint,
			Raw:                e.Raw,
		})
	}
	if len(rows) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "inserted": 0})
		return
	}

	if err := h.metricsSvc.LogPostgresLogEvents(r.Context(), instance, rows); err != nil {
		log.Printf("[API] PG logs report insert error for %s: %v", instance, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to store log events"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "inserted": len(rows)})
}

func (h *PostgresHandlers) LogsSummary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	windowMin := 60
	if wq := r.URL.Query().Get("window_minutes"); wq != "" {
		if n, err := strconv.Atoi(wq); err == nil && n > 0 && n <= 1440 {
			windowMin = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"summary":  (*hot.PostgresLogSummary)(nil),
			"source":   "none",
		})
		return
	}
	s, err := h.metricsSvc.GetPostgresLogSummary(r.Context(), instance, windowMin)
	if err != nil {
		log.Printf("[API] PG logs summary error for %s: %v", instance, err)
		s = &hot.PostgresLogSummary{WindowMinutes: windowMin}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"summary":  s,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) LogsRecent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	severity := r.URL.Query().Get("severity")
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"events":   []hot.PostgresLogEventRow{},
			"source":   "none",
		})
		return
	}
	rows, err := h.metricsSvc.GetPostgresLogEvents(r.Context(), instance, limit, severity)
	if err != nil {
		rows = []hot.PostgresLogEventRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"events":   rows,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) Config(w http.ResponseWriter, r *http.Request) {
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

	settings, err := h.metricsSvc.PgRepo.GetConfig(instance)
	if err != nil {
		log.Printf("[API] PG config error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"settings": []map[string]interface{}{}})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"settings": settings,
	})
}

func (h *PostgresHandlers) BestPractices(w http.ResponseWriter, r *http.Request) {
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

	result := h.metricsSvc.FetchPgBestPracticesWithTimescale(r.Context(), instance)
	if result.DataSource != "" {
		w.Header().Set("X-Data-Source", result.DataSource)
	}
	json.NewEncoder(w).Encode(result)
}

func (h *PostgresHandlers) Storage(w http.ResponseWriter, r *http.Request) {
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

	tables, terr := h.metricsSvc.PgRepo.GetTableStats(instance)
	indexes, ierr := h.metricsSvc.PgRepo.GetIndexStats(instance)

	if tables == nil {
		tables = []repository.PgTableStat{}
	}
	if indexes == nil {
		indexes = []repository.PgIndexStat{}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"tables":   tables,
		"indexes":  indexes,
		"error":    firstErrString(terr, ierr),
	})
}

func (h *PostgresHandlers) DatabaseSize(w http.ResponseWriter, r *http.Request) {
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
	json.NewEncoder(w).Encode(h.metricsSvc.PgRepo.GetDatabaseSizeStats(instance))
}

func (h *PostgresHandlers) VacuumProgress(w http.ResponseWriter, r *http.Request) {
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

	rows, err := h.metricsSvc.PgRepo.GetVacuumProgress(instance)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "progress": []interface{}{}})
		return
	}
	if rows == nil {
		rows = []repository.PgVacuumProgressRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"progress": rows,
		"source":   "live",
	})
}

func (h *PostgresHandlers) VacuumProgressHistory(w http.ResponseWriter, r *http.Request) {
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
	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	rows, err := h.metricsSvc.GetTimescalePostgresVacuumProgress(instance, limit)
	if err != nil {
		rows = []hot.PostgresVacuumProgressRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history":  rows,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) TableMaintenanceHistory(w http.ResponseWriter, r *http.Request) {
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
	schema := r.URL.Query().Get("schema")
	table := r.URL.Query().Get("table")
	if strings.TrimSpace(schema) == "" || strings.TrimSpace(table) == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "schema and table are required"})
		return
	}
	limit := 180
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	rows, err := h.metricsSvc.GetPostgresTableMaintenanceHistory(r.Context(), instance, schema, table, limit)
	if err != nil {
		rows = []hot.PostgresTableMaintRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"schema":   schema,
		"table":    table,
		"history":  rows,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) TableMaintenanceLatest(w http.ResponseWriter, r *http.Request) {
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
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "latest": []hot.PostgresTableMaintRow{}, "source": "none"})
		return
	}
	rows, err := h.metricsSvc.GetLatestPostgresTableMaintenance(r.Context(), instance, limit)
	if err != nil {
		rows = []hot.PostgresTableMaintRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"latest":   rows,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) SessionStateHistory(w http.ResponseWriter, r *http.Request) {
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

	limit := 180
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"history":  map[string]interface{}{},
			"source":   "none",
		})
		return
	}

	rows, err := h.metricsSvc.GetPostgresSessionStateCountsHistory(r.Context(), instance, limit)
	if err != nil {
		rows = []hot.PostgresSessionStateCountRow{}
	}
	// Return arrays for charting convenience (similar to control-center/history).
	labels := make([]string, 0, len(rows))
	active := make([]int, 0, len(rows))
	idle := make([]int, 0, len(rows))
	idleInTxn := make([]int, 0, len(rows))
	waiting := make([]int, 0, len(rows))
	total := make([]int, 0, len(rows))

	for i := len(rows) - 1; i >= 0; i-- { // oldest -> newest
		rw := rows[i]
		labels = append(labels, rw.CaptureTimestamp.UTC().Format(time.RFC3339))
		active = append(active, rw.ActiveCount)
		idle = append(idle, rw.IdleCount)
		idleInTxn = append(idleInTxn, rw.IdleInTxnCount)
		waiting = append(waiting, rw.WaitingCount)
		total = append(total, rw.TotalCount)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history": map[string]interface{}{
			"labels":      labels,
			"active":      active,
			"idle":        idle,
			"idle_in_txn": idleInTxn,
			"waiting":     waiting,
			"total":       total,
		},
		"source": "timescale",
	})
}

func (h *PostgresHandlers) PoolerLatest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "latest": nil, "source": "none"})
		return
	}
	row, err := h.metricsSvc.GetLatestPostgresPoolerStats(r.Context(), instance)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "latest": nil, "source": "timescale"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "latest": row, "source": "timescale"})
}

func (h *PostgresHandlers) PoolerHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	limit := 180
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "history": map[string]interface{}{}, "source": "none"})
		return
	}
	rows, err := h.metricsSvc.GetPostgresPoolerStatsHistory(r.Context(), instance, limit)
	if err != nil {
		rows = []hot.PostgresPoolerStatRow{}
	}
	labels := make([]string, 0, len(rows))
	clActive := make([]int, 0, len(rows))
	clWaiting := make([]int, 0, len(rows))
	svUsed := make([]int, 0, len(rows))
	maxwait := make([]float64, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		rw := rows[i]
		labels = append(labels, rw.CaptureTimestamp.UTC().Format(time.RFC3339))
		clActive = append(clActive, rw.ClActive)
		clWaiting = append(clWaiting, rw.ClWaiting)
		svUsed = append(svUsed, rw.SvUsed)
		maxwait = append(maxwait, rw.MaxwaitSeconds)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history": map[string]interface{}{
			"labels":     labels,
			"cl_active":  clActive,
			"cl_waiting": clWaiting,
			"sv_used":    svUsed,
			"maxwait_s":  maxwait,
		},
		"source": "timescale",
	})
}

func (h *PostgresHandlers) DeadlocksHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	limit := 180
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}
	minutes := 180
	if s := r.URL.Query().Get("window_minutes"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 10080 {
			minutes = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "history": map[string]interface{}{}, "source": "none"})
		return
	}
	rows, err := h.metricsSvc.GetPostgresDeadlocksHistory(r.Context(), instance, minutes, limit)
	if err != nil {
		rows = []hot.PostgresDeadlockStatRow{}
	}
	// Aggregate deltas across DBs per timestamp.
	byTs := map[string]int64{}
	for _, r := range rows {
		k := r.CaptureTimestamp.UTC().Format(time.RFC3339)
		byTs[k] += r.DeadlocksDelta
	}
	// Build ascending series.
	type kv struct {
		ts string
		v  int64
	}
	var xs []kv
	for ts, v := range byTs {
		xs = append(xs, kv{ts: ts, v: v})
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i].ts < xs[j].ts })
	labels := make([]string, 0, len(xs))
	deltas := make([]int64, 0, len(xs))
	for _, x := range xs {
		labels = append(labels, x.ts)
		deltas = append(deltas, x.v)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history": map[string]interface{}{
			"labels":          labels,
			"deadlocks_delta": deltas,
		},
		"source": "timescale",
	})
}

func (h *PostgresHandlers) LockWaitHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	limit := 400
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}
	minutes := 180
	if s := r.URL.Query().Get("window_minutes"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 10080 {
			minutes = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"history":  map[string]interface{}{},
			"source":   "none",
		})
		return
	}
	labels, counts, err := h.metricsSvc.GetPostgresLockWaitHistory(r.Context(), instance, minutes, limit)
	if err != nil {
		log.Printf("[API] PG lock-wait history error for %s: %v", instance, err)
		labels, counts = nil, nil
	}
	if labels == nil {
		labels = []string{}
	}
	if counts == nil {
		counts = []int{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history": map[string]interface{}{
			"labels":                labels,
			"lock_waiting_sessions": counts,
		},
		"source": "timescale",
	})
}

func (h *PostgresHandlers) LocksBlockingKPIs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"kpis":     map[string]interface{}{},
			"source":   "none",
		})
		return
	}
	k, err := h.metricsSvc.GetPgLocksBlockingKpis(r.Context(), instance)
	if err != nil || k == nil {
		log.Printf("[API] PG locks-blocking kpis error for %s: %v", instance, err)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"kpis":     map[string]interface{}{},
			"source":   "timescale",
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"kpis":     k,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) LocksBlockingTimeline(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance":  instance,
			"timeline":  []interface{}{},
			"incidents": []interface{}{},
			"source":    "none",
		})
		return
	}

	q := r.URL.Query()
	fromStr := strings.TrimSpace(q.Get("from"))
	toStr := strings.TrimSpace(q.Get("to"))
	var points []hot.PgBlockingTimelinePoint
	var incs []hot.PgBlockingIncident
	var err error
	// If from/to provided (RFC3339), prefer range mode. Otherwise fall back to window_hours.
	if fromStr != "" && toStr != "" {
		fromT, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid from (use RFC3339)"})
			return
		}
		toT, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid to (use RFC3339)"})
			return
		}
		points, err = h.metricsSvc.GetPgBlockingTimelineRange(r.Context(), instance, fromT, toT)
		if err != nil {
			log.Printf("[API] PG locks-blocking timeline range error for %s: %v", instance, err)
			points = []hot.PgBlockingTimelinePoint{}
		}
		incs, err = h.metricsSvc.GetPgBlockingIncidentsRange(r.Context(), instance, fromT, toT)
		if err != nil {
			log.Printf("[API] PG locks-blocking incidents range error for %s: %v", instance, err)
			incs = []hot.PgBlockingIncident{}
		}
	} else {
		windowHours := 24
		if s := q.Get("window_hours"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 168 {
				windowHours = n
			}
		}
		window := time.Duration(windowHours) * time.Hour
		points, err = h.metricsSvc.GetPgBlockingTimeline(r.Context(), instance, window)
		if err != nil {
			log.Printf("[API] PG locks-blocking timeline error for %s: %v", instance, err)
			points = []hot.PgBlockingTimelinePoint{}
		}
		incs, err = h.metricsSvc.GetPgBlockingIncidentsInWindow(r.Context(), instance, window)
		if err != nil {
			log.Printf("[API] PG locks-blocking incidents error for %s: %v", instance, err)
			incs = []hot.PgBlockingIncident{}
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":  instance,
		"timeline":  points,
		"incidents": incs,
		"source":    "timescale",
	})
}

func (h *PostgresHandlers) LocksBlockingTopLockedTables(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	limit := 10
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	lookbackMin := 10
	if s := r.URL.Query().Get("lookback_minutes"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 180 {
			lookbackMin = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"tables":   []interface{}{},
			"source":   "none",
		})
		return
	}

	q := r.URL.Query()
	fromStr := strings.TrimSpace(q.Get("from"))
	toStr := strings.TrimSpace(q.Get("to"))
	var rows []hot.PgTopLockedTable
	if fromStr != "" && toStr != "" {
		fromT, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid from (use RFC3339)"})
			return
		}
		toT, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid to (use RFC3339)"})
			return
		}
		rows, err = h.metricsSvc.GetPgTopLockedTablesRange(r.Context(), instance, fromT, toT, limit)
		if err != nil {
			log.Printf("[API] PG top locked tables range error for %s: %v", instance, err)
			rows = []hot.PgTopLockedTable{}
		}
	} else {
		var err error
		rows, err = h.metricsSvc.GetPgTopLockedTables(r.Context(), instance, time.Duration(lookbackMin)*time.Minute, limit)
		if err != nil {
			log.Printf("[API] PG top locked tables error for %s: %v", instance, err)
			rows = []hot.PgTopLockedTable{}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"tables":   rows,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) LocksBlockingDetails(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance":      instance,
			"collected_at":  "",
			"blocking_tree": []interface{}{},
			"source":        "none",
		})
		return
	}

	q := r.URL.Query()
	fromStr := strings.TrimSpace(q.Get("from"))
	toStr := strings.TrimSpace(q.Get("to"))
	if fromStr == "" || toStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "from and to are required (RFC3339)"})
		return
	}
	fromT, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid from (use RFC3339)"})
		return
	}
	toT, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid to (use RFC3339)"})
		return
	}

	resp, err := h.metricsSvc.GetPgBlockingDetailsInRange(r.Context(), instance, fromT, toT)
	if err != nil || resp == nil {
		log.Printf("[API] PG locks-blocking details error for %s: %v", instance, err)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance":      instance,
			"collected_at":  "",
			"blocking_tree": []interface{}{},
			"source":        "timescale",
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":      instance,
		"collected_at":  resp.CollectedAt,
		"blocking_tree": resp.BlockingTree,
		"source":        "timescale",
	})
}

func firstErrString(errs ...error) string {
	for _, e := range errs {
		if e != nil {
			return e.Error()
		}
	}
	return ""
}

func (h *PostgresHandlers) Replication(w http.ResponseWriter, r *http.Request) {
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

	stats, err := h.metricsSvc.PgRepo.GetReplicationStats(instance)
	if err != nil {
		log.Printf("[API] PG replication error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"is_primary":        true,
			"local_lag_mb":      0,
			"cluster_state":     "unknown",
			"max_lag_mb":        0,
			"wal_gen_rate_mbps": 0,
			"bg_writer_eff_pct": 0,
			"standbys":          []interface{}{},
		})
		return
	}

	// Determine HA provider from config hint or safe auto-detection.
	var hinted string
	for _, inst := range h.cfg.Instances {
		if inst.Name == instance {
			hinted = inst.HAProvider
			break
		}
	}
	hint := repository.NormalizeHaProvider(hinted)
	clusterName, _ := h.metricsSvc.PgRepo.FetchClusterName(instance)
	var appNames []string
	if stats != nil {
		for _, st := range stats.Standbys {
			appNames = append(appNames, st.ReplicaPodName)
		}
	}
	det := repository.DetectHaProviderAuto(clusterName, appNames, len(appNames) > 0)
	if hint != repository.HaProviderAuto {
		det.Provider = hint
		det.DetectedBy = "config.ha_provider"
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"ha_provider":    det.Provider,
		"ha_detected_by": det.DetectedBy,
		"stats":          stats,
	})
}

func (h *PostgresHandlers) Sessions(w http.ResponseWriter, r *http.Request) {
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

	sessions, err := h.metricsSvc.PgRepo.GetSessions(instance)
	if err != nil {
		log.Printf("[API] PG sessions error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"sessions": []interface{}{}})
		return
	}
	if sessions == nil {
		sessions = []repository.PgSession{}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"sessions": sessions,
	})
}

func (h *PostgresHandlers) KillSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}
	instance := r.URL.Query().Get("instance")
	pidStr := r.URL.Query().Get("pid")
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

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid pid"})
		return
	}

	err = h.metricsSvc.PgRepo.TerminateSession(instance, pid)
	if err != nil {
		log.Printf("[API] PG kill-session error for %s pid %d: %v", instance, pid, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *PostgresHandlers) Locks(w http.ResponseWriter, r *http.Request) {
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

	locks, err := h.metricsSvc.PgRepo.GetLocks(instance)
	if err != nil {
		log.Printf("[API] PG locks error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"locks": []interface{}{}})
		return
	}
	if locks == nil {
		locks = []repository.PgLock{}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"locks":    locks,
	})
}

func (h *PostgresHandlers) BlockingTree(w http.ResponseWriter, r *http.Request) {
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

	// Use pg_blocking_pids()-based tree for better consistency with incident pipeline.
	tree, err := h.metricsSvc.PgRepo.GetBlockingTreeFast(instance)
	if err != nil {
		log.Printf("[API] PG blocking-tree error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"blocking_tree": []interface{}{}})
		return
	}
	if tree == nil {
		tree = []repository.PgBlockingNode{}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":      instance,
		"blocking_tree": tree,
	})
}

func (h *PostgresHandlers) Queries(w http.ResponseWriter, r *http.Request) {
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

	q := r.URL.Query()
	fromStr := strings.TrimSpace(q.Get("from"))
	toStr := strings.TrimSpace(q.Get("to"))
	toT := time.Now().UTC()
	fromT := toT.Add(-1 * time.Hour)
	if fromStr != "" && toStr != "" {
		var perr error
		fromT, perr = time.Parse(time.RFC3339, fromStr)
		if perr != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid from (use RFC3339, e.g. 2026-04-10T12:00:00Z)"})
			return
		}
		toT, perr = time.Parse(time.RFC3339, toStr)
		if perr != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid to (use RFC3339)"})
			return
		}
	}

	queries, meta, err := h.metricsSvc.GetPostgresQueriesForAPI(r.Context(), instance, fromT, toT)
	enabled := err == nil
	if queries == nil {
		queries = []repository.PgQueryStat{}
	}
	if err != nil {
		if strings.Contains(err.Error(), "pg_stat_statements") {
			enabled = false
		}
		log.Printf("[API] PG queries error for %s: %v", instance, err)
	}

	resp := map[string]interface{}{
		"instance":                   instance,
		"queries":                    queries,
		"pg_stat_statements_enabled": enabled,
		"collected_at":               time.Now().UTC(),
	}
	for k, v := range meta {
		resp[k] = v
	}
	if err == nil && meta != nil {
		if ec, ok := meta["end_capture"].(string); ok && ec != "" {
			if t, perr := time.Parse(time.RFC3339, ec); perr == nil {
				resp["collected_at"] = t
			}
		}
	}
	if err != nil {
		resp["error"] = err.Error()
	}

	json.NewEncoder(w).Encode(resp)
}

func (h *PostgresHandlers) ResetQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}
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

	err := h.metricsSvc.PgRepo.ResetQueryStats(instance)
	if err != nil {
		log.Printf("[API] PG reset-queries error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

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
