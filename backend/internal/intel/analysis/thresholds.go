// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package analysis




import (
	"math"

	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/intel/utils"
)

type DynamicThresholdCalculator struct{}

func NewDynamicThresholdCalculator() *DynamicThresholdCalculator {
	return &DynamicThresholdCalculator{}
}

func (c *DynamicThresholdCalculator) Compute(config models.ServerConfig, metricHistory map[string][]float64) models.DynamicThresholds {
	t := models.DynamicThresholds{}
	c.computeCPUThresholds(&t, config, metricHistory)
	c.computeMemoryThresholds(&t, config, metricHistory)
	c.computeDiskThresholds(&t, config, metricHistory)
	c.computeIOThresholds(&t, config, metricHistory)
	c.computeReplicationThresholds(&t, config, metricHistory)
	c.computeQueryThresholds(&t, config, metricHistory)
	c.computeTempDBThresholds(&t, config, metricHistory)
	return t
}

func (c *DynamicThresholdCalculator) computeCPUThresholds(t *models.DynamicThresholds, config models.ServerConfig, hist map[string][]float64) {
	cpuData := hist["avg_cpu_load"]
	runnableData := hist["total_runnable_tasks_count"]
	if runnableData == nil {
		runnableData = hist["avg_runnable_tasks_count"]
	}

	baseMax := 95.0
	if !config.IsVirtual {
		baseMax = 90.0
	}
	t.CPUMaxThreshold = baseMax

	if len(cpuData) > 0 {
		p95 := utils.Percentile(cpuData, 95)
		t.CPUSustainedThreshold = math.Min(baseMax, math.Max(p95*1.15, baseMax-15))
	} else {
		t.CPUSustainedThreshold = math.Min(baseMax, 70+float64(config.CPUCount)*1.5)
	}

	if len(runnableData) > 0 {
		avgRunnable := utils.Mean(runnableData)
		t.CPURunnablePerCore = avgRunnable / math.Max(float64(config.CPUCount), 1)
		t.CPURunnableAbsolute = int(math.Max(avgRunnable*1.5, float64(config.CPUCount)*3))
	} else {
		t.CPURunnablePerCore = 0.5
		t.CPURunnableAbsolute = config.CPUCount * 3
	}
}

func (c *DynamicThresholdCalculator) computeMemoryThresholds(t *models.DynamicThresholds, config models.ServerConfig, hist map[string][]float64) {
	pleData := hist["ple_seconds"]
	if pleData == nil {
		pleData = hist["ple"]
	}
	grantsData := hist["memory_grants_pending"]
	if grantsData == nil {
		grantsData = hist["waiting_memory_grants"]
	}
	memUsedData := hist["memory_usage"]

	// RAM-based PLE floor: Microsoft's guidance is PLE ≥ (buffer_pool_GB / 4) × 300.
	// We approximate buffer pool as 85% of total RAM (typical max_server_memory allocation).
	// Minimum absolute floor is 300s per MSDN; for large servers this formula is far more accurate.
	ramGBF := math.Max(float64(config.TotalRAMGB), 8)
	bufferPoolGB := ramGBF * 0.85
	ramBasedFloor := math.Max(300.0, (bufferPoolGB/4.0)*300.0)

	if len(pleData) > 0 {
		pleBaseline := utils.Percentile(pleData, 50)
		// Alert when PLE drops below 50% of the healthy baseline OR the RAM-based floor,
		// whichever is higher. The old 0.2 multiplier allowed PLE to fall 80% before alerting,
		// which masks genuine memory pressure until it is severe.
		historicalFloor := math.Max(300.0, pleBaseline*0.5)
		t.MemoryPLEMinSeconds = math.Max(ramBasedFloor, historicalFloor)
	} else {
		t.MemoryPLEMinSeconds = ramBasedFloor
	}

	if len(grantsData) > 0 {
		avgGrants := utils.Mean(grantsData)
		stdGrants := utils.StdDev(grantsData)
		t.MemoryGrantsPendingMax = int(math.Max(1, avgGrants+2*stdGrants))
	} else {
		t.MemoryGrantsPendingMax = int(math.Max(1, float64(config.TotalRAMGB)/8))
	}

	if len(memUsedData) > 0 {
		p95 := utils.Percentile(memUsedData, 95)
		t.MemoryUsedPctMax = math.Min(98, p95*1.15)
	} else {
		t.MemoryUsedPctMax = 85.0
	}

	bchrData := hist["buffer_cache_hit_ratio"]
	if len(bchrData) > 0 {
		// Alert when hit ratio falls below the historical p10 or 5 points under p50, floored at 90%.
		p10 := utils.Percentile(bchrData, 10)
		p50 := utils.Percentile(bchrData, 50)
		t.BufferCacheHitRatioMin = math.Max(90, math.Min(99, math.Min(p10, p50-5)))
	} else {
		t.BufferCacheHitRatioMin = 95.0
	}

	sortWarnData := hist["sort_warnings_per_sec"]
	if len(sortWarnData) > 0 {
		avg := utils.Mean(sortWarnData)
		std := utils.StdDev(sortWarnData)
		t.SortWarningsPerSecMax = math.Max(0.5, avg+2*std)
	} else {
		t.SortWarningsPerSecMax = 1.0
	}

	hashWarnData := hist["hash_warnings_per_sec"]
	if len(hashWarnData) > 0 {
		avg := utils.Mean(hashWarnData)
		std := utils.StdDev(hashWarnData)
		t.HashWarningsPerSecMax = math.Max(0.5, avg+2*std)
	} else {
		t.HashWarningsPerSecMax = 1.0
	}
}

func (c *DynamicThresholdCalculator) computeDiskThresholds(t *models.DynamicThresholds, config models.ServerConfig, hist map[string][]float64) {
	freeMBData := hist["free_disk_mb"]
	if freeMBData == nil {
		freeMBData = hist["free_mb"]
	}
	deltaData := hist["delta_data_mb"]
	if deltaData == nil {
		deltaData = hist["delta_log_mb"]
	}

	tenPct := float64(config.TotalDiskGB) * 1024 * 0.1
	if len(freeMBData) > 0 {
		histMin := utils.Min(freeMBData)
		twentyPct := histMin * 0.2
		if twentyPct <= 0 {
			twentyPct = tenPct
		}
		t.DiskFreeMBMin = math.Max(tenPct, twentyPct)
	} else {
		t.DiskFreeMBMin = math.Max(10240, tenPct)
	}

	if len(deltaData) > 3 {
		growthMean := utils.Mean(deltaData)
		growthStd := utils.StdDev(deltaData)
		t.DiskGrowthRateMaxMBPerDay = math.Max(100, (growthMean+2*growthStd)*24*4)
	} else {
		t.DiskGrowthRateMaxMBPerDay = 500
	}
}

func (c *DynamicThresholdCalculator) computeIOThresholds(t *models.DynamicThresholds, config models.ServerConfig, hist map[string][]float64) {
	readLatency := hist["read_latency_ms"]
	writeLatency := hist["write_latency_ms"]
	allLatency := append(readLatency, writeLatency...)

	if len(allLatency) > 0 {
		p95 := utils.Percentile(allLatency, 95)
		// Floor of 5ms for virtual (cloud NVMe), 10ms for physical (may be HDD/SAN).
		// p95×1.5 gives a workload-adaptive ceiling so a historically slow SAN doesn't
		// mask real latency degradation.
		floor := 10.0
		if config.IsVirtual {
			floor = 5.0
		}
		t.IOLatencyMaxMS = math.Max(floor, p95*1.5)
	} else {
		// Default when no history: virtual cloud instances should be on NVMe-backed
		// storage (alert at 10ms); physical servers may have spinning disk (alert at 20ms).
		if config.IsVirtual {
			t.IOLatencyMaxMS = 10.0
		} else {
			t.IOLatencyMaxMS = 20.0
		}
	}
}

func (c *DynamicThresholdCalculator) computeReplicationThresholds(t *models.DynamicThresholds, config models.ServerConfig, hist map[string][]float64) {
	lagData := hist["secondary_lag_seconds"]
	if lagData == nil {
		lagData = hist["latency_seconds"]
	}

	if len(lagData) > 0 {
		p95 := utils.Percentile(lagData, 95)
		t.ReplicationLagMaxSeconds = math.Max(30, p95*2)
	} else {
		t.ReplicationLagMaxSeconds = 60.0
	}
}

func (c *DynamicThresholdCalculator) computeQueryThresholds(t *models.DynamicThresholds, config models.ServerConfig, hist map[string][]float64) {
	blockData := hist["blocking_sessions"]
	durationData := hist["total_elapsed_time_ms"]
	if durationData == nil {
		durationData = hist["query_duration_seconds"]
	}
	batchData := hist["batch_requests_per_sec"]

	if len(blockData) > 0 {
		p95 := utils.Percentile(blockData, 95)
		t.BlockingSessionMax = int(math.Max(1, math.Min(5, p95*2)))
	} else {
		t.BlockingSessionMax = 5
	}

	if len(durationData) > 0 {
		p95 := utils.Percentile(durationData, 95)
		t.QueryDurationMaxSeconds = math.Max(30, p95*1.5) / 1000.0
	} else {
		t.QueryDurationMaxSeconds = 300.0
	}

	if len(batchData) > 0 {
		avgBatch := utils.Mean(batchData)
		t.BatchRequestsPerCoreMax = avgBatch * 2 / math.Max(float64(config.CPUCount), 1)
	} else {
		t.BatchRequestsPerCoreMax = 200.0
	}
}

func (c *DynamicThresholdCalculator) computeTempDBThresholds(t *models.DynamicThresholds, config models.ServerConfig, hist map[string][]float64) {
	tempdbData := hist["tempdb_used_percent"]
	if tempdbData == nil {
		tempdbData = hist["tempdb_used_pct"]
	}

	if len(tempdbData) > 0 {
		p95 := utils.Percentile(tempdbData, 95)
		t.TempDBUsedPctMax = math.Min(95, p95*1.3)
	} else {
		t.TempDBUsedPctMax = 80.0
	}
}

func DefaultServerConfig(rawData map[string]interface{}) models.ServerConfig {
	cfg := models.ServerConfig{
		CPUCount:         8,
		TotalRAMGB:       32,
		TotalDiskGB:      500,
		HyperthreadRatio: 2,
		SocketCount:      2,
		CoresPerSocket:   4,
		NumaNodes:        2,
		MaxWorkers:       1024,
		IsVirtual:        true,
	}

	readInt := func(keys ...string) *int {
		for _, key := range keys {
			if v, ok := rawData[key].(float64); ok {
				val := int(v)
				return &val
			}
		}
		return nil
	}

	readBool := func(key string, def bool) bool {
		if v, ok := rawData[key].(bool); ok {
			return v
		}
		return def
	}

	if v := readInt("cpu_count"); v != nil {
		cfg.CPUCount = *v
	}
	if v := readInt("total_ram_gb", "physical_memory_gb"); v != nil {
		cfg.TotalRAMGB = *v
	}
	if v := readInt("total_disk_gb"); v != nil {
		cfg.TotalDiskGB = *v
	}
	if v := readInt("hyperthread_ratio"); v != nil {
		cfg.HyperthreadRatio = *v
	}
	if v := readInt("socket_count"); v != nil {
		cfg.SocketCount = *v
	}
	if v := readInt("cores_per_socket"); v != nil {
		cfg.CoresPerSocket = *v
	}
	if v := readInt("numa_nodes", "total_node_count"); v != nil {
		cfg.NumaNodes = *v
	}
	if v := readInt("max_workers_count"); v != nil {
		cfg.MaxWorkers = *v
	}
	cfg.IsVirtual = readBool("is_virtual", true)

	return cfg
}

func ExtractHistoriesFromRaw(rawData map[string]interface{}) map[string][]float64 {
	histories := make(map[string][]float64)
	for k, v := range rawData {
		if len(k) > 7 && k[len(k)-7:] == "_series" {
			baseKey := k[:len(k)-7]
			switch arr := v.(type) {
			case []float64:
				histories[baseKey] = arr
			case []interface{}:
				floats := make([]float64, 0, len(arr))
				for _, item := range arr {
					if f, ok := toFloat64(item); ok {
						floats = append(floats, f)
					}
				}
				if len(floats) > 0 {
					histories[baseKey] = floats
				}
			case []int:
				floats := make([]float64, len(arr))
				for i, n := range arr {
					floats[i] = float64(n)
				}
				histories[baseKey] = floats
			}
		}
	}
	return histories
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint64:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint16:
		return float64(val), true
	case uint8:
		return float64(val), true
	case uint:
		return float64(val), true
	default:
		return 0, false
	}
}