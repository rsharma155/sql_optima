// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Database optimization recommendation engine based on collected metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package recommendations

import (
	"strings"
)

// GetAdviceForWaitType returns tuning recommendations based on wait type
func GetAdviceForWaitType(waitType string) string {
	if waitType == "" {
		return "Unknown wait type. Review wait statistics for more details."
	}

	// Normalize wait type for case-insensitive comparison
	waitTypeUpper := strings.ToUpper(waitType)

	switch {
	case strings.Contains(waitTypeUpper, "WRITELOG"):
		return "Transaction log bottleneck detected. Check log disk latency, increase log file size, and avoid frequent row-by-row commits."

	case strings.Contains(waitTypeUpper, "PAGEIOLATCH"):
		return "High physical reads detected. Look for missing indexes, increase buffer pool memory, or check storage latency."

	case strings.Contains(waitTypeUpper, "CXPACKET"):
		return "Parallelism inefficiency detected. Review MAXDOP and Cost Threshold for Parallelism settings."

	case strings.Contains(waitTypeUpper, "PAGELATCH"):
		return "Page latch contention detected. Check for hot spots in allocation pages ( PFS, GAM, SGAM). Consider partition alignment."

	case strings.Contains(waitTypeUpper, "ASYNC_NETWORK_IO"):
		return "Network I/O bottleneck detected. Review client application processing time, reduce result set size, or enable MARS."

	case strings.Contains(waitTypeUpper, "OLEDB"):
		return "OLEDB wait detected. Check for linked server issues or distributed query performance problems."

	case strings.Contains(waitTypeUpper, "SOS_SCHEDULER_YIELD"):
		return "Scheduler yield wait detected. Indicates CPU pressure. Review CPU-intensive queries and consider optimization."

	case strings.Contains(waitTypeUpper, "DBMIRROR"):
		return "Database mirroring wait detected. Review mirroring latency and network performance between principal and mirror."

	case strings.Contains(waitTypeUpper, "BACKUP"):
		return "Backup operation wait detected. Check backup performance and consider optimizing backup schedule."

	case strings.Contains(waitTypeUpper, "LCK_M_"):
		return "Lock wait detected. Review blocking chains, consider optimistic isolation levels, or optimize index strategy to reduce lock contention."

	default:
		return "Review wait statistics in detail. Consider index optimization, query tuning, or resource allocation adjustments."
	}
}

// GetAdviceForQueryRegression provides recommendations for query performance regressions
func GetAdviceForQueryRegression(baselineMs, currentMs float64) string {
	regressionPct := ((currentMs - baselineMs) / baselineMs) * 100

	switch {
	case regressionPct > 100:
		return "Query execution time has more than doubled. Likely cause: execution plan change. Review recent statistics updates, index changes, or schema modifications."
	case regressionPct > 50:
		return "Significant query regression detected (50-100%). Check for parameter sniffing issues, statistics staleness, or memory grant changes."
	default:
		return "Minor regression detected. Review query execution plan for subtle changes."
	}
}

// GetAdviceForResourcePressure provides recommendations for resource exhaustion
func GetAdviceForResourcePressure(resourceType string, utilizationPct float64) string {
	switch resourceType {
	case "memory":
		if utilizationPct >= 95 {
			return "Critical memory pressure. Review large memory-consuming queries, consider clearing plan cache, or add more RAM."
		}
		return "Memory utilization high. Monitor memory trends and review buffer pool usage."

	case "cpu":
		return "CPU pressure detected. Review CPU-intensive queries, consider MAXDOP adjustments, or optimize computational workloads."

	case "workers":
		return "Worker thread exhaustion. Increase max worker threads or optimize concurrent workload."

	default:
		return "Resource pressure detected. Review workload patterns and resource configuration."
	}
}
