// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : Pure RPO and RTO calculation functions.
//           These functions have zero side effects and no external imports so
//           they are trivially unit-testable without any database or mock setup.
//
// DBA notes
//   RPO — Recovery Point Objective
//     = MAX(secondary_lag_seconds) across all SECONDARY replicas.
//     Green < 5 s, Yellow 5–60 s, Red > 60 s.
//
//   RTO — Recovery Time Objective
//     = redo_time + cluster_failover_overhead
//     redo_time  = max(redo_queue_kb) / max(redo_rate_kbps)
//                  fallback rate 100 KB/s when redo_rate == 0.
//     overhead   = 30 s (WSFC quorum re-election constant).
//     Green < 30 s, Yellow 30–120 s, Red > 120 s.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package domain

import "time"

const (
	// clusterFailoverOverheadSec is the assumed WSFC quorum re-election time
	// added to every RTO estimate.
	clusterFailoverOverheadSec int64 = 30

	// fallbackRedoRateKBPS is used when the DMV reports redo_rate == 0 to
	// avoid division-by-zero while still returning a pessimistic estimate.
	fallbackRedoRateKBPS int64 = 100
)

// ComputeRPO derives the current RPO from a slice of replica health snapshots.
// Only SECONDARY replicas contribute to the calculation.
// Returns a zero-value RPOResult when replicas is empty or contains no secondaries.
func ComputeRPO(replicas []ReplicaHealthRow) RPOResult {
	var maxLag int64
	var totalLag int64
	var secondaryCount int64
	worstReplica := ""

	for _, r := range replicas {
		if r.RoleDesc != "SECONDARY" {
			continue
		}
		secondaryCount++
		totalLag += r.SecondaryLagSeconds
		if r.SecondaryLagSeconds > maxLag {
			maxLag = r.SecondaryLagSeconds
			worstReplica = r.ReplicaServerName
		}
	}

	var avgLag int64
	if secondaryCount > 0 {
		avgLag = totalLag / secondaryCount
	}

	return RPOResult{
		Seconds:      maxLag,
		AvgSeconds:   avgLag,
		Threshold:    rpoThreshold(maxLag),
		WorstReplica: worstReplica,
		Timestamp:    time.Now().UTC(),
	}
}

// ComputeRTO derives an estimated Recovery Time Objective from replica health.
// Uses the secondary with the largest redo queue as the worst-case scenario.
func ComputeRTO(replicas []ReplicaHealthRow) RTOResult {
	var maxRedoQueueKB int64
	var maxRedoRateKBPS int64

	for _, r := range replicas {
		if r.RoleDesc != "SECONDARY" {
			continue
		}
		if r.RedoQueueKB > maxRedoQueueKB {
			maxRedoQueueKB = r.RedoQueueKB
			maxRedoRateKBPS = r.RedoRateKBPS
		}
	}

	rate := maxRedoRateKBPS
	if rate <= 0 {
		rate = fallbackRedoRateKBPS
	}

	redoSec := maxRedoQueueKB / rate
	estimatedSec := redoSec + clusterFailoverOverheadSec

	return RTOResult{
		EstimatedSeconds:   estimatedSec,
		RedoSeconds:        redoSec,
		MaxRedoQueueKB:     maxRedoQueueKB,
		ClusterOverheadSec: clusterFailoverOverheadSec,
		Threshold:          rtoThreshold(estimatedSec),
		Timestamp:          time.Now().UTC(),
	}
}

// rpoThreshold converts a lag value in seconds to a traffic-light colour.
func rpoThreshold(lagSec int64) RPOThreshold {
	switch {
	case lagSec <= 5:
		return RPOGreen
	case lagSec <= 60:
		return RPOYellow
	default:
		return RPORed
	}
}

// rtoThreshold converts an estimated RTO in seconds to a traffic-light colour.
func rtoThreshold(sec int64) RTOThreshold {
	switch {
	case sec <= 30:
		return RTOGreen
	case sec <= 120:
		return RTOYellow
	default:
		return RTORed
	}
}
