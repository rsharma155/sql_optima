// Package intel provides the SQL Optima autonomous health intelligence engine.
// This file implements per-database and instance-level disk capacity forecasting.
//
// Design context:
//   - DEFECT-12 fix: uses true 24h aggregated deltas from sqlserver_disk_history,
//     not a single collection cycle delta (which would be off by 96×).
//   - DEFECT-13 fix: uses 7-day and 30-day linear regression for short/medium forecasts.
//   - GrowthPattern is classified by the coefficient of variation (CV) of daily growth rates.
//   - Breach date uses a 10% headroom floor (at minimum 10 GB) to avoid pushing too close to full.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT

package forecasting

import (
	"fmt"
	"math"
	"time"

	"github.com/rsharma155/sql_optima/internal/intel/utils"
	"github.com/rsharma155/sql_optima/internal/models"
)

// DBDailyGrowth holds per-database daily delta aggregates (one row per calendar day).
type DBDailyGrowth struct {
	DatabaseName    string
	GrowthDay       time.Time
	DailyDataGrowth float64 // MB, SUM(delta_data_mb) for that calendar day
	DailyLogGrowth  float64 // MB
	EODDataMB       float64 // MAX(data_mb) at end of day
	EODLogMB        float64 // MAX(log_mb) at end of day
}

// InstanceCapacityPoint holds a single point in the instance disk capacity timeline.
type InstanceCapacityPoint struct {
	CaptureTimestamp time.Time
	DataDiskMB       float64
	LogDiskMB        float64
	FreeDiskMB       float64
	UsedPct          float64
}

// ForecastCapacity computes the full CapacityForecastV2 from raw data.
// dbGrowthRows: per-database daily deltas (query 7.6 result).
// capacityTimeline: instance-level disk capacity over 30 days (query 7.7 result).
func ForecastCapacity(dbGrowthRows []DBDailyGrowth, capacityTimeline []InstanceCapacityPoint) *models.CapacityForecastV2 {
	if len(capacityTimeline) == 0 {
		return nil
	}

	// Latest snapshot for current values.
	latest := capacityTimeline[len(capacityTimeline)-1]
	totalDiskMB := latest.DataDiskMB + latest.LogDiskMB + latest.FreeDiskMB
	usedDiskMB := latest.DataDiskMB + latest.LogDiskMB
	if totalDiskMB <= 0 {
		return nil
	}

	// Compute true 24h daily growth from timeline (DEFECT-12 fix).
	dailyGrowthMBAvg, growthPattern, rSquared := analyzeGrowthPattern(capacityTimeline)

	// Short-term projection (30-day via 7-day average).
	projected30 := usedDiskMB + dailyGrowthMBAvg*30
	projected60 := usedDiskMB + dailyGrowthMBAvg*60
	projected90 := usedDiskMB + dailyGrowthMBAvg*90

	// Breach date with 10% headroom floor.
	headroomMB := math.Max(totalDiskMB*0.10, 10*1024) // at least 10 GB
	effectiveCapacity := totalDiskMB - headroomMB
	daysUntilBreach := -1
	breachDateApprox := "No breach projected in 90 days"
	if dailyGrowthMBAvg > 0 {
		days := (effectiveCapacity - usedDiskMB) / dailyGrowthMBAvg
		if days > 0 && days < 3650 {
			daysUntilBreach = int(days)
			breachDate := time.Now().AddDate(0, 0, daysUntilBreach)
			breachDateApprox = breachDate.Format("Jan 2006")
		}
	}

	reliabilityTier := ReliabilityTierFromR2(rSquared)

	// Per-database growth.
	dbEntries := buildDatabaseGrowthEntries(dbGrowthRows)

	return &models.CapacityForecastV2{
		TotalDiskMB:      totalDiskMB,
		UsedDiskMB:       usedDiskMB,
		FreeDiskMB:       latest.FreeDiskMB,
		UsedPct:          latest.UsedPct,
		DailyGrowthMBAvg: roundMB(dailyGrowthMBAvg),
		GrowthPattern:    growthPattern,
		ReliabilityTier:  reliabilityTier,
		Projected30dMB:   roundMB(projected30),
		Projected60dMB:   roundMB(projected60),
		Projected90dMB:   roundMB(projected90),
		DaysUntilBreach:  daysUntilBreach,
		BreachDateApprox: breachDateApprox,
		RSquared:         math.Round(rSquared*100) / 100,
		Databases:        dbEntries,
	}
}

// analyzeGrowthPattern computes the true daily average growth, classifies the growth
// pattern, and returns R² from a linear regression on the capacity timeline.
func analyzeGrowthPattern(timeline []InstanceCapacityPoint) (dailyAvgMB float64, pattern string, rSquared float64) {
	if len(timeline) < 2 {
		return 0, "Unknown", 0
	}

	// Build daily growth rates from adjacent timeline points.
	var dailyRates []float64
	for i := 1; i < len(timeline); i++ {
		prev := timeline[i-1]
		cur := timeline[i]
		elapsed := cur.CaptureTimestamp.Sub(prev.CaptureTimestamp).Hours()
		if elapsed <= 0 {
			continue
		}
		growthMB := (cur.DataDiskMB + cur.LogDiskMB) - (prev.DataDiskMB + prev.LogDiskMB)
		// Normalise to daily rate (hours × 24).
		dailyRate := growthMB * 24 / elapsed
		if dailyRate >= 0 { // discard shrinkage points (auto-shrink, backups)
			dailyRates = append(dailyRates, dailyRate)
		}
	}

	if len(dailyRates) == 0 {
		return 0, "Unknown", 0
	}

	dailyAvgMB = utils.Mean(dailyRates)

	// Linear regression on (index, usedMB) to compute R².
	x := make([]float64, len(timeline))
	y := make([]float64, len(timeline))
	for i, pt := range timeline {
		x[i] = float64(i)
		y[i] = pt.DataDiskMB + pt.LogDiskMB
	}
	lr := utils.FitLinearRegression(x, y)
	rSquared = lr.RSquared

	// Growth pattern via coefficient of variation (CV).
	std := utils.StdDev(dailyRates)
	if dailyAvgMB > 0 {
		cv := std / dailyAvgMB
		switch {
		case cv < 0.15:
			pattern = "Steady"
		case cv < 0.50:
			pattern = "Variable"
		default:
			pattern = "Spikey"
		}
	} else {
		pattern = "Steady"
	}

	return dailyAvgMB, pattern, rSquared
}

// buildDatabaseGrowthEntries aggregates per-database daily deltas into growth entries.
func buildDatabaseGrowthEntries(rows []DBDailyGrowth) []models.DatabaseGrowthEntry {
	if len(rows) == 0 {
		return nil
	}

	// Group rows by database name.
	type dbData struct {
		latestDataMB float64
		latestLogMB  float64
		growthRates  []float64
	}
	byDB := make(map[string]*dbData)
	for _, r := range rows {
		if byDB[r.DatabaseName] == nil {
			byDB[r.DatabaseName] = &dbData{}
		}
		d := byDB[r.DatabaseName]
		if r.EODDataMB > d.latestDataMB {
			d.latestDataMB = r.EODDataMB
			d.latestLogMB = r.EODLogMB
		}
		if r.DailyDataGrowth >= 0 {
			d.growthRates = append(d.growthRates, r.DailyDataGrowth)
		}
	}

	var entries []models.DatabaseGrowthEntry
	for dbName, d := range byDB {
		if len(d.growthRates) == 0 {
			continue
		}
		// Use last 7 days of available growth for short-term rate.
		window := d.growthRates
		if len(window) > 7 {
			window = window[len(window)-7:]
		}
		daily7Avg := utils.Mean(window)

		// Growth pattern via CV.
		std := utils.StdDev(d.growthRates)
		pattern := "Steady"
		if daily7Avg > 0 {
			cv := std / daily7Avg
			switch {
			case cv >= 0.50:
				pattern = "Spikey"
			case cv >= 0.15:
				pattern = "Variable"
			}
		}

		// Reliability: need at least 7 data points for linear regression.
		reliabilityTier := "Unreliable"
		if len(d.growthRates) >= 7 {
			x := make([]float64, len(d.growthRates))
			y := make([]float64, len(d.growthRates))
			for i, v := range d.growthRates {
				x[i] = float64(i)
				y[i] = v
			}
			lr := utils.FitLinearRegression(x, y)
			reliabilityTier = ReliabilityTierFromR2(lr.RSquared)
		}

		proj30 := d.latestDataMB + daily7Avg*30
		proj90 := d.latestDataMB + daily7Avg*90

		entries = append(entries, models.DatabaseGrowthEntry{
			DatabaseName:      dbName,
			CurrentDataMB:     roundMB(d.latestDataMB),
			CurrentLogMB:      roundMB(d.latestLogMB),
			Daily7AvgGrowthMB: roundMB(daily7Avg),
			Projected30dMB:    roundMB(proj30),
			Projected90dMB:    roundMB(proj90),
			GrowthPattern:     pattern,
			ReliabilityTier:   reliabilityTier,
		})
	}
	return entries
}

func roundMB(v float64) float64 {
	return math.Round(v*10) / 10
}

// BreachSummary returns a human-readable summary of the capacity breach estimate.
func BreachSummary(fc *models.CapacityForecastV2) string {
	if fc == nil {
		return "No capacity data available."
	}
	if fc.DaysUntilBreach <= 0 {
		return fmt.Sprintf("Disk is %.0f%% used (%.0f GB / %.0f GB). No capacity breach projected in 90 days at current growth rate of %.1f GB/day.",
			fc.UsedPct, fc.UsedDiskMB/1024, fc.TotalDiskMB/1024, fc.DailyGrowthMBAvg/1024)
	}
	return fmt.Sprintf("Disk is %.0f%% used. Estimated capacity breach: %s (~%d days). Growth: %.1f GB/day (%s pattern, %s forecast).",
		fc.UsedPct, fc.BreachDateApprox, fc.DaysUntilBreach, fc.DailyGrowthMBAvg/1024, fc.GrowthPattern, fc.ReliabilityTier)
}
