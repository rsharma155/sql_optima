// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL health score calculation from multiple metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import "math"

type PgHealthInputs struct {
	ReplicationLagSeconds float64
	XIDWraparoundPct      float64
	DeadTupleRatioPct     float64
	CheckpointReqRatio    float64
	WALRateMBPerMin       float64
	BlockingSessions      int
	// New — 2026-05-07
	CacheHitRatioPct    float64 // 0–100; below 95 = I/O pressure
	ConnectionsUsagePct float64 // 0–100; above 85 = saturation risk
	DeadlocksPerMin     float64 // 0 = healthy; >0 = immediate attention
}

// ComputePgHealthScore returns score (0-100) and status.
// Thresholds are intentionally simple and can be tuned later; the key is stable semantics.
func ComputePgHealthScore(in PgHealthInputs) (score int, status string) {
	scoreF := 0.0

	// 20: Replication lag (seconds)
	scoreF += weightedLinear(in.ReplicationLagSeconds, 0, 300, 20)
	// 15: XID age risk (% toward freeze max age)
	scoreF += weightedLinear(in.XIDWraparoundPct, 0, 95, 15)
	// 15: Dead tuple ratio %
	scoreF += weightedLinear(in.DeadTupleRatioPct, 0, 50, 15)
	// 15: Cache Hit Ratio % (penalize below 99% toward 90%)
	scoreF += weightedLinear(100.0-in.CacheHitRatioPct, 1.0, 10.0, 15)
	// 10: Connections Usage % (penalize above 60% toward 90%)
	scoreF += weightedLinear(in.ConnectionsUsagePct, 60, 90, 10)
	// 10: Checkpoint pressure (req/timed); >1 means mostly requested
	scoreF += weightedLinear(in.CheckpointReqRatio, 0, 3.0, 10)
	// 5: WAL gen rate MB/min
	scoreF += weightedLinear(in.WALRateMBPerMin, 0, 1000, 5)
	// 5: Blocking sessions
	scoreF += weightedLinear(float64(in.BlockingSessions), 0, 20, 5)
	// 5: Deadlocks/min
	scoreF += weightedLinear(in.DeadlocksPerMin, 0, 5, 5)

	score = int(math.Round(scoreF))
	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}

	switch {
	case score >= 90:
		status = "Healthy"
	case score >= 70:
		status = "Watch"
	default:
		status = "At Risk"
	}
	return score, status
}

// weightedLinear maps value in [good..bad] to [weight..0] linearly. Clamps outside range.
func weightedLinear(value, good, bad, weight float64) float64 {
	if weight <= 0 {
		return 0
	}
	if bad <= good {
		if value <= good {
			return weight
		}
		return 0
	}
	if value <= good {
		return weight
	}
	if value >= bad {
		return 0
	}
	// between good and bad -> linearly decay
	f := 1.0 - ((value - good) / (bad - good))
	if f < 0 {
		f = 0
	} else if f > 1 {
		f = 1
	}
	return f * weight
}
