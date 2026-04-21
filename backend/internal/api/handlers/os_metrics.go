package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/rsharma155/sql_optima/internal/service"
)

type OSMetricsHandler struct {
	metricsSvc *service.MetricsService
}

func NewOSMetricsHandler(svc *service.MetricsService) *OSMetricsHandler {
	return &OSMetricsHandler{metricsSvc: svc}
}

type OSMetricsPayload struct {
	Timestamp    time.Time `json:"ts"`
	Hostname     string    `json:"hostname"`
	InstanceName string    `json:"instance_name"`

	// Memory
	TotalBytes     uint64 `json:"total_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	FreeBytes      uint64 `json:"free_bytes"`
	CachedBytes    uint64 `json:"cached_bytes"`
	BuffersBytes   uint64 `json:"buffers_bytes"`
	SharedBytes    uint64 `json:"shared_bytes"`
	SlabBytes      uint64 `json:"slab_bytes"`
	SwapTotalBytes uint64 `json:"swap_total_bytes"`
	SwapUsedBytes  uint64 `json:"swap_used_mb"` // matches payload if they used different names
	SwapFreeBytes  uint64 `json:"swap_free_bytes"`
	DirtyBytes     uint64 `json:"dirty_bytes"`
	WritebackBytes uint64 `json:"writeback_bytes"`
	PageFaults     uint64 `json:"page_faults"`
	MajPageFaults  uint64 `json:"major_page_faults"`
	OOMKills       uint64 `json:"oom_kill_count"`

	// CPU
	CPUUserPct   float64 `json:"cpu_user_pct"`
	CPUSystemPct float64 `json:"cpu_system_pct"`
	CPUIdlePct   float64 `json:"cpu_idle_pct"`
	CPUIOWaitPct float64 `json:"cpu_iowait_pct"`
	Load1m       float64 `json:"load_1m"`
	Load5m       float64 `json:"load_5m"`
	Load15m      float64 `json:"load_15m"`
	CPUCores     int     `json:"cpu_cores"`

	// Postgres Process
	PostgresRSS     uint64 `json:"postgres_rss_bytes"`
	PostgresVSZ     uint64 `json:"postgres_vsz_bytes"`
	PostgresShared  uint64 `json:"postgres_shared_bytes"`
	PostgresPrivate uint64 `json:"postgres_private_bytes"`
	BackendCount    int    `json:"backend_count"`
}

func (h *OSMetricsHandler) ReceiveMetrics(w http.ResponseWriter, r *http.Request) {
	var p OSMetricsPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// In a real app, validate API Key here

	err := h.metricsSvc.SaveOSMetrics(r.Context(), p.Hostname, p.InstanceName, p.convert())
	if err != nil {
		log.Printf("[OSMetricsHandler] Failed to save metrics: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// Map the payload to a service/model structure if needed, or just pass it through
func (p *OSMetricsPayload) convert() map[string]interface{} {
	// Simple mapping for now, could use a proper struct
	return map[string]interface{}{
		"ts":                 p.Timestamp,
		"hostname":           p.Hostname,
		"instance_name":      p.InstanceName,
		"total_bytes":        p.TotalBytes,
		"available_bytes":    p.AvailableBytes,
		"used_bytes":         p.UsedBytes,
		"free_bytes":         p.FreeBytes,
		"cached_bytes":       p.CachedBytes,
		"buffers_bytes":      p.BuffersBytes,
		"shared_bytes":       p.SharedBytes,
		"slab_bytes":         p.SlabBytes,
		"swap_total_bytes":   p.SwapTotalBytes,
		"swap_used_bytes":    p.SwapUsedBytes,
		"swap_free_bytes":    p.SwapFreeBytes,
		"dirty_bytes":        p.DirtyBytes,
		"writeback_bytes":    p.WritebackBytes,
		"page_faults":        p.PageFaults,
		"major_page_faults":  p.MajPageFaults,
		"oom_kill_count":     p.OOMKills,
		"cpu_user_pct":       p.CPUUserPct,
		"cpu_system_pct":     p.CPUSystemPct,
		"cpu_idle_pct":       p.CPUIdlePct,
		"cpu_iowait_pct":     p.CPUIOWaitPct,
		"load_1m":            p.Load1m,
		"load_5m":            p.Load5m,
		"load_15m":           p.Load15m,
		"cpu_cores":          p.CPUCores,
		"postgres_rss_bytes": p.PostgresRSS,
		"postgres_vsz_bytes": p.PostgresVSZ,
		"backend_count":      p.BackendCount,
	}
}
