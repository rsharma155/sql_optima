// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Regression tests for DEFECT-7 — duplicate CurrentIssues entries from
// conflicting YAML and dynamic rules on the same metric.
//
// Each test verifies that a given metric scenario produces exactly ONE fired rule
// for that metric, not two rules from the two parallel systems.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT

package tests

import (
	"strings"
	"testing"

	"github.com/rsharma155/sql_optima/internal/intel/analysis"
	"github.com/rsharma155/sql_optima/internal/intel/rule_engine"
	"github.com/rsharma155/sql_optima/internal/models"
)

// countRulesForMetric counts how many triggered rules reference the given metric key.
func countRulesForMetric(rules []models.RuleTriggerResult, metricKey string) int {
	n := 0
	for _, r := range rules {
		if _, ok := r.MetricValues[metricKey]; ok {
			n++
		}
	}
	return n
}

// allRuleNames returns a slice of all fired rule names (for diagnostic messages).
func allRuleNames(rules []models.RuleTriggerResult) []string {
	names := make([]string, 0, len(rules))
	for _, r := range rules {
		names = append(names, r.RuleName)
	}
	return names
}

// ---- CPU: avg_cpu_load must not fire both cpu_saturation and cpu_burst ----

func TestDefect7NoCPUBurstDuplicate(t *testing.T) {
	// avg_cpu_load = 92% with runnable tasks — was triggering both
	// dynamic "cpu_saturation" AND yaml "cpu_burst" simultaneously.
	raw := map[string]interface{}{
		"avg_cpu_load":               92.0,
		"total_runnable_tasks_count": 8.0,
		"socket_count":               1.0,
		"numa_nodes":                 1.0,
		"max_workers_count":          512.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")

	// Must not see "cpu_burst" at all
	for _, r := range result.TriggeredRules {
		if r.RuleName == "cpu_burst" {
			t.Errorf("DEFECT-7 regression: cpu_burst fired alongside cpu_saturation — duplicate avg_cpu_load issue. All rules: %v", allRuleNames(result.TriggeredRules))
		}
	}

	// Must see exactly one rule referencing avg_cpu_load
	cpuRules := countRulesForMetric(result.TriggeredRules, "avg_cpu_load")
	if cpuRules > 1 {
		t.Errorf("expected at most 1 rule referencing avg_cpu_load, got %d. Rules: %v", cpuRules, allRuleNames(result.TriggeredRules))
	}
}

func TestDefect7CPUSaturationStillFires(t *testing.T) {
	// Removing cpu_burst must not suppress the legitimate dynamic rule.
	raw := map[string]interface{}{
		"avg_cpu_load":               92.0,
		"total_runnable_tasks_count": 15.0,
		"socket_count":               1.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")

	found := false
	for _, r := range result.TriggeredRules {
		if r.RuleName == "cpu_saturation" {
			found = true
		}
	}
	if !found {
		t.Error("expected cpu_saturation to still fire after DEFECT-7 fix — removing cpu_burst must not suppress the dynamic rule")
	}
}

// ---- Replication: secondary_lag_seconds must not fire ha_replication_lag + replication_lag ----

func TestDefect7NoReplicationLagDuplicate(t *testing.T) {
	// secondary_lag_seconds = 80s — was triggering both
	// dynamic "replication_lag" AND yaml "ha_replication_lag" simultaneously.
	raw := map[string]interface{}{
		"secondary_lag_seconds": 80.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")

	for _, r := range result.TriggeredRules {
		if r.RuleName == "ha_replication_lag" {
			t.Errorf("DEFECT-7 regression: ha_replication_lag fired — duplicates dynamic replication_lag. All rules: %v", allRuleNames(result.TriggeredRules))
		}
		if r.RuleName == "ha_replication_lag_critical" {
			t.Errorf("DEFECT-7 regression: ha_replication_lag_critical fired — duplicates dynamic replication_lag. All rules: %v", allRuleNames(result.TriggeredRules))
		}
	}

	// Exactly one rule should reference replication_lag_seconds
	replRules := countRulesForMetric(result.TriggeredRules, "replication_lag_seconds")
	if replRules > 1 {
		t.Errorf("expected at most 1 rule referencing replication_lag_seconds, got %d. Rules: %v", replRules, allRuleNames(result.TriggeredRules))
	}
}

func TestDefect7ReplicationCriticalLagStillFires(t *testing.T) {
	// 350s lag — was also triggering ha_replication_lag_critical. Verify only dynamic fires.
	raw := map[string]interface{}{
		"secondary_lag_seconds": 350.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")

	found := false
	for _, r := range result.TriggeredRules {
		if r.RuleName == "replication_lag" {
			found = true
			if r.Severity != "critical" {
				t.Errorf("expected replication_lag to be critical at 350s, got %s", r.Severity)
			}
		}
		if r.RuleName == "ha_replication_lag_critical" {
			t.Errorf("DEFECT-7 regression: ha_replication_lag_critical fired at 350s — should have been removed")
		}
	}
	if !found {
		t.Error("expected dynamic replication_lag to fire at 350s — removing YAML rules must not suppress it")
	}
}

// ---- Disk: delta_data_mb / free_disk_mb must not fire storage_exhaustion + growth rules ----

func TestDefect7NoStorageExhaustionDuplicate(t *testing.T) {
	// High growth + low free disk — was triggering storage_exhaustion (yaml) alongside
	// rapid_disk_growth and low_disk_space (dynamic).
	raw := map[string]interface{}{
		"delta_data_mb": 200.0,
		"free_disk_mb":  30000.0, // below 50000 MB
		"total_free_mb": 30000.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")

	for _, r := range result.TriggeredRules {
		if r.RuleName == "storage_exhaustion" {
			t.Errorf("DEFECT-7 regression: storage_exhaustion fired — duplicates dynamic disk rules. All rules: %v", allRuleNames(result.TriggeredRules))
		}
		if r.RuleName == "database_growth_acceleration" {
			t.Errorf("DEFECT-7 regression: database_growth_acceleration fired — duplicates dynamic rapid_disk_growth. All rules: %v", allRuleNames(result.TriggeredRules))
		}
	}
}

func TestDefect7DiskGrowthDynamicStillFires(t *testing.T) {
	// Removing storage_exhaustion must not suppress rapid_disk_growth.
	raw := map[string]interface{}{
		"delta_data_mb": 1200.0,
		"free_disk_mb":  80000.0,
		"total_free_mb": 80000.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")

	found := false
	for _, r := range result.TriggeredRules {
		if r.RuleName == "rapid_disk_growth" {
			found = true
		}
	}
	if !found {
		t.Error("expected rapid_disk_growth dynamic rule to still fire after DEFECT-7 fix")
	}
}

// ---- Buffer cache: dynamic threshold (no fixed 90% YAML) ----

func TestDynamicBufferCacheHitRatioLow(t *testing.T) {
	raw := map[string]interface{}{
		"buffer_cache_hit_ratio": 82.0,
		"total_ram_gb":           32.0,
		"cpu_count":              8.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")

	found := 0
	for _, r := range result.TriggeredRules {
		if r.RuleName == "buffer_cache_hit_ratio_low" {
			found++
			if !strings.Contains(r.Message, "dynamic threshold") {
				t.Errorf("expected dynamic threshold in message, got: %s", r.Message)
			}
			if thr, ok := r.MetricValues["bchr_threshold"]; !ok || thr <= 0 {
				t.Error("expected bchr_threshold in metric values")
			}
		}
	}
	if found != 1 {
		t.Errorf("expected exactly 1 buffer_cache_hit_ratio_low rule, got %d. Rules: %v", found, allRuleNames(result.TriggeredRules))
	}
}

// ---- PLE: no fixed 300s YAML duplicate ----

func TestPLECollapseUsesDynamicThresholdOnly(t *testing.T) {
	raw := map[string]interface{}{
		"ple_seconds":  250.0,
		"total_ram_gb": 32.0,
		"cpu_count":    8.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")

	pleRules := 0
	for _, r := range result.TriggeredRules {
		if r.RuleName == "ple_collapse" {
			pleRules++
			if strings.Contains(r.Message, "300") {
				t.Errorf("ple_collapse message must not reference fixed 300s YAML threshold: %s", r.Message)
			}
			thr, ok := r.MetricValues["ple_threshold"]
			if !ok || thr < 300 {
				t.Errorf("expected dynamic ple_threshold > 300 for 32GB server, got %.0f", thr)
			}
		}
	}
	if pleRules != 1 {
		t.Errorf("expected exactly 1 ple_collapse rule, got %d. Rules: %v", pleRules, allRuleNames(result.TriggeredRules))
	}
}

// ---- Backup failure: backup_failure_risk must still fire (deduped by name, not removed) ----

func TestDefect7BackupFailureRiskPreserved(t *testing.T) {
	raw := map[string]interface{}{
		"failed_jobs_24h": 2.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")

	found := false
	for _, r := range result.TriggeredRules {
		if r.RuleName == "backup_failure_risk" {
			found = true
		}
	}
	if !found {
		t.Error("expected backup_failure_risk to fire from dynamic rules")
	}
}

func TestRulePacksExcludeFixedThresholdDuplicates(t *testing.T) {
	rules, err := rule_engine.LoadRulePacks()
	if err != nil {
		t.Fatalf("LoadRulePacks: %v", err)
	}
	excluded := map[string]bool{
		"ple_collapse": true, "cpu_saturation": true, "blocking_chains": true,
		"buffer_cache_hit_ratio_low": true, "low_disk_space": true,
	}
	for _, r := range rules {
		if excluded[r.Name] {
			t.Errorf("YAML pack must not define %q — use dynamic evaluateDynamicRules only", r.Name)
		}
		for _, c := range r.Conditions {
			if c.Metric == "ple_seconds" && c.Value == 300 {
				t.Errorf("rule %q still uses fixed PLE 300 threshold", r.Name)
			}
			if c.Metric == "buffer_cache_hit_ratio" && c.Value == 90 {
				t.Errorf("rule %q still uses fixed BCHR 90 threshold", r.Name)
			}
		}
	}
}
