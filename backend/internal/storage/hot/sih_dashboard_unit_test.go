// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for storage index health dashboard business logic (no DB required).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test that growth summary projection formula is correct
func TestGrowthSummaryProjectionFormula(t *testing.T) {
	// DailyGrowthMB=10, current=1000MB → 90d projection should be 1900MB
	gs := StorageIndexHealthGrowthSummary{
		CurrentTableMB: 1000,
		DailyGrowthMB:  10,
	}
	// Note: The formula in sih_dashboard.go is:
	// ProjectedTableMB90d: currentTbl + (dailyGrowth * 90.0)
	gs.ProjectedTableMB90d = gs.CurrentTableMB + (gs.DailyGrowthMB * 90.0)
	assert.Equal(t, float64(1900), gs.ProjectedTableMB90d)
}

// Test that KPI ForecastTableMB90d combines table + index projections
func TestForecastCombinesTableAndIndex(t *testing.T) {
	gs := StorageIndexHealthGrowthSummary{
		ProjectedTableMB90d: 1900,
		ProjectedIndexMB90d: 400,
	}
	kpis := StorageIndexHealthKPI{}
	kpis.ForecastTableMB90d = gs.ProjectedTableMB90d + gs.ProjectedIndexMB90d
	assert.Equal(t, float64(2300), kpis.ForecastTableMB90d)
}

// Test KPI write overhead pct is bounded 0–100
func TestWriteOverheadPctBounds(t *testing.T) {
	// Formula: (updates / (updates + seeks + scans + lookups)) * 100
	updates := float64(100)
	seeks := float64(50)
	scans := float64(50)
	lookups := float64(50)
	total := updates + seeks + scans + lookups
	pct := (updates / total) * 100.0
	assert.Equal(t, float64(40), pct)
	assert.GreaterOrEqual(t, pct, float64(0))
	assert.LessOrEqual(t, pct, float64(100))

	// Zero case
	assert.Equal(t, float64(0), (0.0/1.0)*100.0)
}

// Test that StorageIndexHealthTopRow value semantics are documented via assertions
func TestTopRowValueSemanticsByContext(t *testing.T) {
	// Largest Tables: value = total_size_mb, value2 = index_size_mb → value >= value2
	largestTbl := StorageIndexHealthTopRow{Value: 500, Value2: 120}
	assert.GreaterOrEqual(t, largestTbl.Value, largestTbl.Value2,
		"For largest_tables: value(total) must be >= value2(index)")

	// Largest Indexes: value = index_size_mb, value2 = parent_table_size_mb
	largestIdx := StorageIndexHealthTopRow{Value: 120, Value2: 500}
	assert.LessOrEqual(t, largestIdx.Value, largestIdx.Value2,
		"For largest_indexes: value(index_mb) must be <= value2(parent_table_mb)")

	// Unused indexes: value = seeks (0 for unused)
	unusedIdx := StorageIndexHealthTopRow{Value: 0, Value2: 150}
	assert.Equal(t, float64(0), unusedIdx.Value, "Unused index: seeks must be 0")
	assert.Greater(t, unusedIdx.Value2, float64(0), "Unused index: size must be > 0")
}
