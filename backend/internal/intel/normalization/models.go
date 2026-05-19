// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package normalization




import "time"

type HealthMetric struct {
	CurrentValue    float64     `json:"current_value"`
	HistoricalSeries []float64  `json:"historical_series"`
	Timestamps      []time.Time `json:"timestamps,omitempty"`
	MovingAvg       []float64   `json:"moving_avg"`
	MovingStd       []float64   `json:"moving_std"`
	TrendSlope      float64     `json:"trend_slope"`
	AnomalyScore    float64     `json:"anomaly_score"`
	MinValue        float64     `json:"min_value"`
	MaxValue        float64     `json:"max_value"`
	AvgValue        float64     `json:"avg_value"`
}

type HostHealth struct {
	HostID  string        `json:"host_id,omitempty"`
	CPU     *HealthMetric  `json:"cpu,omitempty"`
	Memory  *HealthMetric  `json:"memory,omitempty"`
	Disk    *HealthMetric  `json:"disk,omitempty"`
	IO      *HealthMetric  `json:"io,omitempty"`
	Network *HealthMetric  `json:"network,omitempty"`
}

type DatabaseHealth struct {
	DatabaseID  string       `json:"database_id,omitempty"`
	Transactions *HealthMetric `json:"transactions,omitempty"`
	Connections *HealthMetric  `json:"connections,omitempty"`
	TempDB      *HealthMetric  `json:"tempdb,omitempty"`
	LogGrowth   *HealthMetric  `json:"log_growth,omitempty"`
	IndexHealth *HealthMetric  `json:"index_health,omitempty"`
}

type StorageHealth struct {
	DiskID           string        `json:"disk_id,omitempty"`
	SpaceUsedPct     *HealthMetric `json:"space_used_pct,omitempty"`
	SpaceFreePct     *HealthMetric `json:"space_free_pct,omitempty"`
	GrowthRate       *HealthMetric `json:"growth_rate,omitempty"`
	ProjectedFillDays *float64     `json:"projected_fill_days,omitempty"`
}

type QueryHealth struct {
	QueryID          string        `json:"query_id,omitempty"`
	Duration         *HealthMetric `json:"duration,omitempty"`
	CPUTime          *HealthMetric `json:"cpu_time,omitempty"`
	LogicalReads     *HealthMetric `json:"logical_reads,omitempty"`
	BlockingDuration *HealthMetric `json:"blocking_duration,omitempty"`
}

type ReplicationHealth struct {
	Publisher  string        `json:"publisher,omitempty"`
	LagSeconds *HealthMetric `json:"lag_seconds,omitempty"`
	Latency    *HealthMetric `json:"latency,omitempty"`
	SyncStatus *HealthMetric `json:"sync_status,omitempty"`
}

type CapacityHealth struct {
	StorageGrowthDailyPct  float64  `json:"storage_growth_daily_pct"`
	DatabaseGrowthDailyPct float64  `json:"database_growth_daily_pct"`
	LogGrowthDailyPct      float64  `json:"log_growth_daily_pct"`
	DaysUntilDiskFull      *float64 `json:"days_until_disk_full,omitempty"`
	DaysUntilLogFull       *float64 `json:"days_until_log_full,omitempty"`
}

type OperationalHealth struct {
	BackupSuccessRate         float64  `json:"backup_success_rate"`
	LastSuccessfulBackupHours *float64 `json:"last_successful_backup_hours,omitempty"`
	IndexMaintenanceAgeDays   *float64 `json:"index_maintenance_age_days,omitempty"`
	StatsUpdateAgeDays        *float64 `json:"stats_update_age_days,omitempty"`
}

type NormalizedSystem struct {
	Host         *HostHealth         `json:"host,omitempty"`
	Database     *DatabaseHealth     `json:"database,omitempty"`
	Storage      *StorageHealth      `json:"storage,omitempty"`
	Query        *QueryHealth        `json:"query,omitempty"`
	Replication  *ReplicationHealth  `json:"replication,omitempty"`
	Capacity     *CapacityHealth     `json:"capacity,omitempty"`
	Operational  *OperationalHealth  `json:"operational,omitempty"`
}