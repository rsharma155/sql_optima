// Package intel provides the SQL Optima autonomous health intelligence engine.
// This file implements linear regression and exponential smoothing forecasting.
//
// Design context:
//   - DEFECT-9 fix: inferFailureThreshold is used only as a fallback; callers should
//     pass a thresholdMap derived from DynamicThresholds for hardware-accurate thresholds.
//   - DEFECT-10 fix: every ForecastSeries now carries a ReliabilityTier computed from R².
//     Callers must check this tier before displaying numerical date estimates.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT

package forecasting

import (
	"math"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/intel/utils"
)

// ForecastLinear performs OLS linear regression on the value series and projects
// horizonDays into the future. The returned ForecastSeries includes a ReliabilityTier
// derived from R² (DEFECT-10). An optional thresholdMap can override the name-based
// failure threshold inference (DEFECT-9).
func ForecastLinear(values []float64, metricName string, horizonDays int, confidenceLevel float64, thresholdMap ...map[string]float64) ForecastSeries {
	if len(values) < 3 {
		return ForecastSeries{
			MetricName:      metricName,
			HorizonDays:     horizonDays,
			Confidence:      0.0,
			ReliabilityTier: "Unreliable",
		}
	}

	n := len(values)
	x := make([]float64, n)
	y := make([]float64, n)
	for i := 0; i < n; i++ {
		x[i] = float64(i)
		y[i] = values[i]
	}

	lr := utils.FitLinearRegression(x, y)
	slope := lr.Slope
	intercept := lr.Intercept

	residuals := make([]float64, n)
	for i := 0; i < n; i++ {
		pred := slope*float64(i) + intercept
		residuals[i] = y[i] - pred
	}
	stdError := utils.StdError(residuals)

	r2 := lr.RSquared

	// Resolve failure threshold: prefer explicit thresholdMap, fall back to name-based inference.
	var failureThreshold *float64
	if len(thresholdMap) > 0 && thresholdMap[0] != nil {
		if v, ok := thresholdMap[0][metricName]; ok {
			failureThreshold = &v
		}
	}
	if failureThreshold == nil {
		failureThreshold = inferFailureThreshold(metricName)
	}

	now := time.Now().UTC()
	var points []ForecastPoint
	var predictedFailureDays *int

	for i := 1; i <= horizonDays; i++ {
		futureX := float64(n + i - 1)
		predVal := slope*futureX + intercept

		ts := now.AddDate(0, 0, i)
		z := 1.96
		bound := z * stdError * math.Sqrt(1+float64(i)/float64(n))

		upper := predVal + bound
		lower := predVal - bound
		if lower < 0 {
			lower = 0
		}
		if upper < 0 {
			upper = 0
		}
		if predVal < 0 {
			predVal = 0
		}

		points = append(points, ForecastPoint{
			Timestamp:  ts,
			Value:      predVal,
			UpperBound: &upper,
			LowerBound: &lower,
		})

		if failureThreshold != nil && predVal >= *failureThreshold && predictedFailureDays == nil {
			days := i
			predictedFailureDays = &days
		}
	}

	meanVal := utils.Mean(values)
	var growthRate float64
	if meanVal != 0 {
		growthRate = slope / meanVal * 100
	}

	trend := "stable"
	stdV := utils.StdDev(values)
	if stdV > 0 {
		if slope > 0.01*stdV {
			trend = "increasing"
		} else if slope < -0.01*stdV {
			trend = "decreasing"
		}
	} else if slope > 0 {
		trend = "increasing"
	} else if slope < 0 {
		trend = "decreasing"
	}

	return ForecastSeries{
		MetricName:           metricName,
		HorizonDays:          horizonDays,
		Points:               points,
		Confidence:           math.Max(0, math.Min(1, r2)),
		PredictedFailureDays: predictedFailureDays,
		TrendDirection:       trend,
		GrowthRatePct:        growthRate,
		ReliabilityTier:      ReliabilityTierFromR2(r2),
	}
}

// ForecastExponentialSmoothing applies single exponential smoothing with alpha decay.
func ForecastExponentialSmoothing(values []float64, metricName string, horizonDays int, alpha float64) ForecastSeries {
	if len(values) < 2 {
		return ForecastSeries{
			MetricName:      metricName,
			HorizonDays:     horizonDays,
			Confidence:      0.0,
			ReliabilityTier: "Unreliable",
		}
	}

	smoothed := make([]float64, len(values))
	smoothed[0] = values[0]
	for t := 1; t < len(values); t++ {
		smoothed[t] = alpha*values[t] + (1-alpha)*smoothed[t-1]
	}

	var recentTrend float64
	if len(smoothed) >= 5 {
		diffs := make([]float64, 4)
		for i := 0; i < 4; i++ {
			idx := len(smoothed) - 5 + i
			diffs[i] = smoothed[idx+1] - smoothed[idx]
		}
		recentTrend = utils.Mean(diffs)
	}
	lastValue := smoothed[len(smoothed)-1]

	now := time.Now().UTC()
	var points []ForecastPoint
	current := lastValue

	variance := utils.StdDev(values)
	variance = variance * variance
	stdError := math.Sqrt(variance) * 0.1

	failureThreshold := inferFailureThreshold(metricName)
	var predictedFailureDays *int

	for i := 1; i <= horizonDays; i++ {
		current += recentTrend
		if current < 0 {
			current = 0
		}
		val := current
		ts := now.AddDate(0, 0, i)
		bound := stdError * 2

		upper := val + bound
		lower := val - bound
		if lower < 0 {
			lower = 0
		}

		points = append(points, ForecastPoint{
			Timestamp:  ts,
			Value:      val,
			UpperBound: &upper,
			LowerBound: &lower,
		})

		if failureThreshold != nil && val >= *failureThreshold && predictedFailureDays == nil {
			days := i
			predictedFailureDays = &days
		}
	}

	trend := "stable"
	if recentTrend > 0 {
		trend = "increasing"
	} else if recentTrend < 0 {
		trend = "decreasing"
	}

	var growthRate float64
	if lastValue != 0 {
		growthRate = recentTrend / lastValue * 100
	}

	return ForecastSeries{
		MetricName:           metricName,
		HorizonDays:          horizonDays,
		Points:               points,
		Confidence:           0.6,
		PredictedFailureDays: predictedFailureDays,
		TrendDirection:       trend,
		GrowthRatePct:        growthRate,
		ReliabilityTier:      "Indicative", // exponential smoothing is always indicative
	}
}

// inferFailureThreshold derives a default threshold from the metric name.
// Prefer passing an explicit thresholdMap (DEFECT-9 fix) over this function.
func inferFailureThreshold(metricName string) *float64 {
	lower := strings.ToLower(metricName)
	if strings.Contains(lower, "disk") || strings.Contains(lower, "space") || strings.Contains(lower, "storage") {
		v := 95.0
		return &v
	}
	if strings.Contains(lower, "cpu") {
		v := 95.0
		return &v
	}
	if strings.Contains(lower, "memory") || strings.Contains(lower, "ple") {
		v := 90.0
		return &v
	}
	if strings.Contains(lower, "tempdb") {
		v := 90.0
		return &v
	}
	if strings.Contains(lower, "log") {
		v := 95.0
		return &v
	}
	return nil
}
