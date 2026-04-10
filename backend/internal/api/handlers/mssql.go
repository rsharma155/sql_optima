package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

type MssqlHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewMssqlHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *MssqlHandlers {
	return &MssqlHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

func (h *MssqlHandlers) Overview(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.metricsSvc.GetMssqlOverview(instance))
}

func (h *MssqlHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	source := r.URL.Query().Get("source")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if source == "" || source == "live" {
		w.Header().Set("X-Data-Source", "live_cache")
		json.NewEncoder(w).Encode(h.metricsSvc.GetCachedDashboard(instance))
		return
	}

	tsData, err := h.metricsSvc.GetDashboardFromTimescale(instance)
	if err != nil {
		log.Printf("[Router] TimescaleDB fetch failed for %s, falling back to cache: %v", instance, err)
		w.Header().Set("X-Data-Source", "live_cache_fallback")
		json.NewEncoder(w).Encode(h.metricsSvc.GetCachedDashboard(instance))
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(tsData)
}

// DashboardV2 returns the Phase-1 DBA homepage payload.
// It is intentionally cached-only in Phase-1 to keep latency low and behavior predictable.
func (h *MssqlHandlers) DashboardV2(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	out, src := h.metricsSvc.GetDashboardHomepageV2WithSource(instance)
	w.Header().Set("X-Data-Source", src)
	json.NewEncoder(w).Encode(out)
}

// PerformanceDebt returns maintenance/risk findings collected into TimescaleDB (hourly snapshots).
func (h *MssqlHandlers) PerformanceDebt(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if !h.metricsSvc.IsTimescaleConnected() {
		w.Header().Set("X-Data-Source", "timescale_unavailable")
		json.NewEncoder(w).Encode(map[string]any{
			"findings": []any{},
		})
		return
	}

	lookback := 2 // hours
	if v := r.URL.Query().Get("lookback_hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 168 {
			lookback = n
		}
	}

	out, err := h.metricsSvc.GetTimescalePerformanceDebtFindings(r.Context(), instance, time.Duration(lookback)*time.Hour)
	if err != nil {
		w.Header().Set("X-Data-Source", "timescale_error")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to load performance debt findings"})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]any{
		"findings": out,
	})
}

func (h *MssqlHandlers) Jobs(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// Jobs currently query MSDB live (not Timescale-backed yet).
	w.Header().Set("X-Data-Source", "live_dmv")
	json.NewEncoder(w).Encode(h.metricsSvc.MsRepo.FetchAgentJobs(instance))
}

func (h *MssqlHandlers) XEvents(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	events, err := h.metricsSvc.GetRecentXEvents(instance)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to retrieve extended events"})
		return
	}

	json.NewEncoder(w).Encode(events)
}

func (h *MssqlHandlers) BestPractices(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	result := h.metricsSvc.GetBestPractices(instance)
	json.NewEncoder(w).Encode(result)
}

func (h *MssqlHandlers) Guardrails(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	result := h.metricsSvc.GetGuardrails(instance)
	json.NewEncoder(w).Encode(result)
}

func (h *MssqlHandlers) CPUDrilldown(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	limitStr := r.URL.Query().Get("limit")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	queries, err := h.metricsSvc.MsRepo.FetchTopCPUQueries(instance, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"queries": queries, "count": len(queries)})
}

func (h *MssqlHandlers) AGHealth(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	if !instanceType(h.cfg, instance, "sqlserver") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not sqlserver"})
		return
	}

	stats, err := h.metricsSvc.MsRepo.FetchAGHealthStats(instance)
	if err != nil {
		log.Printf("[Router] AG Health error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"hadr_enabled": false,
			"ag_health":    []interface{}{},
			"ag_stats":     []interface{}{}, // backward/forward compat with frontend
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hadr_enabled": len(stats) > 0,
		"ag_health":    stats,
		"ag_stats":     stats, // backward/forward compat with frontend
	})
}

func (h *MssqlHandlers) DBThroughput(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	stats, err := h.metricsSvc.MsRepo.FetchDatabaseThroughput(instance)
	if err != nil {
		log.Printf("[Router] DB Throughput error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"db_throughput": []interface{}{},
			"db_stats":      []interface{}{}, // compat with frontend
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"db_throughput": stats,
		"db_stats":      stats, // compat with frontend
	})
}

func (h *MssqlHandlers) LatchStats(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		if stats, err := h.metricsSvc.GetTimescaleLatchWaits(r.Context(), instance, 50); err == nil && stats != nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"latch_stats": stats})
			return
		}
	}
	stats, err := h.metricsSvc.MsRepo.FetchLatchStats(instance)
	if err != nil {
		log.Printf("[Router] Latch stats error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"latch_stats": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"latch_stats": stats})
}

func (h *MssqlHandlers) WaitingTasks(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		if stats, err := h.metricsSvc.GetTimescaleWaitingTasks(r.Context(), instance, 50); err == nil && stats != nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"waiting_tasks": stats})
			return
		}
	}
	stats, err := h.metricsSvc.MsRepo.FetchWaitingTasks(instance)
	if err != nil {
		log.Printf("[Router] Waiting tasks error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"waiting_tasks": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"waiting_tasks": stats})
}

func (h *MssqlHandlers) MemoryGrants(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		if stats, err := h.metricsSvc.GetTimescaleMemoryGrants(r.Context(), instance, 50); err == nil && stats != nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"memory_grants": stats})
			return
		}
	}
	stats, err := h.metricsSvc.MsRepo.FetchMemoryGrants(instance)
	if err != nil {
		log.Printf("[Router] Memory grants error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"memory_grants": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"memory_grants": stats})
}

func (h *MssqlHandlers) SchedulerWorkers(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		if stats, err := h.metricsSvc.GetTimescaleSchedulerWG(r.Context(), instance, 50); err == nil && stats != nil {
			w.Header().Set("X-Data-Source", "timescale")
			// Return both keys for compatibility.
			json.NewEncoder(w).Encode(map[string]interface{}{
				"scheduler_wg":      stats,
				"scheduler_workers": stats,
			})
			return
		}
	}

	stats, err := h.metricsSvc.MsRepo.FetchSchedulerWG(instance)
	if err != nil {
		log.Printf("[Router] Scheduler worker stats error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"scheduler_wg":      []interface{}{},
			"scheduler_workers": []interface{}{},
		})
		return
	}

	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"scheduler_wg":      stats,
		"scheduler_workers": stats,
	})
}

func (h *MssqlHandlers) ProcedureStats(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		if stats, err := h.metricsSvc.GetTimescaleProcedureStats(r.Context(), instance, 50); err == nil && stats != nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"procedure_stats": stats})
			return
		}
	}
	stats, err := h.metricsSvc.MsRepo.FetchProcedureStats(instance)
	if err != nil {
		log.Printf("[Router] Procedure stats error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"procedure_stats": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"procedure_stats": stats})
}

func (h *MssqlHandlers) FileIOLatency(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		if stats, err := h.metricsSvc.GetTimescaleFileIOLatency(r.Context(), instance, 50); err == nil && stats != nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"file_io_latency": stats})
			return
		}
	}
	stats, err := h.metricsSvc.MsRepo.FetchFileIOLatency(instance)
	if err != nil {
		log.Printf("[Router] File IO latency error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"file_io_latency": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"file_io_latency": stats})
}

func (h *MssqlHandlers) SpinlockStats(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		if stats, err := h.metricsSvc.GetTimescaleSpinlockStats(r.Context(), instance, 50); err == nil && stats != nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"spinlock_stats": stats})
			return
		}
	}
	stats, err := h.metricsSvc.MsRepo.FetchSpinlockStats(instance)
	if err != nil {
		log.Printf("[Router] Spinlock stats error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"spinlock_stats": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"spinlock_stats": stats})
}

func (h *MssqlHandlers) MemoryClerks(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		if stats, err := h.metricsSvc.GetTimescaleMemoryClerks(r.Context(), instance, 50); err == nil && stats != nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"memory_clerks": stats})
			return
		}
	}
	stats, err := h.metricsSvc.MsRepo.FetchMemoryClerks(instance)
	if err != nil {
		log.Printf("[Router] Memory clerks error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"memory_clerks": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"memory_clerks": stats})
}

func (h *MssqlHandlers) TempdbStats(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// This endpoint returns file-level stats; prefer TimescaleDB-backed snapshots when available.
	if h.metricsSvc.IsTimescaleConnected() {
		if stats, err := h.metricsSvc.GetTimescaleTempdbFiles(r.Context(), instance, 50); err == nil && stats != nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]interface{}{"tempdb_stats": stats})
			return
		}
	}
	stats, err := h.metricsSvc.MsRepo.FetchTempdbStats(instance)
	if err != nil {
		log.Printf("[Router] Tempdb stats error: %v", err)
		w.Header().Set("X-Data-Source", "live_dmv_error")
		json.NewEncoder(w).Encode(map[string]interface{}{"tempdb_stats": []interface{}{}})
		return
	}
	w.Header().Set("X-Data-Source", "live_dmv_fallback")
	json.NewEncoder(w).Encode(map[string]interface{}{"tempdb_stats": stats})
}

func (h *MssqlHandlers) PlanCacheHealth(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		if rows, err := h.metricsSvc.GetTimescalePlanCacheHealth(r.Context(), instance, 60); err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]any{"plan_cache_health": rows})
			return
		}
	}

	w.Header().Set("X-Data-Source", "timescale_unavailable")
	json.NewEncoder(w).Encode(map[string]any{"plan_cache_health": []any{}})
}

func (h *MssqlHandlers) MemoryGrantWaiters(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		if rows, err := h.metricsSvc.GetTimescaleMemoryGrantWaiters(r.Context(), instance, 50); err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]any{"memory_grant_waiters": rows})
			return
		}
	}

	w.Header().Set("X-Data-Source", "timescale_unavailable")
	json.NewEncoder(w).Encode(map[string]any{"memory_grant_waiters": []any{}})
}

func (h *MssqlHandlers) TempdbTopConsumers(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		if rows, err := h.metricsSvc.GetTimescaleTempdbTopConsumers(r.Context(), instance, 50); err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]any{"tempdb_top_consumers": rows})
			return
		}
	}

	w.Header().Set("X-Data-Source", "timescale_unavailable")
	json.NewEncoder(w).Encode(map[string]any{"tempdb_top_consumers": []any{}})
}

func (h *MssqlHandlers) WaitCategories(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		if rows, err := h.metricsSvc.GetTimescaleWaitCategoryAgg(r.Context(), instance, 15); err == nil {
			w.Header().Set("X-Data-Source", "timescale")
			json.NewEncoder(w).Encode(map[string]any{"wait_categories_15m": rows})
			return
		}
	}
	w.Header().Set("X-Data-Source", "timescale_unavailable")
	json.NewEncoder(w).Encode(map[string]any{"wait_categories_15m": []any{}})
}

func (h *MssqlHandlers) CPUSchedulerStats(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	stats, err := h.metricsSvc.GetTimescaleCPUSchedulerStats(instance, 50)
	if err != nil {
		log.Printf("[Router] CPU Scheduler stats error: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{"cpu_scheduler_stats": []interface{}{}})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"cpu_scheduler_stats": stats})
}

func (h *MssqlHandlers) ServerProperties(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	props, err := h.metricsSvc.GetTimescaleServerProperties(instance)
	if err != nil {
		log.Printf("[Router] Server properties error: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{"server_properties": nil})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"server_properties": props})
}
