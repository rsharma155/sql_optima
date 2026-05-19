// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package tests




import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/intel/analysis"
	"github.com/rsharma155/sql_optima/internal/models"
)

func TestThresholdsComputedFromConfig(t *testing.T) {
	config := models.ServerConfig{CPUCount: 16, TotalRAMGB: 64, TotalDiskGB: 1000}
	calc := analysis.NewDynamicThresholdCalculator()
	th := calc.Compute(config, nil)
	if th.CPUSustainedThreshold <= 0 {
		t.Error("expected cpu_sustained_threshold > 0")
	}
	if th.MemoryPLEMinSeconds < 120 {
		t.Error("expected memory_ple_min_seconds >= 120")
	}
	if th.DiskFreeMBMin <= 0 {
		t.Error("expected disk_free_mb_min > 0")
	}
	if th.CPURunnableAbsolute <= 0 {
		t.Error("expected cpu_runnable_absolute > 0")
	}
}

func TestCPUThresholdScalesWithCores(t *testing.T) {
	small := models.ServerConfig{CPUCount: 4, TotalRAMGB: 16}
	large := models.ServerConfig{CPUCount: 64, TotalRAMGB: 256}
	calc := analysis.NewDynamicThresholdCalculator()
	tSmall := calc.Compute(small, nil)
	tLarge := calc.Compute(large, nil)
	if tLarge.CPURunnableAbsolute <= tSmall.CPURunnableAbsolute {
		t.Error("expected larger cpu_runnable_absolute for more cores")
	}
}

func TestPLEThresholdScalesWithRAM(t *testing.T) {
	small := models.ServerConfig{CPUCount: 8, TotalRAMGB: 16}
	large := models.ServerConfig{CPUCount: 8, TotalRAMGB: 256}
	calc := analysis.NewDynamicThresholdCalculator()
	tSmall := calc.Compute(small, nil)
	tLarge := calc.Compute(large, nil)
	if tLarge.MemoryPLEMinSeconds < tSmall.MemoryPLEMinSeconds {
		t.Error("expected larger memory_ple_min_seconds for more RAM")
	}
}

func TestEngineAnalyzesRawData(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load":               45.0,
		"memory_usage":               72.0,
		"ple_seconds":                850.0,
		"free_disk_mb":               45000.0,
		"buffer_cache_hit_ratio":     98.5,
		"secondary_lag_seconds":      5.0,
		"blocking_sessions":          0.0,
		"tempdb_used_percent":        35.0,
		"failed_jobs_24h":            0.0,
		"total_runnable_tasks_count": 3.0,
		"batch_requests_per_sec":     800.0,
		"read_latency_ms":            5.0,
		"write_latency_ms":           3.0,
		"cpu_count":                  8.0,
		"total_ram_gb":               32.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")
	if result.RunID == "" {
		t.Error("expected non-empty run_id")
	}
	if result.OverallRisk.OverallScore < 0 || result.OverallRisk.OverallScore > 100 {
		t.Error("expected score 0-100, got 0.0", result.OverallRisk.OverallScore)
	}
}

func TestEngineDetectsHighCPU(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load":               95.0,
		"total_runnable_tasks_count": 30.0,
		"cpu_count":                  4.0,
		"total_ram_gb":               16.0,
		"free_disk_mb":               50000.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")
	found := false
	for _, r := range result.TriggeredRules {
		if r.RuleName == "cpu_saturation" || r.RuleName == "scheduler_starvation" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected cpu-related rule triggered")
	}
}

func TestEngineDetectsLowPLE(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load": 30.0,
		"ple_seconds":  45.0,
		"ple":          45.0,
		"total_ram_gb": 32.0,
		"cpu_count":    8.0,
		"free_disk_mb": 50000.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")
	found := false
	for _, r := range result.TriggeredRules {
		if r.RuleName == "ple_collapse" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ple_collapse rule triggered")
	}
}

func TestEngineDetectsLowDisk(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load":  30.0,
		"free_disk_mb":  2000.0,
		"free_mb":       2000.0,
		"cpu_count":     8.0,
		"total_ram_gb":  32.0,
		"total_disk_gb": 100.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")
	found := false
	for _, r := range result.TriggeredRules {
		if r.RuleName == "low_disk_space" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected low_disk_space rule triggered")
	}
}

func TestEngineDetectsReplicationLag(t *testing.T) {
	raw := map[string]interface{}{
		"secondary_lag_seconds": 300.0,
		"cpu_count":             8.0,
		"total_ram_gb":          32.0,
		"free_disk_mb":          50000.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")
	found := false
	for _, r := range result.TriggeredRules {
		if r.RuleName == "replication_lag" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected replication_lag rule triggered")
	}
}

func TestEngineDetectsBlocking(t *testing.T) {
	raw := map[string]interface{}{
		"blocking_sessions": 15.0,
		"cpu_count":          8.0,
		"total_ram_gb":       32.0,
		"free_disk_mb":       50000.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")
	found := false
	for _, r := range result.TriggeredRules {
		if r.RuleName == "blocking_chains" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected blocking_chains rule triggered")
	}
}

func TestEngineHealthyGeneratesWorkingWell(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load":          25.0,
		"ple_seconds":           2500.0,
		"buffer_cache_hit_ratio": 99.0,
		"free_disk_mb":          200000.0,
		"secondary_lag_seconds": 2.0,
		"failed_jobs_24h":       0.0,
		"blocking_sessions":     0.0,
		"tempdb_used_percent":   20.0,
		"cpu_count":             16.0,
		"total_ram_gb":          128.0,
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")
	if len(result.WorkingWell) == 0 {
		t.Error("expected working_well entries")
	}
}

func TestEngineGeneratesForecasts(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load":            50.0,
		"free_disk_mb":            50000.0,
		"memory_usage":            65.0,
		"cpu_count":               8.0,
		"total_ram_gb":            32.0,
		"avg_cpu_load_series":     []float64{42, 42.5, 43, 43.5, 44, 44.5, 45, 45.5, 46, 46.5, 47, 47.5, 48, 48.5, 49, 49.5, 50, 50.5, 51, 51.5},
		"free_disk_mb_series":     []float64{50000, 49900, 49800, 49700, 49600, 49500, 49400, 49300, 49200, 49100, 49000, 48900, 48800, 48700, 48600, 48500, 48400, 48300, 48200, 48100},
	}
	engine := analysis.NewAnalysisEngine()
	result := engine.Analyze(raw, nil, "")
	if len(result.Forecasts) == 0 {
		t.Error("expected at least one forecast")
	}
}

func TestDefaultServerConfigMapping(t *testing.T) {
	raw := map[string]interface{}{
		"cpu_count":         16.0,
		"physical_memory_gb": 64.0,
		"max_workers_count":  2048.0,
		"total_disk_gb":      2000.0,
		"hyperthread_ratio":  2.0,
		"socket_count":       4.0,
		"cores_per_socket":   8.0,
		"numa_nodes":         4.0,
		"is_virtual":         false,
	}
	cfg := analysis.DefaultServerConfig(raw)
	if cfg.CPUCount != 16 {
		t.Error("expected CPUCount=16, got 0", cfg.CPUCount)
	}
	if cfg.TotalRAMGB != 64 {
		t.Error("expected TotalRAMGB=64, got 0", cfg.TotalRAMGB)
	}
	if cfg.MaxWorkers != 2048 {
		t.Error("expected MaxWorkers=2048, got 0", cfg.MaxWorkers)
	}
	if cfg.TotalDiskGB != 2000 {
		t.Error("expected TotalDiskGB=2000, got 0", cfg.TotalDiskGB)
	}
	if cfg.HyperthreadRatio != 2 {
		t.Error("expected HyperthreadRatio=2, got 0", cfg.HyperthreadRatio)
	}
	if cfg.SocketCount != 4 {
		t.Error("expected SocketCount=4, got 0", cfg.SocketCount)
	}
	if cfg.CoresPerSocket != 8 {
		t.Error("expected CoresPerSocket=8, got 0", cfg.CoresPerSocket)
	}
	if cfg.NumaNodes != 4 {
		t.Error("expected NumaNodes=4, got 0", cfg.NumaNodes)
	}
	if cfg.IsVirtual != false {
		t.Error("expected IsVirtual=false")
	}
}

func TestHistoriesExtracted(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load_series": []float64{1, 2, 3},
		"memory_usage_series": []float64{4, 5, 6},
		"plain_value":         42,
	}
	h := analysis.ExtractHistoriesFromRaw(raw)
	if _, ok := h["avg_cpu_load"]; !ok {
		t.Error("expected avg_cpu_load in histories")
	}
	if _, ok := h["plain_value"]; ok {
		t.Error("did not expect plain_value in histories")
	}
}

func TestHistoriesExtractedFromInterfaceSlice(t *testing.T) {
	raw := map[string]interface{}{
		"avg_cpu_load_series": []interface{}{1.0, 2.0, 3.0},
	}
	h := analysis.ExtractHistoriesFromRaw(raw)
	if _, ok := h["avg_cpu_load"]; !ok {
		t.Error("expected avg_cpu_load from []interface{}")
	}
}