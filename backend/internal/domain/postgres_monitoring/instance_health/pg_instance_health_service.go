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

	// Signal: blocked sessions >0 Penalty: -20
	if snapshot.BlockedSessions > 0 {
		score -= 20
	}

	// Signal: cache hit <95 Penalty: -10
	if snapshot.CacheHitRatio < 95 {
		score -= 10
	}

	// Signal: replica lag >30s Penalty: -15
	if snapshot.ReplicaLagSec > 30 {
		score -= 15
	}

	// Signal: xid age >1B Penalty: -20
	if snapshot.MaxXIDAge > 1000000000 {
		score -= 20
	}

	// Signal: dead tuples >20% Penalty: -10
	if snapshot.DeadTuplePct > 20 {
		score -= 10
	}

	// Signal: checkpoint req/timed >0.5 Penalty: -10
	if snapshot.CheckpointsTimed+snapshot.CheckpointsReq > 0 {
		ratio := float64(snapshot.CheckpointsReq) / float64(snapshot.CheckpointsTimed+snapshot.CheckpointsReq)
		if ratio > 0.5 {
			score -= 10
		}
	}

	if score < 0 {
		score = 0
	}
	return score
}
