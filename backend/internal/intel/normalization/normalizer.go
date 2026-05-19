// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package normalization




import (
	"github.com/rsharma155/sql_optima/internal/intel/ontology"
	"github.com/rsharma155/sql_optima/internal/intel/utils"
)

func NormalizeSystem(ont *ontology.OntologyGraph, rawData map[string]interface{}) *NormalizedSystem {
	system := &NormalizedSystem{}

	host := &HostHealth{}
	host.CPU = buildMetricFromKeys(rawData, "cpu_utilization", []string{"avg_cpu_load", "sql_process", "cpu_user_pct"}, 45.0)
	host.Memory = buildMetricFromKeys(rawData, "memory_usage", []string{"memory_usage", "sql_memory_used_mb", "memory_used_pct"}, 72.0)
	host.Disk = buildMetricFromKeys(rawData, "disk_usage", []string{"data_disk_mb", "disk_used_pct", "free_disk_mb"}, 65.0)
	host.IO = buildMetricFromKeys(rawData, "io_latency", []string{"read_latency_ms", "write_latency_ms", "avg_read_latency_ms"}, 15.0)
	system.Host = host

	db := &DatabaseHealth{}
	db.TempDB = buildMetricFromKeys(rawData, "tempdb_usage", []string{"tempdb_used_percent", "temp_files"}, 30.0)
	system.Database = db

	storage := &StorageHealth{}
	freeMB := getValue(rawData, "free_disk_mb", "free_mb")
	dataMB := getValue(rawData, "data_disk_mb", "data_mb")
	if freeMB != nil && dataMB != nil && *dataMB > 0 {
		usedPct := ((*dataMB - *freeMB) / *dataMB) * 100
		storage.SpaceUsedPct = makeMetric([]float64{usedPct})
	} else {
		storage.SpaceUsedPct = buildMetricFromKeys(rawData, "space_used_pct", []string{"data_disk_mb"}, 70.0)
	}
	system.Storage = storage

	query := &QueryHealth{}
	query.Duration = buildMetricFromKeys(rawData, "query_duration", []string{"total_elapsed_time_ms", "avg_duration_ms"}, 100.0)
	query.CPUTime = buildMetricFromKeys(rawData, "query_cpu", []string{"cpu_time_ms", "avg_cpu_time_ms"}, 50.0)
	system.Query = query

	repl := &ReplicationHealth{}
	repl.LagSeconds = buildMetricFromKeys(rawData, "repl_lag", []string{"secondary_lag_seconds", "latency_seconds"}, 0.0)
	system.Replication = repl

	capacity := &CapacityHealth{}
	deltaData := getValue(rawData, "delta_data_mb", "delta_log_mb")
	if deltaData != nil {
		if dataMB != nil && *dataMB > 0 {
			capacity.StorageGrowthDailyPct = minf(infAbs(*deltaData)*100.0/maxf(*dataMB, 1), 100.0)
		}
	}
	if freeMB != nil && deltaData != nil && *deltaData > 0 {
		dd := *freeMB / *deltaData
		capacity.DaysUntilDiskFull = &dd
	}
	system.Capacity = capacity

	system.Operational = &OperationalHealth{}
	failedJobs := getValue(rawData, "failed_jobs_24h", "failed_jobs")
	if failedJobs != nil {
		system.Operational.BackupSuccessRate = maxf(0, 100-*failedJobs*5)
	}

	_ = ont
	return system
}

func BuildMetricMapFromRaw(rawData map[string]interface{}) map[string]float64 {
	m := make(map[string]float64)

	m["avg_cpu_load"] = getValueOr(rawData, 45.0, "avg_cpu_load")
	m["sql_process_cpu"] = getValueOr(rawData, 30.0, "sql_process")
	m["memory_usage_pct"] = getValueOr(rawData, 72.0, "memory_usage")
	m["ple_seconds"] = getValueOr(rawData, 500.0, "ple_seconds", "ple")
	m["memory_grants_pending"] = getValueOr(rawData, 0.0, "memory_grants_pending", "waiting_memory_grants")
	m["buffer_cache_hit_ratio"] = getValueOr(rawData, 98.0, "buffer_cache_hit_ratio")
	m["free_disk_mb"] = getValueOr(rawData, 5000.0, "free_disk_mb", "free_mb")
	m["data_disk_mb"] = getValueOr(rawData, 50000.0, "data_disk_mb", "data_mb")
	m["log_disk_mb"] = getValueOr(rawData, 10000.0, "log_disk_mb", "log_mb")
	m["tempdb_used_pct"] = getValueOr(rawData, 30.0, "tempdb_used_percent")
	m["blocking_sessions"] = getValueOr(rawData, 0.0, "blocking_sessions")
	m["deadlocks"] = getValueOr(rawData, 0.0, "deadlocks")
	m["tps"] = getValueOr(rawData, 100.0, "tps")
	m["batch_requests_per_sec"] = getValueOr(rawData, 500.0, "batch_requests_per_sec")
	m["read_latency_ms"] = getValueOr(rawData, 5.0, "read_latency_ms")
	m["write_latency_ms"] = getValueOr(rawData, 5.0, "write_latency_ms")
	m["secondary_lag_seconds"] = getValueOr(rawData, 0.0, "secondary_lag_seconds")
	m["failed_jobs_24h"] = getValueOr(rawData, 0.0, "failed_jobs_24h")
	m["total_runnable_tasks"] = getValueOr(rawData, 0.0, "total_runnable_tasks_count", "avg_runnable_tasks_count")
	m["sort_warnings_per_sec"] = getValueOr(rawData, 0.0, "sort_warnings_per_sec")
	m["hash_warnings_per_sec"] = getValueOr(rawData, 0.0, "hash_warnings_per_sec")
	m["delta_data_mb"] = getValueOr(rawData, 0.0, "delta_data_mb")
	m["log_send_queue_kb"] = getValueOr(rawData, 0.0, "log_send_queue_kb")
	m["redo_queue_kb"] = getValueOr(rawData, 0.0, "redo_queue_kb")

	return m
}

func getValue(rawData map[string]interface{}, keys ...string) *float64 {
	for _, key := range keys {
		if v, ok := rawData[key]; ok {
			switch val := v.(type) {
			case float64:
				return &val
			case int:
				f := float64(val)
				return &f
			case []float64:
				if len(val) > 0 {
					return &val[len(val)-1]
				}
			}
		}
	}
	return nil
}

func getValueOr(rawData map[string]interface{}, def float64, keys ...string) float64 {
	if v := getValue(rawData, keys...); v != nil {
		return *v
	}
	return def
}

func buildMetricFromKeys(rawData map[string]interface{}, metricKey string, possibleKeys []string, defaultVal float64) *HealthMetric {
	metric := &HealthMetric{}
	var values []float64
	for _, key := range possibleKeys {
		if v := getValue(rawData, key); v != nil {
			values = append(values, *v)
		}
	}
	if len(values) == 0 {
		if v, ok := rawData[metricKey]; ok {
			switch val := v.(type) {
			case float64:
				values = []float64{val}
			case []float64:
				values = val
			case []interface{}:
				for _, item := range val {
					if f, ok := item.(float64); ok {
						values = append(values, f)
					}
				}
			}
		}
	}
	if len(values) == 0 {
		values = []float64{defaultVal}
	}

	metric.HistoricalSeries = values
	metric.CurrentValue = values[len(values)-1]
	metric.MinValue = utils.Min(values)
	metric.MaxValue = utils.Max(values)
	metric.AvgValue = utils.Mean(values)
	metric.MovingAvg = utils.ComputeMovingAverage(values, 5)
	metric.MovingStd = utils.ComputeMovingStd(values, 5)
	metric.TrendSlope = computeTrend(values)
	if len(values) > 5 {
		mean := utils.Mean(values)
		std := utils.StdDev(values)
		if std > 0 {
			metric.AnomalyScore = infAbs(values[len(values)-1]-mean) / std
		}
	}
	return metric
}

func makeMetric(values []float64) *HealthMetric {
	metric := &HealthMetric{}
	if len(values) > 0 {
		metric.CurrentValue = values[len(values)-1]
		metric.MinValue = utils.Min(values)
		metric.MaxValue = utils.Max(values)
		metric.AvgValue = utils.Mean(values)
		metric.HistoricalSeries = values
		metric.MovingAvg = utils.ComputeMovingAverage(values, 5)
		metric.MovingStd = utils.ComputeMovingStd(values, 5)
		metric.TrendSlope = computeTrend(values)
	}
	return metric
}

func computeTrend(values []float64) float64 {
	if len(values) < 3 {
		return 0
	}
	n := len(values)
	x := make([]float64, n)
	y := make([]float64, n)
	for i := 0; i < n; i++ {
		x[i] = float64(i)
		y[i] = values[i]
	}
	lr := utils.FitLinearRegression(x, y)
	return lr.Slope
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func infAbs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}