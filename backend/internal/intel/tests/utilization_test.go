// Package tests provides integration tests for the SQL Optima intelligence engine.
// This file covers utilization classification, peak window detection, and wait analysis.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT

package tests

import (
	"testing"
	"time"

	intelanalysis "github.com/rsharma155/sql_optima/internal/intel/analysis"
	"github.com/rsharma155/sql_optima/internal/models"
)

// ---- UtilizationClassifier Tests ----

func TestClassifyUnderUtilized(t *testing.T) {
	p := map[string]float64{
		"cpu_p50": 5, "cpu_p90": 15, "cpu_p95": 20, "cpu_p99": 28, "cpu_avg": 8,
		"mem_p50": 30, "mem_p95": 40, "mem_avg": 32,
		"sample_count": 500,
	}
	result := intelanalysis.ClassifyUtilization(p)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Class != "Under-utilized" {
		t.Errorf("expected Under-utilized, got %q (cpu_p95=20, mem_p95=40)", result.Class)
	}
}

func TestClassifyOptimal(t *testing.T) {
	p := map[string]float64{
		"cpu_p50": 35, "cpu_p90": 50, "cpu_p95": 55, "cpu_p99": 65, "cpu_avg": 38,
		"mem_p50": 60, "mem_p95": 70, "mem_avg": 62,
		"sample_count": 500,
	}
	result := intelanalysis.ClassifyUtilization(p)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Class != "Optimal" {
		t.Errorf("expected Optimal, got %q (cpu_p95=55, mem_p95=70)", result.Class)
	}
}

func TestClassifyOverUtilized(t *testing.T) {
	p := map[string]float64{
		"cpu_p50": 75, "cpu_p90": 88, "cpu_p95": 92, "cpu_p99": 98, "cpu_avg": 78,
		"mem_p50": 88, "mem_p95": 95, "mem_avg": 90,
		"sample_count": 500,
	}
	result := intelanalysis.ClassifyUtilization(p)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Class != "Over-utilized" {
		t.Errorf("expected Over-utilized, got %q (cpu_p95=92, mem_p95=95)", result.Class)
	}
}

func TestClassifyInsufficientSamples(t *testing.T) {
	p := map[string]float64{
		"cpu_p95": 50, "mem_p95": 60, "sample_count": 5,
	}
	result := intelanalysis.ClassifyUtilization(p)
	if result != nil {
		t.Error("expected nil result for < 10 samples")
	}
}

func TestClassifyNilPercentiles(t *testing.T) {
	result := intelanalysis.ClassifyUtilization(nil)
	if result != nil {
		t.Error("expected nil result for nil percentiles")
	}
}

// ---- PeakWindowDetector Tests ----

func TestDetectPeakWindowBasic(t *testing.T) {
	// Build a 7×24 grid: weekdays 9–11 have high CPU, rest is low.
	var slots []models.HourlySlot
	base := time.Now()
	_ = base
	for dow := 1; dow <= 7; dow++ {
		for hour := 0; hour < 24; hour++ {
			cpu := 10.0
			if dow <= 5 && hour >= 9 && hour < 11 {
				cpu = 85.0
			}
			slots = append(slots, models.HourlySlot{
				DayOfWeek:   dow,
				HourOfDay:   hour,
				AvgCPUPct:   cpu,
				SampleCount: 30,
			})
		}
	}
	result := intelanalysis.DetectPeakWindow(slots)
	if result == nil {
		t.Fatal("expected non-nil peak window")
	}
	if len(result.PeakDays) == 0 {
		t.Error("expected peak days to be populated")
	}
	if result.PeakCPUAvg < 50 {
		t.Errorf("expected peak CPU avg > 50, got %.1f", result.PeakCPUAvg)
	}
}

func TestDetectPeakWindowInsufficientData(t *testing.T) {
	slots := []models.HourlySlot{{DayOfWeek: 1, HourOfDay: 9, AvgCPUPct: 50}}
	result := intelanalysis.DetectPeakWindow(slots)
	if result != nil {
		t.Error("expected nil result with < 24 slots")
	}
}

// ---- WaitAnalyzer Tests ----

func TestAnalyzeWaitTypesCXPACKET(t *testing.T) {
	rows := []intelanalysis.WaitRawRow{
		{WaitType: "CXPACKET", WaitCategory: "Parallelism", TotalWaitMS: 50000, PctOfTotal: 38, Rank: 1},
		{WaitType: "PAGEIOLATCH_SH", WaitCategory: "I/O", TotalWaitMS: 25000, PctOfTotal: 19, Rank: 2},
	}
	result := intelanalysis.AnalyzeWaitTypes(rows)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if result[0].Category != "Parallelism" {
		t.Errorf("expected CXPACKET to be Parallelism, got %q", result[0].Category)
	}
	if result[0].WaitScore != 76 {
		// 38% × 2 = 76
		t.Errorf("expected WaitScore=76, got %.1f", result[0].WaitScore)
	}
	if result[1].Category != "IO" {
		t.Errorf("expected PAGEIOLATCH_SH to be IO, got %q", result[1].Category)
	}
}

func TestAnalyzeWaitTypesCapped(t *testing.T) {
	rows := []intelanalysis.WaitRawRow{
		{WaitType: "LCK_M_X", WaitCategory: "Lock", TotalWaitMS: 100000, PctOfTotal: 60, Rank: 1},
	}
	result := intelanalysis.AnalyzeWaitTypes(rows)
	if result[0].WaitScore != 100 {
		t.Errorf("expected WaitScore capped at 100, got %.1f (60%% × 2 = 120, capped to 100)", result[0].WaitScore)
	}
}

func TestAnalyzeWaitTypesEmpty(t *testing.T) {
	result := intelanalysis.AnalyzeWaitTypes(nil)
	if result != nil {
		t.Error("expected nil for empty input")
	}
}
