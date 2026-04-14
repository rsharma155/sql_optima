// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: JSON/API models and filter structs for the storage index health dashboard.
// Engine-specific Timescale query implementations live in pg_ts_logger_storage_index_health.go
// and mssql_ts_logger_storage_index_health.go.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import "time"

type StorageIndexHealthKPI struct {
	UnusedIndexCount               int     `json:"unused_index_count"`
	HighScanTableCount             int     `json:"high_scan_table_count"`
	FastestGrowingTable            string  `json:"fastest_growing_table"`
	FastestGrowingTableGrowthPct7d float64 `json:"fastest_growing_table_growth_7d_pct"`
	IndexWriteOverheadPct          float64 `json:"index_write_overhead_pct"`
}

type StorageIndexHealthTopRow struct {
	DBName        string     `json:"db_name"`
	SchemaName    string     `json:"schema_name"`
	TableName     string     `json:"table_name"`
	IndexName     string     `json:"index_name,omitempty"`
	Value         float64    `json:"value"`
	Value2        float64    `json:"value2,omitempty"`
	LastUserSeek  *time.Time `json:"last_user_seek,omitempty"`
}

type StorageIndexHealthGrowthPoint struct {
	Bucket      time.Time `json:"bucket"`
	TableSizeMB float64   `json:"table_size_mb"`
	IndexSizeMB float64   `json:"index_size_mb"`
}

type StorageIndexHealthGrowthSummary struct {
	CurrentTableMB      float64 `json:"current_table_mb"`
	CurrentIndexMB      float64 `json:"current_index_mb"`
	CurrentRowCount     int64   `json:"current_row_count"`
	DailyGrowthMB       float64 `json:"daily_growth_mb"`
	Growth7dPct         float64 `json:"growth_7d_pct"`
	Growth30dPct        float64 `json:"growth_30d_pct"`
	ProjectedTableMB90d float64 `json:"projected_table_mb_90d"`
	ProjectedIndexMB90d float64 `json:"projected_index_mb_90d"`
}

// StorageIndexHealthSeekScanLookupRow is used for the SQL Server "seek vs scan vs lookup" stacked chart.
type StorageIndexHealthSeekScanLookupRow struct {
	DBName     string  `json:"db_name"`
	SchemaName string  `json:"schema_name"`
	TableName  string  `json:"table_name"`
	Seeks      float64 `json:"seeks"`
	Scans      float64 `json:"scans"`
	Lookups    float64 `json:"lookups"`
}

type StorageIndexHealthDashboard struct {
	Engine                   string                                `json:"engine"`
	Instance                 string                                `json:"instance"`
	From                     string                                `json:"from"`
	To                       string                                `json:"to"`
	KPIs                     StorageIndexHealthKPI                 `json:"kpis"`
	TopScans                 []StorageIndexHealthTopRow            `json:"top_scans"`
	SeekScanLookup           []StorageIndexHealthSeekScanLookupRow `json:"seek_scan_lookup,omitempty"`
	LargestTables            []StorageIndexHealthTopRow            `json:"largest_tables"`
	LargestIndexes           []StorageIndexHealthTopRow            `json:"largest_indexes"`
	Growth                   []StorageIndexHealthGrowthPoint       `json:"growth"`
	GrowthSummary            StorageIndexHealthGrowthSummary       `json:"growth_summary"`
	UnusedIndexes            []StorageIndexHealthTopRow            `json:"unused_indexes"`
	HighScanTables           []StorageIndexHealthTopRow            `json:"high_scan_tables"`
	DuplicateIndexCandidates []map[string]interface{}              `json:"duplicate_index_candidates"`
}

type SIHFilters struct {
	DBNames     []string
	SchemaNames []string
	// TableLike is a case-insensitive substring filter for table_name.
	TableLike string
}

type SIHFilterOptions struct {
	Databases []string `json:"databases"`
	Schemas   []string `json:"schemas"`
	Tables    []string `json:"tables"`
	// SourceRowCounts is best-effort counts in the filter time window (for UI empty-state / collector diagnostics).
	SourceRowCounts SIHFilterSourceRowCounts `json:"source_row_counts"`
}

// SIHFilterSourceRowCounts reports how many raw rows exist per SIH hypertable for the instance in the time window.
type SIHFilterSourceRowCounts struct {
	TableSizeHistory int64 `json:"table_size_history"`
	TableUsageStats  int64 `json:"table_usage_stats"`
	IndexUsageStats  int64 `json:"index_usage_stats"`
}
