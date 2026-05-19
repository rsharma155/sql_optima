// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Service for PostgreSQL Instance Health.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package instance_health

type InstanceHealthService struct {
	repo *InstanceHealthRepository
}

func NewInstanceHealthService(repo *InstanceHealthRepository) *InstanceHealthService {
	return &InstanceHealthService{repo: repo}
}

func (s *InstanceHealthService) CalculateHealthScore(snapshot *PgInstanceSnapshot) int {
	score := 100

	// Signal 1: blocked sessions >0 → -20 (any blocking is severe)
	if snapshot.BlockedSessions > 0 {
		score -= 20
	}

	// Signal 2: cache hit ratio <95% → -10, <90% → -20
	if snapshot.CacheHitRatio < 90 {
		score -= 20
	} else if snapshot.CacheHitRatio < 95 {
		score -= 10
	}

	// Signal 3: replica lag >30s → -10, >60s → -15
	if snapshot.ReplicaLagSec > 60 {
		score -= 15
	} else if snapshot.ReplicaLagSec > 30 {
		score -= 10
	}

	// Signal 4: XID age >1B → -15, >1.5B → -25 (approaching wraparound danger)
	if snapshot.MaxXIDAge > 1500000000 {
		score -= 25
	} else if snapshot.MaxXIDAge > 1000000000 {
		score -= 15
	}

	// Signal 5: dead tuples >20% → -10
	if snapshot.DeadTuplePct > 20 {
		score -= 10
	}

	// Signal 6: checkpoint req ratio >0.5 → -10 (WAL pressure causing forced checkpoints)
	if snapshot.CheckpointsTimed+snapshot.CheckpointsReq > 0 {
		ratio := float64(snapshot.CheckpointsReq) / float64(snapshot.CheckpointsTimed+snapshot.CheckpointsReq)
		if ratio > 0.5 {
			score -= 10
		}
	}

	// Signal 7: connection saturation >85% → -10 (RAM exhaustion and fork-storm precursor)
	if snapshot.ConnectionsUsagePct > 85 {
		score -= 10
	}

	// Signal 8: idle-in-transaction sessions >3 → -10 (autovacuum suppressor)
	if snapshot.IdleInTxSessions > 3 {
		score -= 10
	}

	if score < 0 {
		score = 0
	}
	return score
}
