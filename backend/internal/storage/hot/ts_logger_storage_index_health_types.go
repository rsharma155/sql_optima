// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: JSON/API models and filter structs for the storage index health dashboard.
// Engine-specific Timescale query implementations live in pg_ts_logger_storage_index_health.go
// and sqlserver_ts_logger_storage_index_health.go.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import "time"

type StorageIndexHealthKPI struct {
	UnusedIndexCount               int     `json:"unused_index_count"`
	UnusedIndexMB                  float64 `json:"unused_index_mb"`
	HighScanTableCount             int     `json:"high_scan_table_count"`
	FastestGrowingTable            string  `json:"fastest_growing_table"`
	FastestGrowingTableGrowthPct7d float64 `json:"fastest_growing_table_growth_7d_pct"`
	IndexWriteOverheadPct          float64 `json:"index_write_overhead_pct"`
	TotalDBSizeMB                  float64 `json:"total_db_size_mb"`
	ForecastTableMB90d             float64 `json:"forecast_table_mb_90d"`
	Growth7dPct                    float64 `json:"growth_7d_pct"`
}

// StorageIndexHealthTopRow is a multipurpose row used across several dashboard sections.
// Field semantics vary by usage context:
//
//   largest_tables:  value  = total_size_mb (data + index), value2 = index_size_mb
//   largest_indexes: value  = index_size_mb,                value2 = parent_table_size_mb
//   unused_indexes:  value  = SUM(seeks) in window,         value2 = index_size_mb
//   high_scan_tables:value  = SUM(scans),                   value2 = SUM(seeks)
//   top_scans:       value  = SUM(scans),                   value2 = SUM(seeks)
//   top_growth_tables:value = growth_mb_7d,                 value2 = base_table_size_mb
//
// IndexName is only populated for index-level rows; TableName is always populated.
// LastUserSeek is populated for unused_indexes rows only.
type StorageIndexHealthTopRow struct {
	DBName       string     `json:"db_name"`
	SchemaName   string     `json:"schema_name"`
	TableName    string     `json:"table_name"`
	IndexName    string     `json:"index_name,omitempty"`
	Value        float64    `json:"value"`
	Value2       float64    `json:"value2,omitempty"`
	LastUserSeek *time.Time `json:"last_user_seek,omitempty"`
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
	Engine              string                                `json:"engine"`
	Instance            string                                `json:"serverID"`
	From                string                                `json:"from"`
	To                  string                                `json:"to"`
	CollectorError      string                                `json:"collector_error,omitempty"`
	KPIs                StorageIndexHealthKPI                 `json:"kpis"`
	TopScans            []StorageIndexHealthTopRow            `json:"top_scans"`
	SeekScanLookup      []StorageIndexHealthSeekScanLookupRow `json:"seek_scan_lookup,omitempty"`
	LargestTables       []StorageIndexHealthTopRow            `json:"largest_tables"`
	LargestIndexes      []StorageIndexHealthTopRow            `json:"largest_indexes"`
	TopGrowthTables     []StorageIndexHealthTopRow            `json:"top_growth_tables"`
	Growth              []StorageIndexHealthGrowthPoint       `json:"growth"`
	GrowthSummary       StorageIndexHealthGrowthSummary       `json:"growth_summary"`
	UnusedIndexes       []StorageIndexHealthTopRow            `json:"unused_indexes"`
	HighScanTables      []StorageIndexHealthTopRow            `json:"high_scan_tables"`
	Insights            []map[string]interface{}              `json:"insights"`
	RawDuplicateIndexes []map[string]interface{}              `json:"raw_duplicate_indexes,omitempty"`
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

// SIHFilterSourceRowCounts reports how many raw rows exist per SIH hypertable for the serverID in the time window.
type SIHFilterSourceRowCounts struct {
	TableSizeHistory int64 `json:"table_size_history"`
	TableUsageStats  int64 `json:"table_usage_stats"`
	IndexUsageStats  int64 `json:"index_usage_stats"`
}
