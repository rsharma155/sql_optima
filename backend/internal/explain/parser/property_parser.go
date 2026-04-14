// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Property parser for plan node attributes.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package parser

import (
	"regexp"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain/types"
)

func IsPropertyLine(line string) bool {
	// Skip worker-specific lines
	if strings.HasPrefix(line, "Worker ") {
		return false
	}
	prefixes := []string{
		"Sort Key:", "Group Key:", "Hash Cond:", "Merge Cond:",
		"Index Cond:", "Join Filter:", "Filter:", "Rows Removed by Filter:",
		"Sort Method:", "Workers Planned:", "Workers Launched:", "Buckets:",
		"Batches:", "Memory Usage:", "Buffers:", "Temp Files:",
		"Work Memory:", "Memory Context:", "Details:", "Start Time:",
		"Disk:", "JIT:",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

func ApplyProperty(line string, node *types.PlanNode) {
	if node == nil {
		return
	}

	switch {
	case strings.HasPrefix(line, "Sort Key:"):
		parts := strings.Split(strings.TrimPrefix(line, "Sort Key:"), ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		node.SortKey = parts

	case strings.HasPrefix(line, "Group Key:"):
		parts := strings.Split(strings.TrimPrefix(line, "Group Key:"), ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		node.GroupKey = parts

	case strings.HasPrefix(line, "Hash Cond:"):
		node.HashCond = strings.TrimSpace(strings.TrimPrefix(line, "Hash Cond:"))

	case strings.HasPrefix(line, "Merge Cond:"):
		node.MergeCond = strings.TrimSpace(strings.TrimPrefix(line, "Merge Cond:"))

	case strings.HasPrefix(line, "Index Cond:"):
		node.IndexCond = strings.TrimSpace(strings.TrimPrefix(line, "Index Cond:"))

	case strings.HasPrefix(line, "Join Filter:"):
		node.JoinFilter = strings.TrimSpace(strings.TrimPrefix(line, "Join Filter:"))

	case strings.HasPrefix(line, "Filter:"):
		node.Filter = strings.TrimSpace(strings.TrimPrefix(line, "Filter:"))

	case strings.HasPrefix(line, "Rows Removed by Filter:"):
		re := regexp.MustCompile(`(\d+)`)
		m := re.FindStringSubmatch(line)
		if m != nil {
			node.RowsRemovedByFilter, _ = atoi(m[1])
		}

	case strings.HasPrefix(line, "Sort Method:"):
		val := strings.TrimSpace(strings.TrimPrefix(line, "Sort Method:"))
		if idx := strings.Index(val, " "); idx > 0 {
			val = val[:idx]
		}
		node.SortMethod = val

		if strings.Contains(line, "Disk:") {
			node.SortSpaceType = "Disk"
			re := regexp.MustCompile(`Disk:\s*(\d+)kB`)
			m := re.FindStringSubmatch(line)
			if m != nil {
				node.SortSpaceUsed, _ = atoi(m[1])
			}
		}

	case strings.HasPrefix(line, "Disk:"):
		node.SortSpaceType = "Disk"
		re := regexp.MustCompile(`(\d+)kB`)
		m := re.FindStringSubmatch(line)
		if m != nil {
			node.SortSpaceUsed, _ = atoi(m[1])
		}

	case strings.HasPrefix(line, "Buckets:"):
		re := regexp.MustCompile(`(\d+)`)
		m := re.FindStringSubmatch(line)
		if m != nil {
			node.Buckets, _ = atoi(m[1])
		}

	case strings.HasPrefix(line, "Batches:"):
		re := regexp.MustCompile(`(\d+)`)
		m := re.FindStringSubmatch(line)
		if m != nil {
			node.Batches, _ = atoi(m[1])
		}

	case strings.HasPrefix(line, "Memory Usage:"):
		re := regexp.MustCompile(`(\d+)kB`)
		m := re.FindStringSubmatch(line)
		if m != nil {
			node.MemoryUsage, _ = atoi(m[1])
		}

	case strings.HasPrefix(line, "Buffers:"):
		node.Buffers = ParseBuffers(line)

	case strings.HasPrefix(line, "Workers Planned:"):
		re := regexp.MustCompile(`(\d+)`)
		m := re.FindStringSubmatch(line)
		if m != nil {
			node.WorkersPlanned, _ = atoi(m[1])
		}

	case strings.HasPrefix(line, "Workers Launched:"):
		re := regexp.MustCompile(`(\d+)`)
		m := re.FindStringSubmatch(line)
		if m != nil {
			node.WorkersLaunched, _ = atoi(m[1])
		}

	case strings.HasPrefix(line, "Worker "):
		re := regexp.MustCompile(`Worker\s+(\d+):\s*Sort Method:\s+(\w+)\s+Disk:\s*(\d+)kB`)
		m := re.FindStringSubmatch(line)
		if m != nil {
			// Worker-specific disk info - could store per worker if needed
			// For now, we just track the max disk usage
			diskUsage, _ := atoi(m[3])
			if diskUsage > node.SortSpaceUsed {
				node.SortSpaceUsed = diskUsage
				node.SortSpaceType = "Disk"
			}
		}
	}
}

func atoi(s string) (int, error) {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n, nil
}
