// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Tests for PostgreSQL Instance Health Service.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package instance_health

import (
	"testing"
)

func TestCalculateHealthScore(t *testing.T) {
	service := NewInstanceHealthService(nil)

	tests := []struct {
		name     string
		snapshot *PgInstanceSnapshot
		want     int
	}{
		{
			name: "Perfect Health",
			snapshot: &PgInstanceSnapshot{
				BlockedSessions:  0,
				CacheHitRatio:    99,
				ReplicaLagSec:    0,
				MaxXIDAge:        1000,
				DeadTuplePct:     5,
				CheckpointsTimed: 10,
				CheckpointsReq:   1,
			},
			want: 100,
		},
		{
			name: "Blocked Sessions Penalty",
			snapshot: &PgInstanceSnapshot{
				BlockedSessions: 5,
				CacheHitRatio:   99,
			},
			want: 80, // 100 - 20
		},
		{
			name: "Low Cache Hit Penalty",
			snapshot: &PgInstanceSnapshot{
				BlockedSessions: 0,
				CacheHitRatio:   90,
			},
			want: 90, // 100 - 10
		},
		{
			name: "Replica Lag Penalty (moderate)",
			snapshot: &PgInstanceSnapshot{
				CacheHitRatio: 99,
				ReplicaLagSec: 60, // == 60, falls to >30 tier
			},
			want: 90, // 100 - 10
		},
		{
			name: "Replica Lag Penalty (severe)",
			snapshot: &PgInstanceSnapshot{
				CacheHitRatio: 99,
				ReplicaLagSec: 61, // > 60
			},
			want: 85, // 100 - 15
		},
		{
			name: "XID Age Penalty (critical)",
			snapshot: &PgInstanceSnapshot{
				CacheHitRatio: 99,
				MaxXIDAge:     2000000000, // > 1.5B
			},
			want: 75, // 100 - 25
		},
		{
			name: "XID Age Penalty (warning)",
			snapshot: &PgInstanceSnapshot{
				CacheHitRatio: 99,
				MaxXIDAge:     1200000000, // > 1B, <= 1.5B
			},
			want: 85, // 100 - 15
		},
		{
			name: "High Bloat Penalty",
			snapshot: &PgInstanceSnapshot{
				CacheHitRatio: 99,
				DeadTuplePct:  25,
			},
			want: 90, // 100 - 10
		},
		{
			name: "Checkpoint Pressure Penalty",
			snapshot: &PgInstanceSnapshot{
				CacheHitRatio:    99,
				CheckpointsTimed: 10,
				CheckpointsReq:   15, // > 0.5 ratio
			},
			want: 90, // 100 - 10
		},
		{
			name: "Multiple Penalties",
			snapshot: &PgInstanceSnapshot{
				BlockedSessions: 1,
				CacheHitRatio:   80, // < 90 → -20
				ReplicaLagSec:   100, // > 60 → -15
			},
			want: 45, // 100 - 20 (blocked) - 20 (cache<90) - 15 (lag>60)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.CalculateHealthScore(tt.snapshot); got != tt.want {
				t.Errorf("CalculateHealthScore() = %v, want %v", got, tt.want)
			}
		})
	}
}
