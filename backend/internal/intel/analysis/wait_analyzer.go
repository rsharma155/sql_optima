// Package intel provides the SQL Optima autonomous health intelligence engine.
// This file ranks and categorizes SQL Server wait types into DBA-meaningful categories.
//
// Design context:
//   - Input is the result of query 7.4 (sqlserver_wait_stats_delta, top 15 wait types by 24h delta).
//   - Each wait type is mapped to a category (CPU | Lock | IO | Memory | Network | Other).
//   - WaitScore = min(100, pctOfTotal × 2) so a wait type consuming 50%+ of total = score 100.
//   - Plain-English implications are provided for the top 10 most common SQL Server wait types.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT

package analysis

import (
	"math"
	"strings"

	"github.com/rsharma155/sql_optima/internal/models"
)

// WaitRawRow is a single row from the top wait types query (7.4).
type WaitRawRow struct {
	WaitType             string
	WaitCategory         string
	TotalWaitMS          float64
	TotalResourceWaitMS  float64
	TotalWaitingTasks    float64
	PctOfTotal           float64
	Rank                 int
}

// AnalyzeWaitTypes converts raw query rows into ranked WaitTypeEntry values.
// Returns nil when the input slice is empty.
func AnalyzeWaitTypes(rows []WaitRawRow) []models.WaitTypeEntry {
	if len(rows) == 0 {
		return nil
	}

	result := make([]models.WaitTypeEntry, 0, len(rows))
	for _, r := range rows {
		cat, impl := categorizeWaitType(r.WaitType)
		score := math.Min(100, r.PctOfTotal*2)
		result = append(result, models.WaitTypeEntry{
			Rank:         r.Rank,
			WaitType:     r.WaitType,
			WaitCategory: r.WaitCategory,
			TotalWaitMS:  r.TotalWaitMS,
			PctOfTotal:   r.PctOfTotal,
			WaitingTasks: r.TotalWaitingTasks,
			Category:     cat,
			Implication:  impl,
			WaitScore:    score,
		})
	}
	return result
}

// categorizeWaitType maps a SQL Server wait type name to a DBA category and implication.
func categorizeWaitType(waitType string) (category, implication string) {
	upper := strings.ToUpper(waitType)

	switch {
	case strings.HasPrefix(upper, "PAGEIOLATCH_") || upper == "IO_COMPLETION" ||
		upper == "ASYNC_IO_COMPLETION" || strings.HasPrefix(upper, "IO_QUEUE_"):
		return "IO", "Disk read/write bottleneck. Check read/write latency on data and log volumes."

	case strings.HasPrefix(upper, "LCK_M_") || strings.HasPrefix(upper, "LOCK_"):
		return "Lock", "Exclusive lock contention. Investigate blocking chains and long-running transactions."

	case upper == "CXPACKET" || upper == "CXCONSUMER" || upper == "CXSYNC_PORT" || upper == "CXSYNC_CONSUMER":
		return "Parallelism", "Parallel query overhead. Review MAXDOP setting and cost threshold for parallelism."

	case upper == "SOS_SCHEDULER_YIELD" || upper == "THREADPOOL":
		return "CPU", "CPU saturation. The server may need more CPU capacity or a workload reduction."

	case upper == "RESOURCE_SEMAPHORE" || upper == "RESOURCE_SEMAPHORE_MUTEX" || upper == "RESOURCE_SEMAPHORE_QUERY_COMPILE":
		return "Memory", "Memory grant queue pressure. Queries are waiting for workspace memory."

	case upper == "WRITELOG":
		return "Log IO", "Transaction log I/O bottleneck. Check log disk speed and VLF count."

	case upper == "ASYNC_NETWORK_IO":
		return "Network", "Client not consuming results fast enough (network back-pressure)."

	case strings.HasPrefix(upper, "HADR_") || strings.HasPrefix(upper, "DBMIRROR"):
		return "HA/AG", "Always On or mirroring synchronization overhead."

	case strings.HasPrefix(upper, "LATCH_"):
		return "Latch", "Internal engine latch contention. May indicate hot pages in buffer pool."

	case upper == "SLEEP_DBSTARTUP" || upper == "SLEEP_DBTASK" || upper == "SLEEP_DBSUSPEND":
		return "Other", "Database startup or maintenance activity."

	default:
		return "Other", "Non-critical or background SQL Server wait. Review if share exceeds 15%."
	}
}
