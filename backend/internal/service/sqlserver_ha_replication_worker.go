// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : Background worker for SQL Server HA & Replication monitoring.
//           Orchestrates feature detection, AG health, and replication collectors.
//           Uses dynamic frequencies from optima_collector_configs.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package service

import (
	"log/slog"
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/collectors"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// StartSqlServerHAReplicationCollector begins the background collection loop.
func (s *MetricsService) StartSqlServerHAReplicationCollector(ctx context.Context) {
	if s.tsLogger == nil {
		slog.Info("[HA] TimescaleDB logger unavailable — HA/Replication collector disabled")
		return
	}
	slog.Info("[HA-Collector] starting background worker")

	interval := s.GetCollectorInterval(ctx, "sqlserver_ha_replication", 30*time.Second)
	slog.Info("[HA-Collector] orchestration interval", "val", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Track last run time per collector per server
	lastDiscovery   := make(map[uuid.UUID]time.Time)
	lastHealth      := make(map[uuid.UUID]time.Time)
	lastPerf        := make(map[uuid.UUID]time.Time)
	lastTopo        := make(map[uuid.UUID]time.Time)
	lastLogShipping := make(map[uuid.UUID]time.Time)

	// Track if HA/Replication is enabled per server (cached from discovery)
	haEnabledMap := make(map[uuid.UUID]bool)
	replEnabledMap := make(map[uuid.UUID]bool)

	// Sub-collectors are rebuilt whenever RegistryReload hot-swaps s.MsRepo.
	var activeRepo *repository.SqlServerRepository
	var sourceQuerier *collectors.SourceQuerier
	var featDetector *collectors.FeatureDetector
	var haCollector *collectors.HAReplicaCollector
	var replCollector *collectors.ReplicationCollector

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Rebuild sub-collectors if the repository was replaced by RegistryReload.
			if s.MsRepo != activeRepo {
				activeRepo = s.MsRepo
				sourceQuerier = collectors.NewSourceQuerier(activeRepo)
				featDetector = collectors.NewFeatureDetector(sourceQuerier, s.tsLogger)
				haCollector = collectors.NewHAReplicaCollector(sourceQuerier, s.tsLogger)
				replCollector = collectors.NewReplicationCollector(sourceQuerier, s.tsLogger)
			}
			// Fetch fresh collector configurations for dynamic frequencies
			configs, err := s.ConfigRepo.GetActiveConfigs(ctx)
			if err != nil {
				slog.Error("[HA-Collector] failed to fetch collector configs", "err", err)
				continue
			}

			// Map for easy lookup
			freqMap := make(map[string]int)
			for _, cfg := range configs {
				freqMap[cfg.CollectorName] = cfg.FrequencySeconds
			}

			// Default fallbacks if not in DB
			getFreq := func(name string, def int) time.Duration {
				if f, ok := freqMap[name]; ok && f > 0 {
					return time.Duration(f) * time.Second
				}
				return time.Duration(def) * time.Second
			}

			// Hot-reload: update ticker if admin changed the orchestration interval
			newInterval := s.GetCollectorInterval(ctx, "sqlserver_ha_replication", 30*time.Second)
			if newInterval > 0 && newInterval != interval {
				slog.Info("[HA-Collector] orchestration interval changed from", "arg1", interval, "arg2", newInterval)
				interval = newInterval
				ticker.Reset(interval)
			}

			discFreq        := getFreq("sqlserver_ha_discovery", 60)
			healthFreq      := getFreq("sqlserver_ha_health", 30)
			perfFreq        := getFreq("sqlserver_replication_performance", 60)
			topoFreq        := getFreq("sqlserver_replication_topology", 900)
			logShippingFreq := getFreq("sqlserver_log_shipping", 300)

			for _, inst := range s.Config.Instances {
				if strings.ToLower(inst.Type) != "sqlserver" {
					continue
				}

				serverID := inst.ServerID
				instanceName := inst.Name

				// 1. Feature Detection (Discovery)
				if time.Since(lastDiscovery[serverID]) >= discFreq {
					lastDiscovery[serverID] = time.Now() // always back off, even on failure
					ha, repl, err := featDetector.Run(ctx, serverID, instanceName)
					if err == nil {
						haEnabledMap[serverID] = ha
						replEnabledMap[serverID] = repl
					} else {
						slog.Error("[HA-Collector] feature detection failed", "target", instanceName, "err", err)
					}
				}

				// 2. HA Health Collection
				if haEnabledMap[serverID] && time.Since(lastHealth[serverID]) >= healthFreq {
					if err := haCollector.Run(ctx, serverID, instanceName); err == nil {
						lastHealth[serverID] = time.Now()
					} else {
						slog.Error("[HA-Collector] replica state collection failed", "target", instanceName, "err", err)
					}
				}

				// 3. Log Shipping Health (runs regardless of AG/replication status)
				if time.Since(lastLogShipping[serverID]) >= logShippingFreq {
					lsRows, err := activeRepo.FetchLogShippingHealth(ctx, instanceName)
					if err != nil {
						slog.Error("[HA-Collector] log shipping fetch failed", "target", instanceName, "err", err)
					} else if len(lsRows) > 0 {
						now := time.Now().UTC()
						var hotRows []hot.LogShippingRow
						for _, r := range lsRows {
							row := hot.LogShippingRow{
								CaptureTimestamp:        now,
								ServerID:                serverID,
								PrimaryServer:           r.PrimaryServer,
								PrimaryDatabase:         r.PrimaryDatabase,
								SecondaryServer:         r.SecondaryServer,
								SecondaryDatabase:       r.SecondaryDatabase,
								LastBackupFile:          r.LastBackupFile,
								RestoreDelayMinutes:     r.RestoreDelayMinutes,
								RestoreThresholdMinutes: r.RestoreThresholdMinutes,
								Status:                  r.Status,
								IsPrimary:               r.IsPrimary,
							}
							if r.LastBackupDate.Valid {
								t := r.LastBackupDate.Time
								row.LastBackupDate = &t
							}
							if r.LastRestoreDate.Valid {
								t := r.LastRestoreDate.Time
								row.LastRestoreDate = &t
							}
							if r.LastCopiedDate.Valid {
								t := r.LastCopiedDate.Time
								row.LastCopiedDate = &t
							}
							hotRows = append(hotRows, row)
						}
						if err := s.tsLogger.LogLogShippingHealth(ctx, serverID, hotRows); err != nil {
							slog.Error("[HA-Collector] log shipping persist failed", "target", instanceName, "err", err)
						} else {
							lastLogShipping[serverID] = time.Now()
						}
					} else {
						// No log shipping configured — back off timer so we don't hammer the query
						lastLogShipping[serverID] = time.Now()
					}
				}

				// 4. Replication Collection
				if replEnabledMap[serverID] {
					// Performance (Latency/Backlog)
					if time.Since(lastPerf[serverID]) >= perfFreq {
						if err := replCollector.RunLatency(ctx, serverID, instanceName); err == nil {
							lastPerf[serverID] = time.Now()
						} else {
							slog.Error("[HA-Collector] replication latency collection failed", "target", instanceName, "err", err)
						}
					}

					// Topology & Articles — only advance the timer when both succeed so the
					// next tick retries rather than silently skipping a full topoFreq window.
					if time.Since(lastTopo[serverID]) >= topoFreq {
						topoErr := replCollector.RunTopology(ctx, serverID, instanceName)
						if topoErr != nil {
							slog.Error("[HA-Collector] replication topology collection failed", "target", instanceName, "err", topoErr)
						}
						artErr := replCollector.RunArticles(ctx, serverID, instanceName)
						if artErr != nil {
							slog.Error("[HA-Collector] replication articles collection failed", "target", instanceName, "err", artErr)
						}
						if topoErr == nil && artErr == nil {
							lastTopo[serverID] = time.Now()
						}
					}
				}
			}
		}
	}
}
