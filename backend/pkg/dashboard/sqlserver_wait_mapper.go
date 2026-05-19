// Package dashboard provides wait-type categorisation helpers.
// sqlserver_wait_mapper.go extends the 7-category map from waits.go
// with a comprehensive mapping of ~50 key SQL Server wait types.
// Metadata:
//   - Feature: SQL Server Wait Stats V2
//   - Layer: Utility
package dashboard

import (
	"strings"
)

// WaitCategoryV2 extends WaitCategory with IO split and Parallelism.
type WaitCategoryV2 string

const (
	WaitV2CPU         WaitCategoryV2 = "CPU"
	WaitV2IOData      WaitCategoryV2 = "IO_DATA"
	WaitV2IOLog       WaitCategoryV2 = "IO_LOG"
	WaitV2Locking     WaitCategoryV2 = "LOCKING"
	WaitV2Memory      WaitCategoryV2 = "MEMORY"
	WaitV2Parallelism WaitCategoryV2 = "PARALLELISM"
	WaitV2Network     WaitCategoryV2 = "NETWORK"
	WaitV2Other       WaitCategoryV2 = "OTHER"
)

// waitCategoryV2Map is the exhaustive lookup. Prefix matches handle
// wildcard families (LCK_*, PAGEIOLATCH_*) after exact lookup fails.
var waitCategoryV2Map = map[string]WaitCategoryV2{
	"SOS_SCHEDULER_YIELD":              WaitV2CPU,
	"THREADPOOL":                       WaitV2CPU,
	"CMEMTHREAD":                       WaitV2CPU,
	"PAGEIOLATCH_SH":                   WaitV2IOData,
	"PAGEIOLATCH_EX":                   WaitV2IOData,
	"PAGEIOLATCH_UP":                   WaitV2IOData,
	"PAGEIOLATCH_KP":                   WaitV2IOData,
	"PAGEIOLATCH_NL":                   WaitV2IOData,
	"IO_COMPLETION":                    WaitV2IOData,
	"ASYNC_IO_COMPLETION":              WaitV2IOData,
	"WRITELOG":                         WaitV2IOLog,
	"LOGBUFFER":                        WaitV2IOLog,
	"LOGMGR":                           WaitV2IOLog,
	"LOGMGR_FLUSH":                     WaitV2IOLog,
	"LOGMGR_RESERVE_APPEND":            WaitV2IOLog,
	"RESOURCE_SEMAPHORE":               WaitV2Memory,
	"RESOURCE_SEMAPHORE_QUERY_COMPILE": WaitV2Memory,
	"CMEMPARTITIONED":                  WaitV2Memory,
	"CXPACKET":                         WaitV2Parallelism,
	"CXCONSUMER":                       WaitV2Parallelism,
	"ASYNC_NETWORK_IO":                 WaitV2Network,
	"NET_WAITFOR_PACKET":               WaitV2Network,
}

// CategorizeWaitTypeV2 returns the category for a given wait type.
// Performs exact lookup first, then prefix matching for LCK_* and PAGEIOLATCH_*.
func CategorizeWaitTypeV2(waitType string) WaitCategoryV2 {
	w := strings.ToUpper(strings.TrimSpace(waitType))
	if cat, ok := waitCategoryV2Map[w]; ok {
		return cat
	}
	switch {
	case strings.HasPrefix(w, "LCK_"):
		return WaitV2Locking
	case strings.HasPrefix(w, "PAGEIOLATCH_"):
		return WaitV2IOData
	case strings.HasPrefix(w, "PAGELATCH_"):
		return WaitV2IOData
	}
	return WaitV2Other
}

// WaitDeltaRowV2 holds a per-type delta row with category assigned.
type WaitDeltaRowV2 struct {
	WaitType            string
	Category            WaitCategoryV2
	DeltaWaitMs         int64
	DeltaSignalWaitMs   int64
	DeltaResourceWaitMs int64
	DeltaWaitingTasks   int64
	RestartDetected     bool
}

// ComputeWaitDeltasV2 converts consecutive cumulative snapshots to deltas.
// prev and curr are maps of wait_type → [wait_ms, signal_ms, tasks].
// Negative deltas (SQL Server restart) are clamped to 0 with RestartDetected=true.
func ComputeWaitDeltasV2(
	prev map[string][3]int64, // [wait_ms, signal_ms, tasks]
	curr map[string][3]int64,
) []WaitDeltaRowV2 {
	out := make([]WaitDeltaRowV2, 0, len(curr))
	for wt, cv := range curr {
		pv := prev[wt]
		dWait := cv[0] - pv[0]
		dSignal := cv[1] - pv[1]
		dTasks := cv[2] - pv[2]
		restart := dWait < 0 || dSignal < 0
		if dWait < 0 {
			dWait = 0
		}
		if dSignal < 0 {
			dSignal = 0
		}
		if dTasks < 0 {
			dTasks = 0
		}
		dResource := dWait - dSignal
		if dResource < 0 {
			dResource = 0
		}
		out = append(out, WaitDeltaRowV2{
			WaitType:            wt,
			Category:            CategorizeWaitTypeV2(wt),
			DeltaWaitMs:         dWait,
			DeltaSignalWaitMs:   dSignal,
			DeltaResourceWaitMs: dResource,
			DeltaWaitingTasks:   dTasks,
			RestartDetected:     restart,
		})
	}
	return out
}
