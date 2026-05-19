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