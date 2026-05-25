// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Enrich intelligence report narrative with wait and live-metric context.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package analysis

import (
	"fmt"
	"strings"

	"github.com/rsharma155/sql_optima/internal/models"
)

// AppendWaitAndMetricContext adds DBA-oriented correlation text after the base narrative.
func AppendWaitAndMetricContext(base string, raw map[string]interface{}, topWaits []models.WaitTypeEntry) string {
	if base == "" {
		return base
	}
	var notes []string

	if len(topWaits) > 0 {
		top := topWaits[0]
		note := fmt.Sprintf("Dominant wait: %s (%.1f%% of 24h wait time) — %s",
			top.WaitType, top.PctOfTotal, top.Implication)
		cpu := getFloat(raw, "avg_cpu_load", "", 0)
		runnable := getFloat(raw, "total_runnable_tasks_count", "", 0)
		ple := getFloat(raw, "ple_seconds", "ple", 0)
		freeMB := getFloat(raw, "total_free_mb", "free_disk_mb", -1)
		switch top.Category {
		case "CPU", "Parallelism":
			if cpu > 0 {
				note += fmt.Sprintf(" Current CPU %.1f%%", cpu)
			}
			if runnable > 0 {
				note += fmt.Sprintf(", runnable %.0f", runnable)
			}
		case "Memory":
			if ple > 0 {
				note += fmt.Sprintf(" PLE %.0fs", ple)
			}
			grants := getFloat(raw, "memory_grants_pending", "", 0)
			if grants > 0 {
				note += fmt.Sprintf(", grants pending %.0f", grants)
			}
		case "IO", "Log IO":
			readLat := getFloat(raw, "read_latency_ms", "", 0)
			writeLat := getFloat(raw, "write_latency_ms", "", 0)
			if readLat > 0 || writeLat > 0 {
				note += fmt.Sprintf(" I/O latency read/write %.1f/%.1f ms", readLat, writeLat)
			}
		case "Lock":
			blocking := getFloat(raw, "blocking_sessions", "", 0)
			if blocking > 0 {
				note += fmt.Sprintf(" Blocking sessions %.0f", blocking)
			}
		}
		if freeMB >= 0 && (top.Category == "IO" || strings.Contains(strings.ToUpper(top.WaitType), "PAGEIO")) {
			note += fmt.Sprintf("; free disk %.0f MB", freeMB)
		}
		notes = append(notes, note+".")
	}

	if qw, ok := raw["query_regressions_24h"].(float64); ok && qw > 0 {
		notes = append(notes, fmt.Sprintf("%.0f Query Store regression(s) detected in the last 24 hours — review Query Analysis.", qw))
	}
	if pi, ok := raw["plan_instability_queries"].(float64); ok && pi > 0 {
		notes = append(notes, fmt.Sprintf("%.0f quer(ies) with multiple plans (plan instability) in the last 24 hours.", pi))
	}

	if len(notes) == 0 {
		return base
	}
	return base + " " + strings.Join(notes, " ")
}
