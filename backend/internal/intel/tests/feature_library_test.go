// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Tests for DEFECT-15 Phase A — evaluateFeatureLibraryChecks wiring.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT

package tests

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/intel/analysis"
)

// ruleNames runs Analyze and returns a map of ruleName → severity for quick assertions.
func ruleNames(raw map[string]interface{}) map[string]string {
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")
	names := make(map[string]string, len(result.TriggeredRules))
	for _, r := range result.TriggeredRules {
		names[r.RuleName] = r.Severity
	}
	return names
}

// ---- hadr_always_on: long_running_tx_count ----

func TestFeatureAGLongRunningTxNotFiredWhenAbsent(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load": 10.0,
	}
	rules := ruleNames(raw)
	if _, ok := rules["feature_ag_long_running_tx"]; ok {
		t.Error("expected feature_ag_long_running_tx NOT to fire when long_running_tx_count is absent")
	}
}

func TestFeatureAGLongRunningTxNotFiredBelowWarning(t *testing.T) {
	raw := map[string]interface{}{
		"long_running_tx_count": 3.0, // below warning threshold of 5
	}
	rules := ruleNames(raw)
	if _, ok := rules["feature_ag_long_running_tx"]; ok {
		t.Error("expected feature_ag_long_running_tx NOT to fire below warning threshold (5)")
	}
}

func TestFeatureAGLongRunningTxWarning(t *testing.T) {
	raw := map[string]interface{}{
		"long_running_tx_count": 7.0, // >= warning (5), < critical (15)
	}
	rules := ruleNames(raw)
	sev, ok := rules["feature_ag_long_running_tx"]
	if !ok {
		t.Fatal("expected feature_ag_long_running_tx to fire at warning level")
	}
	if sev != "medium" {
		t.Errorf("expected severity=medium, got %s", sev)
	}
}

func TestFeatureAGLongRunningTxCritical(t *testing.T) {
	raw := map[string]interface{}{
		"long_running_tx_count": 20.0, // >= critical (15)
	}
	rules := ruleNames(raw)
	sev, ok := rules["feature_ag_long_running_tx"]
	if !ok {
		t.Fatal("expected feature_ag_long_running_tx to fire at critical level")
	}
	if sev != "critical" {
		t.Errorf("expected severity=critical, got %s", sev)
	}
}

// ---- memory_configuration: available_physical_memory_kb ----

func TestFeatureAvailMemoryNotFiredWhenAbsent(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load": 10.0,
	}
	rules := ruleNames(raw)
	if _, ok := rules["feature_avail_memory_critical"]; ok {
		t.Error("expected feature_avail_memory_critical NOT to fire when metric is absent")
	}
	if _, ok := rules["feature_avail_memory_warning"]; ok {
		t.Error("expected feature_avail_memory_warning NOT to fire when metric is absent")
	}
}

func TestFeatureAvailMemoryNotFiredAboveWarning(t *testing.T) {
	// 2 GB available — above warning threshold (1024 MB)
	raw := map[string]interface{}{
		"available_physical_memory_kb": float64(2 * 1024 * 1024),
	}
	rules := ruleNames(raw)
	if _, ok := rules["feature_avail_memory_warning"]; ok {
		t.Error("expected feature_avail_memory_warning NOT to fire when 2 GB available")
	}
}

func TestFeatureAvailMemoryWarning(t *testing.T) {
	// 800 MB available: < warning (1024 MB) but > critical (512 MB)
	raw := map[string]interface{}{
		"available_physical_memory_kb": float64(800 * 1024),
	}
	rules := ruleNames(raw)
	sev, ok := rules["feature_avail_memory_warning"]
	if !ok {
		t.Fatal("expected feature_avail_memory_warning to fire with 800 MB available")
	}
	if sev != "medium" {
		t.Errorf("expected severity=medium, got %s", sev)
	}
}

func TestFeatureAvailMemoryCritical(t *testing.T) {
	// 256 MB available: below critical threshold (512 MB)
	raw := map[string]interface{}{
		"available_physical_memory_kb": float64(256 * 1024),
	}
	rules := ruleNames(raw)
	sev, ok := rules["feature_avail_memory_critical"]
	if !ok {
		t.Fatal("expected feature_avail_memory_critical to fire with 256 MB available")
	}
	if sev != "critical" {
		t.Errorf("expected severity=critical, got %s", sev)
	}
}

// ---- parallelism: cxpacket_wait_pct ----

func TestFeatureCXPacketNotFiredWhenAbsent(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load": 10.0,
	}
	rules := ruleNames(raw)
	if _, ok := rules["feature_cxpacket_pressure"]; ok {
		t.Error("expected feature_cxpacket_pressure NOT to fire when cxpacket_wait_pct is absent")
	}
}

func TestFeatureCXPacketNotFiredBelowWarning(t *testing.T) {
	raw := map[string]interface{}{
		"cxpacket_wait_pct": 10.0, // below warning threshold of 15
	}
	rules := ruleNames(raw)
	if _, ok := rules["feature_cxpacket_pressure"]; ok {
		t.Error("expected feature_cxpacket_pressure NOT to fire below warning threshold (15%)")
	}
}

func TestFeatureCXPacketWarning(t *testing.T) {
	raw := map[string]interface{}{
		"cxpacket_wait_pct": 20.0, // >= warning (15), < critical (30)
	}
	rules := ruleNames(raw)
	sev, ok := rules["feature_cxpacket_pressure"]
	if !ok {
		t.Fatal("expected feature_cxpacket_pressure to fire at warning level")
	}
	if sev != "medium" {
		t.Errorf("expected severity=medium, got %s", sev)
	}
}

func TestFeatureCXPacketCritical(t *testing.T) {
	raw := map[string]interface{}{
		"cxpacket_wait_pct": 35.0, // >= critical (30)
	}
	rules := ruleNames(raw)
	sev, ok := rules["feature_cxpacket_pressure"]
	if !ok {
		t.Fatal("expected feature_cxpacket_pressure to fire at critical level")
	}
	if sev != "critical" {
		t.Errorf("expected severity=critical, got %s", sev)
	}
}

// ---- sql_agent_jobs: critical_jobs_disabled ----

func TestFeatureCriticalJobsDisabledNotFiredWhenAbsent(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load": 10.0,
	}
	rules := ruleNames(raw)
	if _, ok := rules["feature_critical_jobs_disabled"]; ok {
		t.Error("expected feature_critical_jobs_disabled NOT to fire when metric is absent")
	}
}

func TestFeatureCriticalJobsDisabledNotFiredAtZero(t *testing.T) {
	raw := map[string]interface{}{
		"critical_jobs_disabled": 0.0,
	}
	rules := ruleNames(raw)
	if _, ok := rules["feature_critical_jobs_disabled"]; ok {
		t.Error("expected feature_critical_jobs_disabled NOT to fire when count is 0")
	}
}

func TestFeatureCriticalJobsDisabledHigh(t *testing.T) {
	raw := map[string]interface{}{
		"critical_jobs_disabled": 1.0, // >= warning (1), < critical (3)
	}
	rules := ruleNames(raw)
	sev, ok := rules["feature_critical_jobs_disabled"]
	if !ok {
		t.Fatal("expected feature_critical_jobs_disabled to fire with 1 disabled job")
	}
	if sev != "high" {
		t.Errorf("expected severity=high, got %s", sev)
	}
}

func TestFeatureCriticalJobsDisabledCritical(t *testing.T) {
	raw := map[string]interface{}{
		"critical_jobs_disabled": 4.0, // >= critical (3)
	}
	rules := ruleNames(raw)
	sev, ok := rules["feature_critical_jobs_disabled"]
	if !ok {
		t.Fatal("expected feature_critical_jobs_disabled to fire at critical level")
	}
	if sev != "critical" {
		t.Errorf("expected severity=critical, got %s", sev)
	}
}

// ---- tempdb_configuration: data file count (Phase B) ----

func TestFeatureTempDBFilesNotFiredWhenAbsent(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load": 10.0,
	}
	rules := ruleNames(raw)
	if _, ok := rules["feature_tempdb_files_critical"]; ok {
		t.Error("expected feature_tempdb_files_critical NOT to fire when tempdb_data_files_count is absent")
	}
	if _, ok := rules["feature_tempdb_files_warning"]; ok {
		t.Error("expected feature_tempdb_files_warning NOT to fire when tempdb_data_files_count is absent")
	}
}

func TestFeatureTempDBFilesNotFiredAtOrAboveWarning(t *testing.T) {
	raw := map[string]interface{}{
		"tempdb_data_files_count": 4.0, // at warning threshold — should NOT fire
	}
	rules := ruleNames(raw)
	if _, ok := rules["feature_tempdb_files_warning"]; ok {
		t.Error("expected feature_tempdb_files_warning NOT to fire at exactly 4 files")
	}
}

func TestFeatureTempDBFilesWarning(t *testing.T) {
	raw := map[string]interface{}{
		"tempdb_data_files_count": 3.0, // < warning (4), > critical (2)
	}
	rules := ruleNames(raw)
	sev, ok := rules["feature_tempdb_files_warning"]
	if !ok {
		t.Fatal("expected feature_tempdb_files_warning to fire with 3 data files")
	}
	if sev != "medium" {
		t.Errorf("expected severity=medium, got %s", sev)
	}
}

func TestFeatureTempDBFilesCritical(t *testing.T) {
	raw := map[string]interface{}{
		"tempdb_data_files_count": 1.0, // < critical (2)
	}
	rules := ruleNames(raw)
	sev, ok := rules["feature_tempdb_files_critical"]
	if !ok {
		t.Fatal("expected feature_tempdb_files_critical to fire with only 1 data file")
	}
	if sev != "critical" {
		t.Errorf("expected severity=critical, got %s", sev)
	}
}

// ---- no phantom rules when all feature metrics are absent ----

func TestFeatureLibraryNoPhantomRulesOnEmptyRawData(t *testing.T) {
	raw := map[string]interface{}{}
	rules := ruleNames(raw)
	featureRules := []string{
		"feature_ag_long_running_tx",
		"feature_avail_memory_critical",
		"feature_avail_memory_warning",
		"feature_cxpacket_pressure",
		"feature_critical_jobs_disabled",
		"feature_tempdb_files_critical",
		"feature_tempdb_files_warning",
	}
	for _, name := range featureRules {
		if _, ok := rules[name]; ok {
			t.Errorf("expected %s NOT to fire on empty rawData (phantom rule)", name)
		}
	}
}

// ---- feature library helper functions ----

func TestGetFeatureThresholdHADR(t *testing.T) {
	ft := analysis.GetFeatureThreshold("hadr_always_on", "log_send_queue_kb")
	if ft == nil {
		t.Fatal("expected hadr_always_on.log_send_queue_kb threshold to exist")
	}
}

func TestGetFeatureThresholdMemory(t *testing.T) {
	ft := analysis.GetFeatureThreshold("memory_configuration", "ple_seconds_below")
	if ft == nil {
		t.Fatal("expected memory_configuration.ple_seconds_below threshold to exist")
	}
}

func TestGetFeatureThresholdParallelism(t *testing.T) {
	ft := analysis.GetFeatureThreshold("parallelism", "cxpacket_wait_pct_above")
	if ft == nil {
		t.Fatal("expected parallelism.cxpacket_wait_pct_above threshold to exist")
	}
}

func TestGetFeatureThresholdSQLAgent(t *testing.T) {
	ft := analysis.GetFeatureThreshold("sql_agent_jobs", "job_failures_24h")
	if ft == nil {
		t.Fatal("expected sql_agent_jobs.job_failures_24h threshold to exist")
	}
}

func TestGetFeatureThresholdMissing(t *testing.T) {
	ft := analysis.GetFeatureThreshold("nonexistent_feature", "some_check")
	if ft != nil {
		t.Error("expected nil for nonexistent feature threshold")
	}
}

func TestGetAllFeatureNamesContainsPhaseAFeatures(t *testing.T) {
	names := analysis.GetAllFeatureNames()
	if len(names) < 10 {
		t.Errorf("expected at least 10 feature names, got %d", len(names))
	}
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}
	required := []string{"hadr_always_on", "memory_configuration", "parallelism", "sql_agent_jobs", "tempdb_configuration"}
	for _, r := range required {
		if !nameSet[r] {
			t.Errorf("expected feature library to contain %q", r)
		}
	}
}
