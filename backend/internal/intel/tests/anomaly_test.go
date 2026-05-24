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

	"github.com/rsharma155/sql_optima/internal/intel/anomaly"
)

func TestPointAnomalyDetected(t *testing.T) {
	detector := anomaly.NewAnomalyDetector(2.5)
	series := []float64{10, 12, 11, 13, 12, 11, 10, 12, 11, 13}
	isAnomaly, score := detector.DetectPointAnomaly(100, series)
	if !isAnomaly {
		t.Error("expected anomaly detection")
	}
	if score <= 2.5 {
		t.Error("expected score > 2.5, got 0.00", score)
	}
}

func TestNoAnomalyInNormalData(t *testing.T) {
	detector := anomaly.NewAnomalyDetector(2.5)
	series := []float64{10, 12, 11, 13, 12, 11, 10, 12, 11, 13}
	isAnomaly, score := detector.DetectPointAnomaly(12, series)
	if isAnomaly {
		t.Error("expected no anomaly")
	}
	if score >= 2.5 {
		t.Error("expected score < 2.5, got 0.00", score)
	}
}

func TestInsufficientData(t *testing.T) {
	detector := anomaly.NewAnomalyDetector(2.5)
	isAnomaly, score := detector.DetectPointAnomaly(10, []float64{1, 2, 3})
	if isAnomaly {
		t.Error("expected no anomaly with insufficient data")
	}
	if score != 0.0 {
		t.Error("expected 0, got 0.00", score)
	}
}

func TestTrendAnomalyDetected(t *testing.T) {
	detector := anomaly.NewAnomalyDetector(2.5)
	series := []float64{10, 12, 11, 13, 12, 90, 95, 92, 98, 95}
	if !detector.DetectTrendAnomaly(series) {
		t.Error("expected trend anomaly detection")
	}
}

func TestTrendAnomalyNotDetected(t *testing.T) {
	detector := anomaly.NewAnomalyDetector(2.5)
	series := []float64{50, 52, 51, 53, 52, 50, 49, 51, 50, 52}
	if detector.DetectTrendAnomaly(series) {
		t.Error("expected no trend anomaly")
	}
}

// DEFECT-8: a constant-zero series must NOT trigger a false anomaly on floating-point noise.
func TestConstantZeroSeriesNoAnomaly(t *testing.T) {
	detector := anomaly.NewAnomalyDetector(2.5)
	// Simulates memory_grants_pending=0 for days — common stable state
	series := make([]float64, 20)
	isAnomaly, _ := detector.DetectPointAnomaly(0.0, series)
	if isAnomaly {
		t.Error("constant-zero series with 0.0 value should NOT be an anomaly")
	}
	// Tiny floating-point noise should also not trigger
	isAnomaly2, _ := detector.DetectPointAnomaly(0.000001, series)
	if isAnomaly2 {
		t.Error("constant-zero series with epsilon noise should NOT be an anomaly")
	}
}

// DEFECT-8: a meaningful spike into a zero-baseline should be flagged.
func TestMeaningfulSpikeIntoZeroBaselineIsAnomaly(t *testing.T) {
	detector := anomaly.NewAnomalyDetector(2.5)
	series := make([]float64, 20)
	// 5 pending grants when baseline is always 0 — that's meaningful
	isAnomaly, _ := detector.DetectPointAnomaly(5.0, series)
	if !isAnomaly {
		t.Error("5 grants pending against zero baseline should be flagged as anomaly")
	}
}