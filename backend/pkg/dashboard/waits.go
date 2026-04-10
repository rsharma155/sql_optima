package dashboard

import "strings"

// WaitCategory groups wait types into DBA-friendly buckets.
type WaitCategory string

const (
	WaitCPU      WaitCategory = "CPU"
	WaitIO       WaitCategory = "IO"
	WaitLog      WaitCategory = "Log"
	WaitLocking  WaitCategory = "Locking"
	WaitMemory   WaitCategory = "Memory"
	WaitNetwork  WaitCategory = "Network"
	WaitOther    WaitCategory = "Other"
)

// CategorizeWaitType maps wait types to categories based on Upgrade_main_dashboard.md patterns.
func CategorizeWaitType(waitType string) WaitCategory {
	w := strings.ToUpper(strings.TrimSpace(waitType))
	switch {
	case w == "SOS_SCHEDULER_YIELD":
		return WaitCPU
	case strings.HasPrefix(w, "PAGEIOLATCH_"):
		return WaitIO
	case w == "WRITELOG":
		return WaitLog
	case strings.HasPrefix(w, "LCK_"):
		return WaitLocking
	case w == "RESOURCE_SEMAPHORE":
		return WaitMemory
	case w == "ASYNC_NETWORK_IO":
		return WaitNetwork
	default:
		return WaitOther
	}
}

// WaitDelta is a delta sample computed from cumulative wait_time_ms.
type WaitDelta struct {
	WaitType  string       `json:"wait_type"`
	Category  WaitCategory `json:"wait_category"`
	DeltaMs   float64      `json:"wait_time_ms_delta"`
}

// ComputeWaitDeltas converts cumulative wait_time_ms totals into per-interval deltas.
// It clamps negative deltas to 0 (e.g., when SQL Server wait stats reset).
func ComputeWaitDeltas(prev map[string]float64, curr map[string]float64) (deltas []WaitDelta, nextPrev map[string]float64) {
	nextPrev = make(map[string]float64, len(curr))
	for k, v := range curr {
		nextPrev[k] = v
		p := prev[k]
		d := v - p
		if d < 0 {
			d = 0
		}
		deltas = append(deltas, WaitDelta{
			WaitType: k,
			Category: CategorizeWaitType(k),
			DeltaMs:  d,
		})
	}
	return deltas, nextPrev
}

