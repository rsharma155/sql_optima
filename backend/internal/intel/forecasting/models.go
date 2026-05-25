// Package intel provides the SQL Optima autonomous health intelligence engine.
// This file defines the forecasting domain types.
//
// Design context:
//   - DEFECT-10 fix: ForecastSeries gains ReliabilityTier so callers can suppress
//     unreliable projections from UI display.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT

package forecasting

import "time"

type ForecastPoint struct {
	Timestamp  time.Time `json:"timestamp"`
	Value      float64   `json:"value"`
	UpperBound *float64  `json:"upper_bound,omitempty"`
	LowerBound *float64  `json:"lower_bound,omitempty"`
}

// ForecastSeries is the output of a single metric forecast.
// ReliabilityTier classifies the forecast quality:
//   - "Reliable"    R² > 0.70  — show day-level precision
//   - "Indicative"  R² 0.30–0.70 — show week-level precision
//   - "Unreliable"  R² < 0.30  — show direction only, suppress date estimates
type ForecastSeries struct {
	MetricName           string          `json:"metric_name"`
	HorizonDays          int             `json:"horizon_days"`
	Points               []ForecastPoint `json:"points"`
	Confidence           float64         `json:"confidence"`
	PredictedFailureDays *int            `json:"predicted_failure_days,omitempty"`
	TrendDirection       string          `json:"trend_direction"`
	GrowthRatePct        float64         `json:"growth_rate_pct"`
	ReliabilityTier      string          `json:"reliability_tier"` // "Reliable" | "Indicative" | "Unreliable"
}

// ReliabilityTierFromR2 returns the reliability tier string for a given R² value.
// Thresholds from spec §8.3: >0.80 = Reliable, 0.50–0.80 = Indicative, <0.50 = Unreliable.
func ReliabilityTierFromR2(r2 float64) string {
	switch {
	case r2 > 0.80:
		return "Reliable"
	case r2 >= 0.50:
		return "Indicative"
	default:
		return "Unreliable"
	}
}
