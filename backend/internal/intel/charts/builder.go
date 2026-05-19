// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package charts


import (
	"encoding/json"

	"github.com/rsharma155/sql_optima/internal/models"
)

type ChartBuilder struct{}

func NewChartBuilder() *ChartBuilder {
	return &ChartBuilder{}
}

type ChartData struct {
	DivID  string      `json:"div_id"`
	Title  string      `json:"title"`
	Series []Series    `json:"series"`
	X      interface{} `json:"x,omitempty"`
	XLabel string      `json:"xlabel,omitempty"`
	YLabel string      `json:"ylabel,omitempty"`
	Type   string      `json:"chart_type"`
	Labels []string    `json:"labels,omitempty"`
	Values []float64   `json:"values,omitempty"`
	Colors []string    `json:"colors,omitempty"`
	Items  []Item      `json:"items,omitempty"`
}

type Series struct {
	Name string        `json:"name"`
	Data []float64     `json:"data"`
	Type string        `json:"type"`
}

type Item struct {
	Metric     string  `json:"metric"`
	Days       int     `json:"days"`
	Confidence float64 `json:"confidence"`
}

func (b *ChartBuilder) CPUTrendChart(series []float64, thresholds map[string]float64) string {
	threshold := 0.0
	if t, ok := thresholds["cpu_sustained_threshold"]; ok {
		threshold = t
	}
	seriesData := []Series{{Name: "CPU %", Data: series, Type: "line"}}
	if threshold > 0 {
		tData := make([]float64, len(series))
		for i := range tData {
			tData[i] = threshold
		}
		seriesData = append([]Series{{Name: "Dynamic Threshold", Data: tData, Type: "threshold"}}, seriesData...)
	}
	return toJSON(ChartData{DivID: "cpu-trend", Title: "CPU Utilization Trend", Series: seriesData, X: indexRange(len(series)), XLabel: "Time", YLabel: "CPU %", Type: "line"})
}

func (b *ChartBuilder) RiskBreakdownChart(risk models.RiskScoreResult) string {
	return toJSON(ChartData{
		DivID:  "risk-breakdown",
		Title:  "Risk Score Breakdown",
		Type:   "radar",
		Labels: []string{"Performance", "Capacity", "Availability", "Replication", "Maintenance", "Query"},
		Values: []float64{risk.PerformanceRisk, risk.CapacityRisk, risk.AvailabilityRisk, risk.ReplicationRisk, risk.MaintenanceRisk, risk.QueryRisk},
		Colors: []string{"#ff6384", "#36a2eb", "#ffce56", "#4bc0c0", "#9966ff", "#ff9f40"},
	})
}

func (b *ChartBuilder) WorkloadTrendChart(tpsSeries, batchSeries []float64) string {
	var series []Series
	if len(tpsSeries) > 0 {
		series = append(series, Series{Name: "TPS", Data: tpsSeries, Type: "line"})
	}
	if len(batchSeries) > 0 {
		series = append(series, Series{Name: "Batch Req/s", Data: batchSeries, Type: "line"})
	}
	if len(series) == 0 {
		return ""
	}
	maxLen := 0
	for _, s := range series {
		if len(s.Data) > maxLen {
			maxLen = len(s.Data)
		}
	}
	for i := range series {
		if len(series[i].Data) < maxLen {
			padded := make([]float64, maxLen)
			copy(padded, series[i].Data)
			for j := len(series[i].Data); j < maxLen; j++ {
				padded[j] = series[i].Data[len(series[i].Data)-1]
			}
			series[i].Data = padded
		}
	}
	return toJSON(ChartData{DivID: "workload-trend", Title: "Workload Trend", Series: series, X: indexRange(maxLen), Type: "multi_line"})
}

func (b *ChartBuilder) FailureCountdownChart(forecasts []models.ForecastResult) string {
	var items []Item
	for _, f := range forecasts {
		if f.PredictedFailureDays != nil {
			items = append(items, Item{Metric: f.MetricName, Days: *f.PredictedFailureDays, Confidence: f.Confidence})
		}
	}
	if len(items) == 0 {
		return ""
	}
	return toJSON(ChartData{DivID: "failure-gauge", Title: "Days to Predicted Failure", Items: items, Type: "gauge"})
}

func toJSON(d ChartData) string {
	b, _ := json.Marshal(d)
	return string(b)
}

func indexRange(n int) []int {
	r := make([]int, n)
	for i := 0; i < n; i++ {
		r[i] = i
	}
	return r
}
