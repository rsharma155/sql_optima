// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : Feature detection logic tests.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package tests

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/domain"
)

func TestFeatureDetection_ShouldShowPage(t *testing.T) {
	tests := []struct {
		name     string
		feature  domain.FeatureDetection
		expected bool
	}{
		{
			name:     "Neither enabled",
			feature:  domain.FeatureDetection{HAEnabled: false, ReplicationEnabled: false},
			expected: false,
		},
		{
			name:     "HA enabled only",
			feature:  domain.FeatureDetection{HAEnabled: true, ReplicationEnabled: false},
			expected: true,
		},
		{
			name:     "Replication enabled only",
			feature:  domain.FeatureDetection{HAEnabled: false, ReplicationEnabled: true},
			expected: true,
		},
		{
			name:     "Both enabled",
			feature:  domain.FeatureDetection{HAEnabled: true, ReplicationEnabled: true},
			expected: true,
		},
		{
			name:     "AG only (HAEnabled derived check)",
			feature:  domain.FeatureDetection{AGEnabled: true, HAEnabled: true},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.feature.ShouldShowPage(); got != tt.expected {
				t.Errorf("FeatureDetection.ShouldShowPage() = %v, want %v", got, tt.expected)
			}
		})
	}
}
