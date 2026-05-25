// Package intel provides the SQL Optima autonomous health intelligence engine.
// This file detects the server's busiest time windows from a 14-day hour-of-day CPU matrix.
//
// Design context:
//   - Input is a slice of HourlySlot values (day × hour → avg_cpu_pct) from query 7.2.
//   - Peak window = largest contiguous block of hour slots with avg CPU above the p75 threshold.
//   - Output drives time-aware anomaly detection and the UI heatmap section.
//   - Minimum required: at least 24 slots (one full day) for meaningful results.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT

package analysis

import (
	"fmt"
	"sort"

	"github.com/rsharma155/sql_optima/internal/models"
)

var dayNames = [8]string{"", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

// DetectPeakWindow analyses a 14-day hourly CPU matrix and returns the peak window descriptor.
// slots must be HourlySlot values with DayOfWeek (1-7, ISO) and HourOfDay (0-23).
// Returns nil when there are fewer than 24 usable slots.
func DetectPeakWindow(slots []models.HourlySlot) *models.PeakWindow {
	if len(slots) < 24 {
		return nil
	}

	// Sort by average CPU descending to find top slots.
	sorted := make([]models.HourlySlot, len(slots))
	copy(sorted, slots)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].AvgCPUPct > sorted[j].AvgCPUPct
	})

	// Compute p75 threshold across all slots.
	p75 := percentile75(slots)

	// Identify peak days and hours from top-25% slots.
	peakDaySet := make(map[int]bool)
	peakHourBuckets := make(map[int]int) // hour → count of peak slots

	peakCount := 0
	for _, s := range sorted {
		if s.AvgCPUPct > p75 {
			peakCount++
			peakDaySet[s.DayOfWeek] = true
			peakHourBuckets[s.HourOfDay]++
		}
	}

	// When no slot exceeds p75 (all slots at same baseline), use the top slots
	// to still produce a valid peak window descriptor.
	if peakCount == 0 {
		for _, s := range sorted[:min3(3, len(sorted))] {
			peakDaySet[s.DayOfWeek] = true
			peakHourBuckets[s.HourOfDay]++
		}
	}

	// Derive peak days list (sorted Mon–Sun).
	var peakDays []string
	for dow := 1; dow <= 7; dow++ {
		if peakDaySet[dow] {
			if dow >= 1 && dow <= 7 {
				peakDays = append(peakDays, dayNames[dow])
			}
		}
	}

	// Find the continuous hour range with highest activity.
	peakStart, peakEnd := findPeakHourRange(peakHourBuckets)
	peakHoursDesc := fmt.Sprintf("%02d:00–%02d:00", peakStart, peakEnd)

	// Compute peak vs off-peak averages.
	var peakTotal, offPeakTotal float64
	var peakN, offPeakN int
	for _, s := range slots {
		if s.AvgCPUPct > p75 {
			peakTotal += s.AvgCPUPct
			peakN++
		} else {
			offPeakTotal += s.AvgCPUPct
			offPeakN++
		}
	}
	peakAvg := safeDivide(peakTotal, float64(peakN))
	offPeakAvg := safeDivide(offPeakTotal, float64(offPeakN))

	// Top 3 slots for display.
	topSlots := sorted[:min3(3, len(sorted))]

	return &models.PeakWindow{
		TopSlots:    topSlots,
		PeakDays:    peakDays,
		PeakHours:   peakHoursDesc,
		OffPeakDesc: deriveOffPeakDesc(peakDaySet, peakStart, peakEnd),
		PeakCPUAvg:  round2(peakAvg),
		OffPeakAvg:  round2(offPeakAvg),
	}
}

// percentile75 returns the 75th percentile avg CPU across all slots.
func percentile75(slots []models.HourlySlot) float64 {
	vals := make([]float64, len(slots))
	for i, s := range slots {
		vals[i] = s.AvgCPUPct
	}
	sort.Float64s(vals)
	idx := int(float64(len(vals)) * 0.75)
	if idx >= len(vals) {
		idx = len(vals) - 1
	}
	return vals[idx]
}

// findPeakHourRange returns the start and end hour of the busiest contiguous block.
func findPeakHourRange(hourCounts map[int]int) (int, int) {
	if len(hourCounts) == 0 {
		return 9, 17
	}
	// Find hour with most activity as anchor.
	maxH, maxCount := 0, 0
	for h, c := range hourCounts {
		if c > maxCount {
			maxCount = c
			maxH = h
		}
	}
	// Expand outward while adjacent hours also have activity.
	start, end := maxH, maxH
	for h := maxH - 1; h >= 0; h-- {
		if hourCounts[h] > 0 {
			start = h
		} else {
			break
		}
	}
	for h := maxH + 1; h <= 23; h++ {
		if hourCounts[h] > 0 {
			end = h
		} else {
			break
		}
	}
	return start, end + 1 // end is exclusive (next hour boundary)
}

func deriveOffPeakDesc(peakDays map[int]bool, peakStart, peakEnd int) string {
	// Weekend is off-peak if not in peakDays.
	if !peakDays[6] && !peakDays[7] {
		return fmt.Sprintf("%02d:00–%02d:00 on weekdays, all day Saturday and Sunday", peakEnd, peakStart)
	}
	return fmt.Sprintf("%02d:00–%02d:00 daily", peakEnd, peakStart)
}

func safeDivide(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func min3(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
