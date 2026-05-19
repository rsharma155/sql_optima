// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package forecasting




import "time"

type ForecastPoint struct {
	Timestamp  time.Time `json:"timestamp"`
	Value      float64   `json:"value"`
	UpperBound *float64  `json:"upper_bound,omitempty"`
	LowerBound *float64  `json:"lower_bound,omitempty"`
}

type ForecastSeries struct {
	MetricName           string          `json:"metric_name"`
	HorizonDays          int             `json:"horizon_days"`
	Points               []ForecastPoint `json:"points"`
	Confidence           float64         `json:"confidence"`
	PredictedFailureDays *int            `json:"predicted_failure_days,omitempty"`
	TrendDirection       string          `json:"trend_direction"`
	GrowthRatePct        float64         `json:"growth_rate_pct"`
}