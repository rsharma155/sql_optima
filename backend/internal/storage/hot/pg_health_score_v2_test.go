// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TDD tests for PostgreSQL health score v2.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import "testing"

func TestHealthScore_PerfectInputsGives100(t *testing.T) {
	in := PgHealthInputs{
		ReplicationLagSeconds: 0,
		XIDWraparoundPct:      0,
		DeadTupleRatioPct:     0,
		CacheHitRatioPct:    100,
		ConnectionsUsagePct: 10,
		CheckpointReqRatio:    0,
		WALRateMBPerMin:       0,
		BlockingSessions:      0,
		DeadlocksPerMin:     0,
	}
	score, status := ComputePgHealthScore(in)
	if score != 100 {
		t.Fatalf("expected 100, got %d", score)
	}
	if status != "Healthy" {
		t.Fatalf("expected Healthy, got %s", status)
	}
}

func TestHealthScore_ZeroCacheHit_LargeScorePenalty(t *testing.T) {
	in := PgHealthInputs{
		CacheHitRatioPct: 0, // Very bad
	}
	// Other inputs default to 0 (which is good)
	score, _ := ComputePgHealthScore(in)
	// Cache hit is 15 points. drop from 100 to 0 should take at least 15 points away.
	if score > 85 {
		t.Fatalf("expected score <= 85 due to cache hit penalty, got %d", score)
	}
}

func TestHealthScore_ConnectionsAt92Pct_ReducesScore(t *testing.T) {
	in := PgHealthInputs{
		ConnectionsUsagePct: 92, // Above 90% bad threshold
	}
	score, _ := ComputePgHealthScore(in)
	// Connections is 10 points. 92% should take all 10 points.
	if score > 90 {
		t.Fatalf("expected score <= 90, got %d", score)
	}
}

func TestHealthScore_DeadlocksPresent_ReducesScore(t *testing.T) {
	in := PgHealthInputs{
		DeadlocksPerMin: 10, // Above 5/min max penalty
	}
	score, _ := ComputePgHealthScore(in)
	// Deadlocks is 5 points.
	if score > 95 {
		t.Fatalf("expected score <= 95, got %d", score)
	}
}

func TestHealthScore_ClampsToZero_WhenAllBad(t *testing.T) {
	in := PgHealthInputs{
		ReplicationLagSeconds: 1000,
		XIDWraparoundPct:      100,
		DeadTupleRatioPct:     100,
		CacheHitRatioPct:    0,
		ConnectionsUsagePct: 100,
		CheckpointReqRatio:    10,
		WALRateMBPerMin:       5000,
		BlockingSessions:      100,
		DeadlocksPerMin:     100,
	}
	score, _ := ComputePgHealthScore(in)
	if score != 0 {
		t.Fatalf("expected 0, got %d", score)
	}
}
