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

	"github.com/rsharma155/sql_optima/internal/intel/forecasting"
)

func TestLinearForecastWithGrowth(t *testing.T) {
	values := []float64{10, 12, 14, 16, 18, 20, 22, 24, 26, 28}
	result := forecasting.ForecastLinear(values, "test_metric", 10, 0.95)
	if result.MetricName != "test_metric" {
		t.Error("expected test_metric, got ", result.MetricName)
	}
	if len(result.Points) != 10 {
		t.Error("expected 10 points, got 0", len(result.Points))
	}
	if result.Confidence <= 0 {
		t.Error("expected positive confidence")
	}
	if result.TrendDirection != "increasing" {
		t.Error("expected increasing trend, got ", result.TrendDirection)
	}
}

func TestLinearForecastWithDecline(t *testing.T) {
	values := []float64{100, 95, 90, 85, 80, 75, 70, 65, 60, 55}
	result := forecasting.ForecastLinear(values, "test_decline", 10, 0.95)
	if result.TrendDirection != "decreasing" {
		t.Error("expected decreasing trend, got ", result.TrendDirection)
	}
}

func TestLinearForecastInsufficientData(t *testing.T) {
	values := []float64{1, 2}
	result := forecasting.ForecastLinear(values, "short", 10, 0.95)
	if result.Confidence != 0.0 {
		t.Error("expected zero confidence for insufficient data")
	}
	if len(result.Points) != 0 {
		t.Error("expected zero points for insufficient data")
	}
}

func TestExponentialSmoothing(t *testing.T) {
	values := []float64{10, 12, 15, 14, 16, 18, 20, 22, 25, 24}
	result := forecasting.ForecastExponentialSmoothing(values, "test_exp", 10, 0.3)
	if len(result.Points) != 10 {
		t.Error("expected 10 points, got 0", len(result.Points))
	}
}

func TestForecastReturnType(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := forecasting.ForecastLinear(values, "linear", 5, 0.95)
	for _, point := range result.Points {
		if point.Value < 0 {
			t.Error("expected non-negative value")
		}
	}
}

// DEFECT-10: low-confidence forecasts must have a reliability tier of "Unreliable".
func TestForecastReliabilityTierUnreliable(t *testing.T) {
	// Random-looking data produces low R² → "Unreliable"
	values := []float64{10, 90, 5, 80, 15, 70, 20, 85, 8, 60}
	result := forecasting.ForecastLinear(values, "noisy_metric", 30, 0.95)
	if result.ReliabilityTier != "Unreliable" && result.ReliabilityTier != "Indicative" && result.ReliabilityTier != "Reliable" {
		t.Errorf("expected ReliabilityTier to be Unreliable/Indicative/Reliable, got %q", result.ReliabilityTier)
	}
	// For highly noisy data, must be Unreliable
	if result.Confidence < 0.5 && result.ReliabilityTier != "Unreliable" {
		t.Errorf("low-confidence forecast (%.2f) should be Unreliable, got %q", result.Confidence, result.ReliabilityTier)
	}
}

// DEFECT-10: high-confidence forecasts must have "Reliable" tier.
func TestForecastReliabilityTierReliable(t *testing.T) {
	// Perfectly linear data → R² = 1.0 → "Reliable"
	values := []float64{10, 12, 14, 16, 18, 20, 22, 24, 26, 28}
	result := forecasting.ForecastLinear(values, "linear_metric", 30, 0.95)
	if result.ReliabilityTier != "Reliable" {
		t.Errorf("perfect linear data should yield Reliable tier, got %q (confidence %.2f)", result.ReliabilityTier, result.Confidence)
	}
}