// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Metrics parser for extracting performance numbers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package parser

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain/types"
)

func ParseCostInfo(info string, node *types.PlanNode) {
	re := regexp.MustCompile(`cost[=:]?\s*([0-9.]+)\.\.([0-9.]+)`)
	m := re.FindStringSubmatch(info)
	if m != nil {
		node.StartupCost, _ = strconv.ParseFloat(m[1], 64)
		node.TotalCost, _ = strconv.ParseFloat(m[2], 64)
	}

	re = regexp.MustCompile(`rows[=:]?\s*(\d+)`)
	m = re.FindStringSubmatch(info)
	if m != nil {
		node.PlanRows, _ = strconv.Atoi(m[1])
	}

	re = regexp.MustCompile(`width[=:]?\s*(\d+)`)
	m = re.FindStringSubmatch(info)
	if m != nil {
		node.PlanWidth, _ = strconv.Atoi(m[1])
	}
}

func ParseActualInfo(info string, node *types.PlanNode) {
	re := regexp.MustCompile(`(?i)actual(?:\s*time)?[=:]\s*([0-9.]+)\.\.([0-9.]+)`)
	m := re.FindStringSubmatch(info)
	if m != nil {
		node.ActualStartupTime, _ = strconv.ParseFloat(m[1], 64)
		node.ActualTotalTime, _ = strconv.ParseFloat(m[2], 64)
	}

	re = regexp.MustCompile(`(?i)loops?[=:]\s*(\d+)`)
	m = re.FindStringSubmatch(info)
	if m != nil {
		node.ActualLoops, _ = strconv.Atoi(m[1])
	}

	re = regexp.MustCompile(`(?i)rows?[=:]\s*(\d+)`)
	m = re.FindStringSubmatch(info)
	if m != nil {
		node.ActualRows, _ = strconv.Atoi(m[1])
	}
}

func ParseActualLine(line string, node *types.PlanNode) {
	re := regexp.MustCompile(`actual[time=:]?\s*([0-9.]+)\.\.([0-9.]+)`)
	m := re.FindStringSubmatch(line)
	if m != nil {
		node.ActualStartupTime, _ = strconv.ParseFloat(m[1], 64)
		node.ActualTotalTime, _ = strconv.ParseFloat(m[2], 64)
	}

	re = regexp.MustCompile(`loops[=:]?\s*(\d+)`)
	m = re.FindStringSubmatch(line)
	if m != nil {
		node.ActualLoops, _ = strconv.Atoi(m[1])
	}

	re = regexp.MustCompile(`rows[=:]?\s*(\d+)`)
	m = re.FindStringSubmatch(line)
	if m != nil && node.ActualTotalTime > 0 {
		node.ActualRows, _ = strconv.Atoi(m[1])
	}
}

func ParsePlanningTime(line string) float64 {
	re := regexp.MustCompile(`([0-9.]+)\s*ms`)
	m := re.FindStringSubmatch(line)
	if m != nil {
		t, _ := strconv.ParseFloat(m[1], 64)
		return t
	}
	return 0
}

func ParseExecutionTime(line string) float64 {
	re := regexp.MustCompile(`([0-9.]+)\s*ms`)
	m := re.FindStringSubmatch(line)
	if m != nil {
		t, _ := strconv.ParseFloat(m[1], 64)
		return t
	}
	return 0
}

func ParseJITInfo(lines []string, plan *types.Plan) {
	var inJIT bool
	var jit *types.JitInfo
	var timingValues map[string]float64

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "JIT:") {
			inJIT = true
			jit = &types.JitInfo{}
			timingValues = make(map[string]float64)
			continue
		}
		if !inJIT {
			continue
		}
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "Functions:") {
			re := regexp.MustCompile(`(\d+)`)
			m := re.FindStringSubmatch(line)
			if m != nil {
				jit.Functions, _ = strconv.Atoi(m[1])
			}
		} else if strings.HasPrefix(line, "Options:") {
			// Options are informational, can be stored if needed
		} else if strings.HasPrefix(line, "Timing:") {
			parts := strings.Split(strings.TrimPrefix(line, "Timing:"), ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasSuffix(part, "ms") {
					re := regexp.MustCompile(`(\w+)\s+(\d+\.\d+)\s*ms`)
					m := re.FindStringSubmatch(part)
					if m != nil {
						timingValues[m[1]], _ = strconv.ParseFloat(m[2], 64)
					}
				}
			}
		} else if strings.HasPrefix(line, "Execution Time:") {
			// This is the total execution time, already parsed
			inJIT = false
		} else if strings.HasPrefix(line, "Planning Time:") {
			inJIT = false
		}
	}

	if jit != nil && len(timingValues) > 0 {
		jit.Timeline = int(timingValues["Total"])
		plan.Jit = jit
	}
}
