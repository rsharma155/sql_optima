// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : HTTP handlers for the SQL Server HA & Replication monitoring
//           domain.  All endpoints are read-only and scoped by server_id.
//
//           Data flow:
//             1. Parse server_id (UUID) from query param.
//             2. Check feature gate — if both HA and Replication are disabled,
//                return the feature-status payload immediately (page hidden).
//             3. Query TimescaleDB repository (pre-aggregated, fast).
//             4. Apply domain calculations (RPO/RTO/readiness) where needed.
//             5. Return JSON.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/api/handlers"
	"github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/domain"
	"github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/repository"
	"github.com/rsharma155/sql_optima/internal/service"
)

// HAReplicationHandler serves all HA & Replication dashboard endpoints.
type HAReplicationHandler struct {
	metricsSvc *service.MetricsService
	repo       *repository.HAReplicationRepository
}

// NewHAReplicationHandler constructs the handler.  If TimescaleDB is
// unavailable (pool == nil) all endpoints return 503.
func NewHAReplicationHandler(svc *service.MetricsService) *HAReplicationHandler {
	h := &HAReplicationHandler{metricsSvc: svc}
	if pool := svc.GetTimescaleDBPool(); pool != nil {
		h.repo = repository.NewHAReplicationRepository(pool)
	}
	return h
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func (h *HAReplicationHandler) writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (h *HAReplicationHandler) writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (h *HAReplicationHandler) serverID(r *http.Request) (uuid.UUID, bool) {
	id, ok := handlers.ParseServerID(r, h.metricsSvc.Config)
	return id, ok
}

func (h *HAReplicationHandler) repoReady(w http.ResponseWriter) bool {
	if h.repo == nil {
		h.writeError(w, http.StatusServiceUnavailable,
			"TimescaleDB not connected — HA metrics unavailable")
		return false
	}
	return true
}

// parseTimeRange extracts from/to query params, defaulting to last 24 hours.
func parseTimeRange(r *http.Request) (time.Time, time.Time) {
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	to := now

	if s := r.URL.Query().Get("from"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			from = t.UTC()
		}
	}
	if s := r.URL.Query().Get("to"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			to = t.UTC()
		}
	}
	return from, to
}

// ---------------------------------------------------------------------------
// GET /api/sqlserver/ha/feature-status
// ---------------------------------------------------------------------------

// FeatureStatus returns the feature gate result.  The frontend calls this
// first; if both flags are false it hides the entire page.
//
// Optional query param: database=<name>
// When provided, returns per-database HA/replication flags instead of
// instance-level flags, enabling per-database sidebar link visibility.
func (h *HAReplicationHandler) FeatureStatus(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.serverID(r)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "server_id required")
		return
	}
	if h.repo == nil {
		h.writeJSON(w, domain.FeatureDetection{HAEnabled: false, ReplicationEnabled: false})
		return
	}
	database := r.URL.Query().Get("database")
	feat, err := h.repo.GetFeatureDetection(r.Context(), serverID, database)
	if err != nil {
		h.writeJSON(w, domain.FeatureDetection{HAEnabled: false, ReplicationEnabled: false})
		return
	}
	h.writeJSON(w, feat)
}

// ---------------------------------------------------------------------------
// GET /api/sqlserver/ha/dashboard-summary
// ---------------------------------------------------------------------------

// DashboardSummary returns a single aggregated payload covering feature gate,
// banner, KPI cards and failover readiness.  Designed for the page's initial
// load — one round-trip instead of six.
func (h *HAReplicationHandler) DashboardSummary(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.serverID(r)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "server_id required")
		return
	}
	if !h.repoReady(w) {
		return
	}
	ctx := r.Context()

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	from, to := parseTimeRange(r)

	feat, _ := h.repo.GetFeatureDetection(ctx, serverID, "")
	replicas, _ := h.repo.GetCurrentReplicaHealth(ctx, serverID)
	dbStates, _ := h.repo.GetCurrentDatabaseSyncState(ctx, serverID)

	longRunningCount := 0
	quorumState := ""
	if len(replicas) > 0 {
		longRunningCount = replicas[0].LongRunningTxCount
		quorumState = replicas[0].QuorumStateDesc
	}

	rpo := domain.ComputeRPO(replicas)
	rto := domain.ComputeRTO(replicas)
	readiness := domain.EvaluateFailoverReadiness(replicas, dbStates, longRunningCount, quorumState)

	// Fetch failover history for the selected range
	history, _ := h.repo.GetFailoverHistory(ctx, serverID, from, to)
	var lastFailover *domain.FailoverEvent
	if len(history) > 0 {
		lastFailover = &history[0]
	}

	rpoWorst := h.repo.GetRPOWorstRange(ctx, serverID, fromStr, toStr)

	var repKPIs *domain.ReplicationKPIs
	if feat.ReplicationEnabled {
		rk, _ := h.repo.GetReplicationKPIs(ctx, serverID)
		repKPIs = &rk
	}

	kpis := buildKPICards(replicas, rpo, rto, readiness, lastFailover, rpoWorst, repKPIs)
	banner := buildBanner(feat, kpis, rpo)

	summary := domain.DashboardSummary{
		Feature:           feat,
		Banner:            banner,
		KPIs:              kpis,
		FailoverReadiness: readiness,
	}
	w.Header().Set("X-Data-Source", "timescale")
	h.writeJSON(w, summary)
}

// ---------------------------------------------------------------------------
// GET /api/sqlserver/ha/rpo/trend
// ---------------------------------------------------------------------------

// RPOTrend returns the per-minute RPO trend for charting (default: last 24 h).
func (h *HAReplicationHandler) RPOTrend(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.serverID(r)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "server_id required")
		return
	}
	if !h.repoReady(w) {
		return
	}
	from, to := parseTimeRange(r)
	points, err := h.repo.GetRPOTrend(r.Context(), serverID, from, to)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "rpo trend query failed")
		return
	}
	h.writeJSON(w, map[string]interface{}{
		"from":  from,
		"to":    to,
		"data":  points,
		"xAxis": map[string]string{"label": "Time", "type": "datetime"},
		"yAxis": map[string]string{"label": "Data Loss Risk (seconds)", "unit": "s"},
	})
}

// ---------------------------------------------------------------------------
// GET /api/sqlserver/ha/rto/trend
// ---------------------------------------------------------------------------

// RTOTrend returns the per-minute RTO estimate trend (default: last 24 h).
func (h *HAReplicationHandler) RTOTrend(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.serverID(r)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "server_id required")
		return
	}
	if !h.repoReady(w) {
		return
	}
	from, to := parseTimeRange(r)
	points, err := h.repo.GetRTOTrend(r.Context(), serverID, from, to)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "rto trend query failed")
		return
	}
	h.writeJSON(w, map[string]interface{}{
		"from":  from,
		"to":    to,
		"data":  points,
		"xAxis": map[string]string{"label": "Time", "type": "datetime"},
		"yAxis": map[string]string{"label": "Est. Failover Time (seconds)", "unit": "s"},
	})
}

// ---------------------------------------------------------------------------
// GET /api/sqlserver/ha/replicas
// ---------------------------------------------------------------------------

// ReplicaHealth returns the current replica health table rows.
func (h *HAReplicationHandler) ReplicaHealth(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.serverID(r)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "server_id required")
		return
	}
	if !h.repoReady(w) {
		return
	}
	replicas, err := h.repo.GetCurrentReplicaHealth(r.Context(), serverID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "replica health query failed")
		return
	}
	h.writeJSON(w, map[string]interface{}{"replicas": replicas})
}

// ---------------------------------------------------------------------------
// GET /api/sqlserver/ha/failover-readiness
// ---------------------------------------------------------------------------

// FailoverReadiness returns the checklist readiness assessment.
func (h *HAReplicationHandler) FailoverReadiness(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.serverID(r)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "server_id required")
		return
	}
	if !h.repoReady(w) {
		return
	}
	ctx := r.Context()
	replicas, _ := h.repo.GetCurrentReplicaHealth(ctx, serverID)
	dbStates, _ := h.repo.GetCurrentDatabaseSyncState(ctx, serverID)

	longRunningCount := 0
	quorumState := ""
	if len(replicas) > 0 {
		longRunningCount = replicas[0].LongRunningTxCount
		quorumState = replicas[0].QuorumStateDesc
	}

	readiness := domain.EvaluateFailoverReadiness(replicas, dbStates, longRunningCount, quorumState)
	h.writeJSON(w, readiness)
}

// ---------------------------------------------------------------------------
// GET /api/sqlserver/ha/failover-history
// ---------------------------------------------------------------------------

// FailoverHistory returns the failover event timeline.
func (h *HAReplicationHandler) FailoverHistory(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.serverID(r)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "server_id required")
		return
	}
	if !h.repoReady(w) {
		return
	}
	from, to := parseTimeRange(r)
	events, err := h.repo.GetFailoverHistory(r.Context(), serverID, from, to)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failover history query failed")
		return
	}
	h.writeJSON(w, map[string]interface{}{"events": events})
}

// ---------------------------------------------------------------------------
// GET /api/sqlserver/ha/replication/kpis
// ---------------------------------------------------------------------------

// ReplicationKPIs returns aggregate replication counts.
func (h *HAReplicationHandler) ReplicationKPIs(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.serverID(r)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "server_id required")
		return
	}
	if !h.repoReady(w) {
		return
	}
	kpis, err := h.repo.GetReplicationKPIs(r.Context(), serverID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "replication kpis query failed")
		return
	}
	h.writeJSON(w, kpis)
}

// ---------------------------------------------------------------------------
// GET /api/sqlserver/ha/replication/topology
// ---------------------------------------------------------------------------

// ReplicationTopology returns the pub→sub topology table rows.
func (h *HAReplicationHandler) ReplicationTopology(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.serverID(r)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "server_id required")
		return
	}
	if !h.repoReady(w) {
		return
	}
	rows, err := h.repo.GetReplicationTopology(r.Context(), serverID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "replication topology query failed")
		return
	}
	h.writeJSON(w, map[string]interface{}{"topology": rows})
}

// ---------------------------------------------------------------------------
// GET /api/sqlserver/ha/replication/backlog/trend
// ---------------------------------------------------------------------------

// ReplicationBacklogTrend returns the per-minute backlog/latency trend.
func (h *HAReplicationHandler) ReplicationBacklogTrend(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.serverID(r)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "server_id required")
		return
	}
	if !h.repoReady(w) {
		return
	}
	from, to := parseTimeRange(r)
	points, err := h.repo.GetReplicationBacklogTrend(r.Context(), serverID, from, to)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "backlog trend query failed")
		return
	}
	h.writeJSON(w, map[string]interface{}{"from": from, "to": to, "data": points})
}

// ---------------------------------------------------------------------------
// GET /api/sqlserver/ha/replication/articles
// ---------------------------------------------------------------------------

// ReplicationArticles returns the replicated tables grid.
func (h *HAReplicationHandler) ReplicationArticles(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.serverID(r)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "server_id required")
		return
	}
	if !h.repoReady(w) {
		return
	}
	articles, err := h.repo.GetReplicationArticles(r.Context(), serverID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "articles query failed")
		return
	}
	h.writeJSON(w, map[string]interface{}{"articles": articles})
}

// GET /api/sqlserver/ha/alerts-timeline
// ---------------------------------------------------------------------------

// AlertsTimeline returns a merged list of HA and Replication events.
func (h *HAReplicationHandler) AlertsTimeline(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.serverID(r)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "server_id required")
		return
	}
	if !h.repoReady(w) {
		return
	}
	from, to := parseTimeRange(r)
	alerts, err := h.repo.GetAlertsTimeline(r.Context(), serverID, from, to)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "alerts timeline query failed")
		return
	}
	h.writeJSON(w, map[string]interface{}{"from": from, "to": to, "data": alerts})
}

// ---------------------------------------------------------------------------
// GET /api/sqlserver/ha/database-coverage
// ---------------------------------------------------------------------------

// DatabaseCoverage returns the Gold/Silver/Bronze/Red coverage panel.
func (h *HAReplicationHandler) DatabaseCoverage(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.serverID(r)
	if !ok {
		h.writeError(w, http.StatusBadRequest, "server_id required")
		return
	}
	if !h.repoReady(w) {
		return
	}
	coverage, err := h.repo.GetDatabaseCoverage(r.Context(), serverID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "database coverage query failed")
		return
	}
	h.writeJSON(w, map[string]interface{}{"coverage": coverage})
}

// ---------------------------------------------------------------------------
// Domain helpers used by DashboardSummary
// ---------------------------------------------------------------------------

func buildKPICards(
	replicas []domain.ReplicaHealthRow,
	rpo domain.RPOResult,
	rto domain.RTOResult,
	readiness domain.FailoverReadiness,
	lastFailover *domain.FailoverEvent,
	rpoWorst24h int64,
	repKPIs *domain.ReplicationKPIs,
) domain.KPICards {
	kpis := domain.KPICards{
		RPOSeconds:    rpo.Seconds,
		RPOWorst24h:   rpoWorst24h,
		RPOThreshold:  rpo.Threshold,
		EstRTOSeconds: rto.EstimatedSeconds,
		RTOThreshold:  rto.Threshold,
		FailoverReady: readiness.Ready,
	}

	if repKPIs != nil && len(repKPIs.TypeBreakdown) > 0 {
		var parts []string
		for k, v := range repKPIs.TypeBreakdown {
			parts = append(parts, fmt.Sprintf("%s: %d", k, v))
		}
		kpis.ReplicationSummary = strings.Join(parts, ", ")
	}

	if lastFailover != nil {
		kpis.LastFailoverAt = &lastFailover.CaptureTimestamp
		kpis.LastFailoverSecs = lastFailover.FailoverDurationSeconds
		uptime := time.Since(lastFailover.CaptureTimestamp)
		kpis.UptimeHuman = formatDuration(uptime)
	} else {
		kpis.UptimeHuman = "> 30d" // Fallback if no failover recorded
	}

	syncModes := map[string]bool{}
	for _, r := range replicas {
		if r.IsPrimary() {
			kpis.PrimaryServer = r.ReplicaServerName
		} else {
			kpis.TotalReplicas++
			if r.IsHealthy() && r.IsConnected() {
				kpis.HealthyReplicas++
			}
		}
		syncModes[r.AvailabilityModeDesc] = true
	}

	switch {
	case syncModes["SYNCHRONOUS_COMMIT"] && syncModes["ASYNCHRONOUS_COMMIT"]:
		kpis.SyncMode = "Mixed"
	case syncModes["SYNCHRONOUS_COMMIT"]:
		kpis.SyncMode = "Synchronous"
	case syncModes["ASYNCHRONOUS_COMMIT"]:
		kpis.SyncMode = "Asynchronous"
	default:
		kpis.SyncMode = "Unknown"
	}
	return kpis
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	return fmt.Sprintf("%dh %dm", hours, int(d.Minutes())%60)
}

func buildBanner(feat domain.FeatureDetection, kpis domain.KPICards, rpo domain.RPOResult) domain.BannerInfo {
	color := "green"
	switch {
	case !kpis.FailoverReady || kpis.HealthyReplicas < kpis.TotalReplicas:
		color = "red"
	case rpo.Threshold == domain.RPOYellow || rpo.Threshold == domain.RPORed:
		color = "yellow"
	}

	text := ""
	if feat.HAEnabled {
		primary := kpis.PrimaryServer
		if primary == "" {
			primary = "Unknown"
		}
		text += "Primary: " + primary
		text += " | Replicas: " + itoa(kpis.HealthyReplicas) + "/" + itoa(kpis.TotalReplicas) + " healthy"
		text += " | RPO: " + itoa64(kpis.RPOSeconds) + "s"
		text += " | Est. RTO: ~" + itoa64(kpis.EstRTOSeconds) + "s"
	}
	if feat.ReplicationEnabled {
		if text != "" {
			text += " | "
		}
		text += "Replication Enabled"
	}
	if !feat.HAEnabled && feat.ReplicationEnabled {
		text += " — No HA configured (DATA LOSS RISK)"
		color = "yellow"
	}

	return domain.BannerInfo{Text: text, Color: color}
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func itoa64(n int64) string {
	return fmt.Sprintf("%d", n)
}
