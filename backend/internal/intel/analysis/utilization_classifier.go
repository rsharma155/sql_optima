// Package intel provides the SQL Optima autonomous health intelligence engine.
// This file classifies server resource utilization into four bands using 14-day
// CPU and memory percentile data from sqlserver_metrics.
//
// Design context:
//   - Classification uses p95 CPU and p95 memory as primary signals.
//   - "Under-utilized" servers may be candidates for workload consolidation.
//   - "Over-utilized" servers need hardware or workload migration within the next cycle.
//   - Under-utilization recommendations are generated when cpu_avg < 10% and mem_p50 < 40%.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT

package analysis

import "github.com/rsharma155/sql_optima/internal/models"

// ClassifyUtilization produces a UtilizationProfile from precomputed percentile data.
// The raw input map must contain keys cpu_p50, cpu_p90, cpu_p95, cpu_p99, cpu_avg,
// mem_p50, mem_p95, mem_avg, sample_count — as populated by the service layer query 7.3.
// Returns nil when insufficient data exists (fewer than 10 samples).
func ClassifyUtilization(percentiles map[string]float64) *models.UtilizationProfile {
	if percentiles == nil {
		return nil
	}

	sampleCount := int(percentiles["sample_count"])
	if sampleCount < 10 {
		return nil
	}

	cpuP50 := percentiles["cpu_p50"]
	cpuP95 := percentiles["cpu_p95"]
	cpuAvg := percentiles["cpu_avg"]
	memP50 := percentiles["mem_p50"]
	memP95 := percentiles["mem_p95"]

	class, msg := classifyBand(cpuP95, memP95)

	// Refine under-utilization with additional rightsizing note.
	if class == "Under-utilized" && cpuAvg < 10 && memP50 < 40 {
		msg = "CPU average is below 10% and memory p50 is below 40%. " +
			"This server is a strong candidate for VM or workload consolidation."
	}

	return &models.UtilizationProfile{
		CPUP50:      cpuP50,
		CPUP90:      percentiles["cpu_p90"],
		CPUP95:      cpuP95,
		CPUP99:      percentiles["cpu_p99"],
		CPUAvg:      cpuAvg,
		MemP50:      memP50,
		MemP95:      memP95,
		MemAvg:      percentiles["mem_avg"],
		Class:       class,
		Message:     msg,
		SampleCount: sampleCount,
	}
}

// classifyBand applies the four-band classification algorithm.
func classifyBand(cpuP95, memP95 float64) (string, string) {
	switch {
	case cpuP95 < 30 && memP95 < 50:
		return "Under-utilized",
			"Server resources are significantly under-used. Consider consolidating workloads or right-sizing."
	case cpuP95 < 60 && memP95 < 75:
		return "Optimal",
			"Resource utilization is healthy with adequate headroom for growth."
	case cpuP95 < 85 || memP95 < 90:
		return "Elevated",
			"Resources are under moderate pressure. Monitor growth trends and plan capacity accordingly."
	default:
		return "Over-utilized",
			"Server is under sustained resource pressure. Hardware upgrade or workload migration is recommended."
	}
}
