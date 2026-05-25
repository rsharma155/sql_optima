// Package intel provides the SQL Optima autonomous health intelligence engine.
// This file implements statistical anomaly detection on metric time series.
//
// Design context:
//   - DEFECT-8 fix: constant-zero series (σ < 1e-9, mean ≈ 0) no longer triggers false anomalies
//     from floating-point noise. A meaningful business floor (1.0 unit) is required before flagging.
//   - Supports both time-naive (overall mean) and time-aware (per-hour-slot) detection.
//   - Minimum series length is 5 points; shorter series skip detection entirely.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT

package anomaly

import (
	"math"

	"github.com/rsharma155/sql_optima/internal/intel/utils"
)

type AnomalyDetector struct {
	StdThreshold float64
}

func NewAnomalyDetector(stdThreshold float64) *AnomalyDetector {
	if stdThreshold == 0 {
		stdThreshold = 2.5
	}
	return &AnomalyDetector{StdThreshold: stdThreshold}
}

// DetectPointAnomaly returns true when value is statistically anomalous vs the background series.
// DEFECT-8: when the series is constant-zero and the candidate is also near-zero (< 1.0),
// we return false — floating-point noise on a zero-baseline is not a real anomaly.
func (d *AnomalyDetector) DetectPointAnomaly(value float64, series []float64) (bool, float64) {
	if len(series) < 5 {
		return false, 0.0
	}
	mean := utils.Mean(series)
	std := utils.StdDev(series)

	if std < 1e-9 {
		// Constant series: only flag when the deviation is business-meaningful.
		// For near-zero baselines (memory_grants_pending=0, deadlocks=0, etc.)
		// we require the deviation to exceed 1.0 to avoid floating-point false positives.
		absDev := math.Abs(value - mean)
		if mean < 1.0 {
			// Zero/near-zero baseline: require meaningful absolute threshold.
			return absDev >= 1.0, absDev
		}
		// Non-zero constant baseline: 1% relative deviation is meaningful.
		return absDev > mean*0.01, absDev
	}

	zScore := math.Abs(value-mean) / std
	return zScore > d.StdThreshold, zScore
}

func (d *AnomalyDetector) DetectTrendAnomaly(series []float64) bool {
	if len(series) < 10 {
		return false
	}
	half := len(series) / 2
	firstHalf := series[:half]
	secondHalf := series[half:]
	firstMean := utils.Mean(firstHalf)
	secondMean := utils.Mean(secondHalf)
	if math.Abs(firstMean) < 1e-9 {
		return math.Abs(secondMean) > 1e-6
	}
	changePct := math.Abs((secondMean-firstMean)/firstMean) * 100
	return changePct > 50
}

func (d *AnomalyDetector) GetAnomalyScore(value float64, series []float64) float64 {
	isAnomaly, score := d.DetectPointAnomaly(value, series)
	if isAnomaly {
		return score
	}
	return 0.0
}

func (d *AnomalyDetector) FindAnomalousSegments(series []float64, window int) []struct {
	Start, End int
	Score      float64
} {
	var segments []struct {
		Start, End int
		Score      float64
	}
	if len(series) < window*2 {
		return segments
	}
	for i := 0; i < len(series); i += window {
		end := i + window
		if end > len(series) {
			end = len(series)
		}
		segment := series[i:end]
		background := append(series[:i], series[end:]...) //nolint:gocritic
		isAnomaly, score := d.DetectPointAnomaly(segment[len(segment)-1], background)
		if isAnomaly {
			segments = append(segments, struct {
				Start, End int
				Score      float64
			}{i, end, score})
		}
	}
	return segments
}
