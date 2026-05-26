// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package recommendations


import (
	"fmt"

	"github.com/rsharma155/sql_optima/internal/models"
)

type RecommendationGenerator struct{}

func NewRecommendationGenerator() *RecommendationGenerator {
	return &RecommendationGenerator{}
}

func (g *RecommendationGenerator) Generate(triggered []models.RuleTriggerResult, rawData map[string]interface{}, thresholds models.DynamicThresholds, config models.ServerConfig) []map[string]string {
	var actions []map[string]string
	seen := make(map[string]bool)
	triggeredNames := make(map[string]bool)
	for _, t := range triggered {
		triggeredNames[t.RuleName] = true
	}
	for _, t := range triggered {
		recs := g.recommendationsForRule(t, rawData, thresholds, config)
		for _, rec := range recs {
			if !seen[rec] {
				seen[rec] = true
				urgency := "short-term"
				if t.Severity == "critical" || t.Severity == "high" {
					urgency = "immediate"
				}
				reason := t.Message
				if len(reason) > 200 {
					reason = reason[:200]
				}
				actions = append(actions, map[string]string{
					"action":  rec,
					"urgency": urgency,
					"reason":  reason,
				})
			}
		}
	}
	if len(actions) == 0 {
		actions = append(actions, map[string]string{
			"action":  "Continue routine monitoring",
			"urgency": "strategic",
			"reason":  "No issues detected",
		})
	}
	if len(actions) > 10 {
		actions = actions[:10]
	}
	return actions
}

func (g *RecommendationGenerator) recommendationsForRule(rule models.RuleTriggerResult, raw map[string]interface{}, thresholds models.DynamicThresholds, config models.ServerConfig) []string {
	switch rule.RuleName {
	case "cpu_saturation":
		return g.recCPUSaturation(rule, raw, thresholds, config)
	case "scheduler_starvation":
		return g.recSchedulerStarvation(rule, raw, thresholds, config)
	case "ple_collapse":
		return g.recPLECollapse(rule, raw, thresholds, config)
	case "memory_grant_pressure":
		return []string{
			"Query sessions with high granted_query_memory_mb",
			"Review RESOURCE_SEMAPHORE waits",
			"Check sort and hash warnings in memory_metrics",
		}
	case "low_disk_space":
		return g.recLowDiskSpace(rule, raw, thresholds, config)
	case "rapid_disk_growth":
		return g.recRapidDiskGrowth(rule, raw, thresholds, config)
	case "io_latency_high":
		return []string{
			"Check PAGEIOLATCH_SH/EX waits",
			"Review per-database read latency and write latency",
			"Move high-I/O databases to faster storage tier",
		}
	case "replication_lag":
		return g.recReplicationLag(rule, raw, thresholds, config)
	case "blocking_chains":
		return []string{
			"Identify head blocker from long_running_queries",
			"Check wait_type for LCK_M_ contention patterns",
			"Consider Read Committed Snapshot Isolation",
		}
	case "backup_failure_risk":
		return []string{
			"Check job failures for specific error messages",
			"Verify backup destination has sufficient space",
			"Test restore process to validate backup integrity",
			"Configure alerting on SQL Agent job failure",
		}
	case "tempdb_pressure":
		return g.recTempDBPressure(rule, raw, thresholds, config)
	case "deadlocks_detected":
		return []string{
			"Enable deadlock graph capture via extended events",
			"Review application code for consistent lock ordering",
			"Check LCK_M_ wait types for contention patterns",
		}
	case "performance_debt":
		return []string{
			"Review performance debt findings by section",
			"Address CRITICAL severity findings first",
			"Schedule index maintenance for fragmented indexes",
			"Update stale statistics",
		}
	default:
		return []string{fmt.Sprintf("Investigate and resolve %s", rule.RuleName)}
	}
}

func (g *RecommendationGenerator) recCPUSaturation(rule models.RuleTriggerResult, raw map[string]interface{}, thresholds models.DynamicThresholds, config models.ServerConfig) []string {
	maxdopRec := config.CoresPerSocket
	if maxdopRec > 8 {
		maxdopRec = 8
	}
	return []string{
		"Identify top CPU consumers in long_running_queries for high cpu_time_ms sessions",
		fmt.Sprintf("Review MAXDOP setting — %d cores detected, MAXDOP should be ≤%d per NUMA", config.CPUCount, maxdopRec),
		"Check scheduler_stats for worker_thread_exhaustion flag",
		"Analyze wait stats for SOS_SCHEDULER_YIELD / CXPACKET",
		"Consider Resource Governor to cap CPU for non-critical workloads",
	}
}

func (g *RecommendationGenerator) recSchedulerStarvation(rule models.RuleTriggerResult, raw map[string]interface{}, thresholds models.DynamicThresholds, config models.ServerConfig) []string {
	return []string{
		fmt.Sprintf("Immediate: reduce concurrent workload — %d max workers, %d cores", config.MaxWorkers, config.CPUCount),
		"Check for spinlock contention via latch waits",
		"Review worker thread exhaustion in scheduler stats",
		"Consider increasing max worker threads if hardware can support",
	}
}

func (g *RecommendationGenerator) recPLECollapse(rule models.RuleTriggerResult, raw map[string]interface{}, thresholds models.DynamicThresholds, config models.ServerConfig) []string {
	memUsed := getFloatRaw(raw, "sql_memory_used_mb")
	memTarget := getFloatRaw(raw, "sql_memory_target_mb")
	cacheHit := getFloatRaw(raw, "buffer_cache_hit_ratio")
	availPhysKB := getFloatRaw(raw, "available_physical_memory_kb")

	recs := []string{}

	// Memory configuration rec — always include when RAM info is available
	osReserveGB := maxInt(4, config.TotalRAMGB/8)
	if config.TotalRAMGB > 0 && memUsed > 0 && memTarget > 0 {
		recs = append(recs, fmt.Sprintf(
			"SQL Server is using %.0fMB of its %.0fMB target on a %dGB server — ensure max server memory leaves at least %dGB free for the OS",
			memUsed, memTarget, config.TotalRAMGB, osReserveGB))
	} else if config.TotalRAMGB > 0 {
		recs = append(recs, fmt.Sprintf(
			"Review max server memory setting on this %dGB server — reserve at least %dGB for OS and non-SQL processes",
			config.TotalRAMGB, osReserveGB))
	} else {
		recs = append(recs, "Review max server memory in sys.configurations and ensure adequate OS memory reservation")
	}

	// Cache hit ratio — only include when we have a real value
	if cacheHit > 0 {
		if cacheHit < 95 {
			recs = append(recs, fmt.Sprintf("Buffer cache hit ratio is %.0f%% — below 95%% indicates the buffer pool is too small for the working dataset", cacheHit))
		} else {
			recs = append(recs, fmt.Sprintf("Buffer cache hit ratio is %.0f%% — PLE pressure may be caused by large scans evicting hot pages", cacheHit))
		}
	} else {
		recs = append(recs, "Check buffer cache hit ratio in sys.dm_os_performance_counters (target: ≥99% for OLTP)")
	}

	recs = append(recs, "Identify large table scans causing buffer pool churn — look for PAGEIOLATCH_SH waits and missing index alerts")

	// OS physical memory — only include when non-zero
	if availPhysKB > 1024 {
		recs = append(recs, fmt.Sprintf("OS has %.0fMB available physical memory — if this is consistently low, other processes are competing for RAM", availPhysKB/1024))
	}

	recs = append(recs, "If PLE stays below threshold after tuning, consider adding RAM or reducing max server memory for other processes")
	return recs
}

func (g *RecommendationGenerator) recLowDiskSpace(rule models.RuleTriggerResult, raw map[string]interface{}, thresholds models.DynamicThresholds, config models.ServerConfig) []string {
	freeMB := getFloatRaw(raw, "free_disk_mb")
	if freeMB == 0 {
		freeMB = getFloatRaw(raw, "total_free_mb")
	}
	growth := getFloatRaw(raw, "delta_data_mb")
	neededMB := thresholds.DiskFreeMBMin - freeMB
	if neededMB < 1024 {
		neededMB = 1024
	}

	totalDisk := float64(config.TotalDiskGB)
	if totalDisk == 0 {
		totalDisk = (getFloatRaw(raw, "total_data_mb") + getFloatRaw(raw, "total_log_mb") + freeMB) / 1024
	}

	recs := []string{
		fmt.Sprintf("Free up %.1fGB of disk space — free space is below the %.1fGB safety threshold", neededMB/1024, thresholds.DiskFreeMBMin/1024),
		"Purge old backup files, logs, and temporary staging tables",
	}
	if growth > 0 {
		recs = append(recs, fmt.Sprintf("Disk growing at %.1f MB/collection-cycle — identify top growth contributors in sqlserver_disk_history", growth))
	} else {
		recs = append(recs, "Review sqlserver_disk_history to identify which databases are consuming the most space")
	}
	recs = append(recs, "Implement data archiving or partitioning for large historical tables")
	if totalDisk > 0 {
		recs = append(recs, fmt.Sprintf("Server has %.0fGB total disk configured — evaluate whether online storage expansion or additional volumes are needed", totalDisk))
	}
	return recs
}

func (g *RecommendationGenerator) recRapidDiskGrowth(rule models.RuleTriggerResult, raw map[string]interface{}, thresholds models.DynamicThresholds, config models.ServerConfig) []string {
	return []string{
		"Identify top growth contributors from disk history",
		"Review retention policies and implement data purging schedule",
		"Consider table partitioning for historical data",
		fmt.Sprintf("Monitor daily growth rate against threshold: %.0f MB/day", thresholds.DiskGrowthRateMaxMBPerDay),
	}
}

func (g *RecommendationGenerator) recReplicationLag(rule models.RuleTriggerResult, raw map[string]interface{}, thresholds models.DynamicThresholds, config models.ServerConfig) []string {
	return []string{
		"Check synchronization state in ha_replica_state",
		fmt.Sprintf("Review log_send_queue_kb (%.0f KB) and redo_queue_kb (%.0f KB)", getFloatRaw(raw, "log_send_queue_kb"), getFloatRaw(raw, "redo_queue_kb")),
		"Verify failover readiness flag",
		"Check network bandwidth between primary and secondary replicas",
	}
}

func (g *RecommendationGenerator) recTempDBPressure(rule models.RuleTriggerResult, raw map[string]interface{}, thresholds models.DynamicThresholds, config models.ServerConfig) []string {
	return []string{
		fmt.Sprintf("Add tempdb data files — recommended %d files of equal size", minInt(8, config.CPUCount/2)),
		fmt.Sprintf("Check for query spills via sort warnings (%.0f/s) and hash warnings (%.0f/s)", getFloatRaw(raw, "sort_warnings_per_sec"), getFloatRaw(raw, "hash_warnings_per_sec")),
		"Review long-running transactions",
		"Check VLF count in tempdb",
	}
}

func getFloatRaw(raw map[string]interface{}, key string) float64 {
	if v, ok := raw[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
