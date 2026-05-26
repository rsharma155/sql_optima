// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
//
// Purpose: Bridge OS collector push payloads into TimescaleDB (pg_os_* and host_memory_samples).

package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/models"
)

// OSCollectorPayload matches the JSON body emitted by os_collector/sql-optima-os-collector.sh.
type OSCollectorPayload struct {
	Timestamp    time.Time `json:"ts"`
	Hostname     string    `json:"hostname"`
	InstanceName string    `json:"instance_name"`

	TotalBytes     uint64 `json:"total_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	FreeBytes      uint64 `json:"free_bytes"`
	CachedBytes    uint64 `json:"cached_bytes"`
	BuffersBytes   uint64 `json:"buffers_bytes"`
	SharedBytes    uint64 `json:"shared_bytes"`
	SlabBytes      uint64 `json:"slab_bytes"`
	SwapTotalBytes uint64 `json:"swap_total_bytes"`
	SwapUsedBytes  uint64 `json:"swap_used_bytes"`
	SwapFreeBytes  uint64 `json:"swap_free_bytes"`
	DirtyBytes     uint64 `json:"dirty_bytes"`
	WritebackBytes uint64 `json:"writeback_bytes"`

	CPUUserPct   float64 `json:"cpu_user_pct"`
	CPUSystemPct float64 `json:"cpu_system_pct"`
	CPUIdlePct   float64 `json:"cpu_idle_pct"`
	CPUIOWaitPct float64 `json:"cpu_iowait_pct"`
	Load1m       float64 `json:"load_1m"`
	Load5m       float64 `json:"load_5m"`
	Load15m      float64 `json:"load_15m"`
	CPUCores     int     `json:"cpu_cores"`

	PostgresRSS     uint64 `json:"postgres_rss_bytes"`
	PostgresVSZ     uint64 `json:"postgres_vsz_bytes"`
	PostgresShared  uint64 `json:"postgres_shared_bytes"`
	PostgresPrivate uint64 `json:"postgres_private_bytes"`
	BackendCount    int    `json:"backend_count"`
}

func (p *OSCollectorPayload) toMetricsMap() map[string]interface{} {
	if p.Timestamp.IsZero() {
		p.Timestamp = time.Now().UTC()
	}
	return map[string]interface{}{
		"ts":                 p.Timestamp,
		"hostname":           p.Hostname,
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

func bytesToMB(v uint64) int64 {
	if v == 0 {
		return 0
	}
	return int64(v / (1024 * 1024))
}

// IngestOSCollectorPayload resolves instance_name to server_id, persists pg_os_* samples,
// and mirrors host RAM into monitor.host_memory_samples for the PG memory dashboard.
func (s *MetricsService) IngestOSCollectorPayload(ctx context.Context, p *OSCollectorPayload) error {
	if s == nil || s.tsLogger == nil || p == nil {
		return fmt.Errorf("os metrics ingest not configured")
	}
	if p.InstanceName == "" {
		return fmt.Errorf("instance_name is required")
	}
	serverID, err := s.ResolveServerIDByInstanceName(ctx, p.InstanceName)
	if err != nil {
		return err
	}

	metrics := p.toMetricsMap()
	hostname := p.Hostname
	if hostname == "" {
		hostname = p.InstanceName
	}
	if err := s.tsLogger.SaveOSMetrics(ctx, hostname, serverID, metrics); err != nil {
		return err
	}

	hostSnap := &models.PgHostMemorySnapshot{
		ServerID:         serverID,
		Timestamp:        p.Timestamp,
		TotalMemoryMB:    bytesToMB(p.TotalBytes),
		UsedMemoryMB:     bytesToMB(p.UsedBytes),
		FreeMemoryMB:     bytesToMB(p.FreeBytes),
		CachedMemoryMB:   bytesToMB(p.CachedBytes),
		BufferedMemoryMB: bytesToMB(p.BuffersBytes),
		SwapTotalMB:      bytesToMB(p.SwapTotalBytes),
		SwapUsedMB:       bytesToMB(p.SwapUsedBytes),
	}
	return s.tsLogger.LogPgHostMemory(ctx, hostSnap)
}

// ResolveServerIDByInstanceName returns the server UUID for a monitoring instance name
// (config.yaml entry or active row in optima_servers).
func (s *MetricsService) ResolveServerIDByInstanceName(ctx context.Context, name string) (uuid.UUID, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return uuid.Nil, fmt.Errorf("instance name is required")
	}
	if s != nil && s.Config != nil {
		for _, inst := range s.Config.Instances {
			if inst.Name == name {
				if inst.ServerID == uuid.Nil {
					return uuid.Nil, fmt.Errorf("instance %q has no server_id", name)
				}
				return inst.ServerID, nil
			}
		}
	}
	if s != nil && s.ServerRepo != nil {
		srv, err := s.ServerRepo.GetByName(ctx, name)
		if err == nil && srv.ID != uuid.Nil {
			return srv.ID, nil
		}
	}
	return uuid.Nil, fmt.Errorf("unknown instance %q", name)
}

// GetOSCollectorStatus reports API ingest flag and whether host memory samples were received recently.
// Unknown instance names (e.g. while typing in the add-server form) return registered=false without an error.
func (s *MetricsService) GetOSCollectorStatus(ctx context.Context, instanceName string) (map[string]interface{}, error) {
	info := s.OSMetricsIngestInfo(ctx)
	base := map[string]interface{}{
		"instance_name":           instanceName,
		"os_collector_configured": false,
		"registered":              false,
		"ingest_enabled":          info.Enabled,
		"ingest_source":           info.Source,
		"ingest_configurable":     info.Configurable,
	}

	serverID, err := s.ResolveServerIDByInstanceName(ctx, instanceName)
	if err != nil {
		if strings.Contains(err.Error(), "unknown instance") {
			return base, nil
		}
		return nil, err
	}
	base["registered"] = true
	base["server_id"] = serverID.String()

	configured := false
	if s != nil && s.tsLogger != nil {
		configured, err = s.tsLogger.HasRecentHostMemorySamples(ctx, serverID, 20*time.Minute)
		if err != nil {
			return nil, err
		}
	}
	base["os_collector_configured"] = configured
	return base, nil
}
