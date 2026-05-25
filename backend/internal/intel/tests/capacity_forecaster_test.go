// Package tests provides integration tests for the SQL Optima intelligence engine.
// This file covers the capacity forecaster (DEFECT-12, DEFECT-13 fixes).
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT

package tests

import (
	"testing"
	"time"

	"github.com/rsharma155/sql_optima/internal/intel/forecasting"
)

func makeCapacityTimeline(days int, startUsedMB, dailyGrowthMB float64) []forecasting.InstanceCapacityPoint {
	points := make([]forecasting.InstanceCapacityPoint, days)
	now := time.Now().UTC()
	for i := 0; i < days; i++ {
		usedMB := startUsedMB + float64(i)*dailyGrowthMB
		freeMB := 1000*1024 - usedMB // 1 TB total disk
		points[i] = forecasting.InstanceCapacityPoint{
			CaptureTimestamp: now.AddDate(0, 0, -(days - i)),
			DataDiskMB:       usedMB * 0.8,
			LogDiskMB:        usedMB * 0.2,
			FreeDiskMB:       freeMB,
			UsedPct:          usedMB / (1000 * 1024) * 100,
		}
	}
	return points
}

func TestForecastCapacitySteadyGrowth(t *testing.T) {
	// 1 GB/day steady growth on a 1 TB disk, starting at 400 GB used
	timeline := makeCapacityTimeline(30, 400*1024, 1024)
	result := forecasting.ForecastCapacity(nil, timeline)
	if result == nil {
		t.Fatal("expected non-nil capacity forecast")
	}
	if result.GrowthPattern != "Steady" {
		t.Errorf("expected Steady growth pattern, got %q", result.GrowthPattern)
	}
	if result.DailyGrowthMBAvg < 900 || result.DailyGrowthMBAvg > 1100 {
		t.Errorf("expected daily growth ~1024 MB, got %.1f", result.DailyGrowthMBAvg)
	}
	if result.Projected30dMB <= result.UsedDiskMB {
		t.Error("30-day projection should be greater than current usage")
	}
	if result.DaysUntilBreach <= 0 {
		t.Error("expected a breach date with steady 1 GB/day growth on 1 TB disk from 400 GB")
	}
}

func TestForecastCapacityNoGrowth(t *testing.T) {
	// Flat usage — no growth
	timeline := makeCapacityTimeline(30, 200*1024, 0)
	result := forecasting.ForecastCapacity(nil, timeline)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// No growth → breach date should be -1 (never)
	if result.DaysUntilBreach > 0 {
		t.Errorf("expected no breach date for flat usage, got %d days", result.DaysUntilBreach)
	}
}

func TestForecastCapacityNilTimeline(t *testing.T) {
	result := forecasting.ForecastCapacity(nil, nil)
	if result != nil {
		t.Error("expected nil for empty timeline")
	}
}

// DEFECT-12: growth per single collection cycle must NOT be used directly as daily rate.
// The correct daily rate is computed from time elapsed between adjacent timeline points.
func TestForecastCapacityRateIsPerDay(t *testing.T) {
	// 10 MB per collection cycle (15 min) would be 10 × 96 = 960 MB/day.
	// The capacity forecaster should derive the rate from elapsed time, not fixed cycles.
	timeline := makeCapacityTimeline(14, 100*1024, 10*96) // 960 MB/day built into timeline
	result := forecasting.ForecastCapacity(nil, timeline)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Daily rate should be ~960 MB ± 10%
	if result.DailyGrowthMBAvg < 800 || result.DailyGrowthMBAvg > 1100 {
		t.Errorf("expected daily growth ~960 MB, got %.1f MB", result.DailyGrowthMBAvg)
	}
}

func TestReliabilityTierFromR2(t *testing.T) {
	// Thresholds per spec §8.3: >0.80 = Reliable, 0.50–0.80 = Indicative, <0.50 = Unreliable.
	cases := []struct {
		r2       float64
		expected string
	}{
		{0.90, "Reliable"},
		{0.81, "Reliable"},
		{0.80, "Indicative"}, // boundary: >0.80 required, 0.80 is not strictly >
		{0.70, "Indicative"},
		{0.50, "Indicative"},
		{0.49, "Unreliable"},
		{0.30, "Unreliable"},
		{0.00, "Unreliable"},
	}
	for _, tc := range cases {
		got := forecasting.ReliabilityTierFromR2(tc.r2)
		if got != tc.expected {
			t.Errorf("R²=%.2f: expected %q, got %q", tc.r2, tc.expected, got)
		}
	}
}
