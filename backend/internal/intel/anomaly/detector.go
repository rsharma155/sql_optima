// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

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

func (d *AnomalyDetector) DetectPointAnomaly(value float64, series []float64) (bool, float64) {
	if len(series) < 5 {
		return false, 0.0
	}
	mean := utils.Mean(series)
	std := utils.StdDev(series)
	if std < 1e-9 {
		return math.Abs(value-mean) > 1e-6, math.Abs(value - mean)
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

func (d *AnomalyDetector) FindAnomalousSegments(series []float64, window int) []struct{ Start, End int; Score float64 } {
	var segments []struct{ Start, End int; Score float64 }
	if len(series) < window*2 {
		return segments
	}
	for i := 0; i < len(series); i += window {
		end := i + window
		if end > len(series) {
			end = len(series)
		}
		segment := series[i:end]
		background := append(series[:i], series[end:]...)
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