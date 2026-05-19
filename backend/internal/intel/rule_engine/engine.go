// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package rule_engine


import (
	"embed"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/intel/normalization"
)

//go:embed packs/*.yaml
var packsFS embed.FS

func LoadRulePacks() ([]CompiledRule, error) {
	var rules []CompiledRule
	entries, err := packsFS.ReadDir("packs")
	if err != nil {
		return nil, fmt.Errorf("reading rule packs: %w", err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		data, err := packsFS.ReadFile(filepath.Join("packs", entry.Name()))
		if err != nil {
			continue
		}
		content := string(data)
		docs := splitYAMLDocs(content)
		for _, docStr := range docs {
			var doc CompiledRule
			if err := yaml.Unmarshal([]byte(docStr), &doc); err != nil {
				continue
			}
			if doc.Name == "" {
				continue
			}
			if doc.Logic == "" {
				doc.Logic = "AND"
			}
			if doc.Severity == "" {
				doc.Severity = "medium"
			}
			rules = append(rules, doc)
		}
	}
	return rules, nil
}

func splitYAMLDocs(content string) []string {
	var docs []string
	current := ""
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if strings.TrimSpace(current) != "" {
				docs = append(docs, current)
			}
			current = ""
		} else {
			current += line + "\n"
		}
	}
	if strings.TrimSpace(current) != "" {
		docs = append(docs, current)
	}
	return docs
}

func ExtractMetricValue(norm *normalization.NormalizedSystem, metricName string) *float64 {
	metricMap := buildMetricMap(norm)
	if v, ok := metricMap[metricName]; ok {
		return &v
	}
	return nil
}

func buildMetricMap(norm *normalization.NormalizedSystem) map[string]float64 {
	m := make(map[string]float64)

	if norm.Host != nil {
		if norm.Host.CPU != nil {
			m["avg_cpu_load"] = norm.Host.CPU.AvgValue
			m["sql_process_cpu"] = norm.Host.CPU.CurrentValue
		}
		if norm.Host.Memory != nil {
			m["memory_usage_pct"] = norm.Host.Memory.CurrentValue
			m["ple_seconds"] = norm.Host.Memory.CurrentValue
		} else {
			m["ple_seconds"] = 500.0
		}
		if norm.Host.Disk != nil {
			m["free_disk_mb"] = norm.Host.Disk.CurrentValue
		}
		if norm.Host.IO != nil {
			m["read_latency_ms"] = norm.Host.IO.CurrentValue
		}
	}
	if norm.Database != nil && norm.Database.TempDB != nil {
		m["tempdb_used_pct"] = norm.Database.TempDB.CurrentValue
	}
	if norm.Replication != nil && norm.Replication.LagSeconds != nil {
		m["secondary_lag_seconds"] = norm.Replication.LagSeconds.CurrentValue
	} else {
		m["secondary_lag_seconds"] = 0.0
	}
	if norm.Capacity != nil {
		m["delta_data_mb"] = norm.Capacity.StorageGrowthDailyPct
		if norm.Capacity.DaysUntilDiskFull != nil {
			m["projected_fill_days"] = *norm.Capacity.DaysUntilDiskFull
		} else {
			m["projected_fill_days"] = 999.0
		}
	}
	return m
}

func EvaluateRules(rules []CompiledRule, norm *normalization.NormalizedSystem, metricOverrides map[string]float64) []models.RuleTriggerResult {
	var triggered []models.RuleTriggerResult
	metricValues := buildMetricMap(norm)
	for k, v := range metricOverrides {
		metricValues[k] = v
	}
	for _, rule := range rules {
		if result := evaluateSingleRule(rule, metricValues); result != nil {
			triggered = append(triggered, *result)
		}
	}
	sort.Slice(triggered, func(i, j int) bool {
		return severityIndex(triggered[i].Severity) < severityIndex(triggered[j].Severity)
	})
	return triggered
}

func EvaluateRulesFromRaw(rules []CompiledRule, rawData map[string]interface{}) []models.RuleTriggerResult {
	metricValues := normalization.BuildMetricMapFromRaw(rawData)
	var triggered []models.RuleTriggerResult
	for _, rule := range rules {
		if result := evaluateSingleRule(rule, metricValues); result != nil {
			triggered = append(triggered, *result)
		}
	}
	sort.Slice(triggered, func(i, j int) bool {
		return severityIndex(triggered[i].Severity) < severityIndex(triggered[j].Severity)
	})
	return triggered
}

func evaluateSingleRule(rule CompiledRule, metricValues map[string]float64) *models.RuleTriggerResult {
	var evaluated []bool
	capturedValues := make(map[string]float64)

	for _, cond := range rule.Conditions {
		val, ok := metricValues[cond.Metric]
		if !ok {
			evaluated = append(evaluated, false)
			continue
		}
		capturedValues[cond.Metric] = val
		var result bool
		switch cond.Operator {
		case ">":
			result = val > cond.Value
		case "<":
			result = val < cond.Value
		case ">=":
			result = val >= cond.Value
		case "<=":
			result = val <= cond.Value
		case "==":
			result = math.Abs(val-cond.Value) < 1e-9
		default:
			result = false
		}
		evaluated = append(evaluated, result)
	}

	var allMet bool
	if strings.ToUpper(rule.Logic) == "OR" {
		allMet = anyTrue(evaluated)
	} else {
		allMet = allTrue(evaluated)
	}

	if allMet && len(evaluated) > 0 {
		msg := rule.Message
		for key, val := range capturedValues {
			msg = strings.ReplaceAll(msg, fmt.Sprintf("{%s}", key), fmt.Sprintf("%.1f", val))
		}
		return &models.RuleTriggerResult{
			RuleName:      rule.Name,
			Severity:      rule.Severity,
			Message:       msg,
			MetricValues:  capturedValues,
			Recommendation: rule.Recommendation,
		}
	}
	return nil
}

func severityIndex(s string) int {
	switch s {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 99
	}
}

func anyTrue(bools []bool) bool {
	for _, b := range bools {
		if b {
			return true
		}
	}
	return false
}

func allTrue(bools []bool) bool {
	for _, b := range bools {
		if !b {
			return false
		}
	}
	return true
}
