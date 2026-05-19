// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package utils




import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

func SnakeToPascal(name string) string {
	parts := strings.Split(name, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

func PascalToSnake(name string) string {
	re := regexp.MustCompile(`(.)([A-Z])`)
	snake := re.ReplaceAllString(name, "${1}_${2}")
	return strings.ToLower(snake)
}

func SafeDivide(numerator, denominator, defaultVal float64) float64 {
	if denominator == 0 {
		return defaultVal
	}
	return numerator / denominator
}

func DaysBetween(d1, d2 time.Time) float64 {
	diff := d2.Sub(d1)
	if diff < 0 {
		diff = -diff
	}
	return diff.Hours() / 24.0
}

func ClassifyMetricName(name string) string {
	lower := strings.ToLower(name)
	if containsAny(lower, "cpu", "processor", "scheduler", "runnable") {
		return "cpu"
	}
	if containsAny(lower, "memory", "ple", "page", "buffer", "grant") {
		return "memory"
	}
	if containsAny(lower, "disk", "storage", "volume", "drive", "space") {
		return "disk"
	}
	if containsAny(lower, "i/o", "io_", "latency", "throughput", "iops") {
		return "io"
	}
	if containsAny(lower, "query", "execution", "plan", "spill") {
		return "query"
	}
	if containsAny(lower, "block", "lock", "deadlock", "wait") {
		return "blocking"
	}
	if containsAny(lower, "replicat", "log_shipping", "sync") {
		return "replication"
	}
	if containsAny(lower, "backup", "restore", "recovery") {
		return "backup"
	}
	if containsAny(lower, "index", "fragmentation", "statistic") {
		return "index"
	}
	if containsAny(lower, "tempdb", "templog") {
		return "tempdb"
	}
	if containsAny(lower, "network", "tcp", "connection") {
		return "network"
	}
	return "other"
}

func ComputeMovingAverage(values []float64, window int) []float64 {
	if len(values) == 0 {
		return nil
	}
	result := make([]float64, len(values))
	for i := range values {
		start := i - window + 1
		if start < 0 {
			start = 0
		}
		chunk := values[start : i+1]
		sum := 0.0
		for _, v := range chunk {
			sum += v
		}
		result[i] = sum / float64(len(chunk))
	}
	return result
}

func ComputeMovingStd(values []float64, window int) []float64 {
	if len(values) == 0 {
		return nil
	}
	result := make([]float64, len(values))
	for i := range values {
		start := i - window + 1
		if start < 0 {
			start = 0
		}
		chunk := values[start : i+1]
		mean := 0.0
		for _, v := range chunk {
			mean += v
		}
		mean /= float64(len(chunk))
		variance := 0.0
		for _, v := range chunk {
			d := v - mean
			variance += d * d
		}
		variance /= float64(len(chunk))
		result[i] = math.Sqrt(variance)
	}
	return result
}

func FormatDuration(minutes float64) string {
	if minutes < 60 {
		return fmt.Sprintf("%.1f minutes", minutes)
	}
	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("%.1f hours", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%.1f days", days)
}

func Mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func StdDev(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	mean := Mean(values)
	variance := 0.0
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	return math.Sqrt(variance / float64(len(values)))
}

func Percentile(values []float64, pct float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sortSlice(sorted)
	idx := pct / 100.0 * float64(len(sorted)-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[lo]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func Min(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func Max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func sortSlice(a []float64) {
	for i := 0; i < len(a)-1; i++ {
		for j := i + 1; j < len(a); j++ {
			if a[i] > a[j] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}