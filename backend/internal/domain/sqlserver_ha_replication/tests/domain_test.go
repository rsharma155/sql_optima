// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : Unit tests for pure domain functions — RPO/RTO calculation and
//           failover readiness checklist evaluation.
//           No database, no mocks, no external dependencies required.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package tests

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/domain"
)

// ---------------------------------------------------------------------------
// RPO tests
// ---------------------------------------------------------------------------

func TestComputeRPO_NoReplicas_ReturnsZero(t *testing.T) {
	result := domain.ComputeRPO(nil)
	if result.Seconds != 0 {
		t.Errorf("expected RPO 0 for empty replicas, got %d", result.Seconds)
	}
	if result.Threshold != domain.RPOGreen {
		t.Errorf("expected green threshold for zero lag, got %s", result.Threshold)
	}
}

func TestComputeRPO_PrimaryOnly_ReturnsZero(t *testing.T) {
	replicas := []domain.ReplicaHealthRow{
		{RoleDesc: "PRIMARY", SecondaryLagSeconds: 100},
	}
	result := domain.ComputeRPO(replicas)
	// Primary does not contribute to RPO
	if result.Seconds != 0 {
		t.Errorf("primary should not contribute to RPO, got %d", result.Seconds)
	}
}

func TestComputeRPO_GreenThreshold(t *testing.T) {
	replicas := []domain.ReplicaHealthRow{
		{RoleDesc: "SECONDARY", ReplicaServerName: "DR1", SecondaryLagSeconds: 3},
	}
	result := domain.ComputeRPO(replicas)
	if result.Seconds != 3 {
		t.Errorf("expected RPO 3s, got %d", result.Seconds)
	}
	if result.Threshold != domain.RPOGreen {
		t.Errorf("expected green threshold for 3s lag, got %s", result.Threshold)
	}
}

func TestComputeRPO_YellowThreshold(t *testing.T) {
	replicas := []domain.ReplicaHealthRow{
		{RoleDesc: "SECONDARY", ReplicaServerName: "DR1", SecondaryLagSeconds: 30},
	}
	result := domain.ComputeRPO(replicas)
	if result.Threshold != domain.RPOYellow {
		t.Errorf("expected yellow threshold for 30s lag, got %s", result.Threshold)
	}
}

func TestComputeRPO_RedThreshold(t *testing.T) {
	replicas := []domain.ReplicaHealthRow{
		{RoleDesc: "SECONDARY", ReplicaServerName: "DR1", SecondaryLagSeconds: 120},
	}
	result := domain.ComputeRPO(replicas)
	if result.Threshold != domain.RPORed {
		t.Errorf("expected red threshold for 120s lag, got %s", result.Threshold)
	}
	if result.ReplicaName != "DR1" {
		t.Errorf("expected worst replica DR1, got %s", result.ReplicaName)
	}
}

func TestComputeRPO_WorstReplicaSelected(t *testing.T) {
	replicas := []domain.ReplicaHealthRow{
		{RoleDesc: "SECONDARY", ReplicaServerName: "DR1", SecondaryLagSeconds: 10},
		{RoleDesc: "SECONDARY", ReplicaServerName: "DR2", SecondaryLagSeconds: 90},
	}
	result := domain.ComputeRPO(replicas)
	if result.Seconds != 90 {
		t.Errorf("expected max lag 90s, got %d", result.Seconds)
	}
	if result.ReplicaName != "DR2" {
		t.Errorf("expected worst replica DR2, got %s", result.ReplicaName)
	}
}

// ---------------------------------------------------------------------------
// RTO tests
// ---------------------------------------------------------------------------

func TestComputeRTO_NoReplicas_ReturnsOverheadOnly(t *testing.T) {
	result := domain.ComputeRTO(nil)
	// When no replicas: redo_time=0, estimated = 0 + 30 = 30
	if result.EstimatedSeconds != 30 {
		t.Errorf("expected 30s RTO with no replicas, got %d", result.EstimatedSeconds)
	}
	if result.Threshold != domain.RTOGreen {
		t.Errorf("expected green for 30s RTO, got %s", result.Threshold)
	}
}

func TestComputeRTO_ZeroRedoQueue(t *testing.T) {
	replicas := []domain.ReplicaHealthRow{
		{RoleDesc: "SECONDARY", RedoQueueKB: 0, RedoRateKBPS: 100},
	}
	result := domain.ComputeRTO(replicas)
	if result.RedoSeconds != 0 {
		t.Errorf("expected 0 redo seconds for empty queue, got %d", result.RedoSeconds)
	}
	if result.EstimatedSeconds != 30 {
		t.Errorf("expected 30s total RTO, got %d", result.EstimatedSeconds)
	}
}

func TestComputeRTO_LargeRedoQueue_YellowThreshold(t *testing.T) {
	// 50000 KB / 1000 KB/s = 50s redo, 50+30 = 80s total → yellow
	replicas := []domain.ReplicaHealthRow{
		{RoleDesc: "SECONDARY", RedoQueueKB: 50000, RedoRateKBPS: 1000},
	}
	result := domain.ComputeRTO(replicas)
	if result.RedoSeconds != 50 {
		t.Errorf("expected 50s redo time, got %d", result.RedoSeconds)
	}
	if result.EstimatedSeconds != 80 {
		t.Errorf("expected 80s total RTO, got %d", result.EstimatedSeconds)
	}
	if result.Threshold != domain.RTOYellow {
		t.Errorf("expected yellow for 80s, got %s", result.Threshold)
	}
}

func TestComputeRTO_ZeroRedoRate_UsesFallback(t *testing.T) {
	// redo_rate=0 should not panic; fallback rate 100 KB/s used
	replicas := []domain.ReplicaHealthRow{
		{RoleDesc: "SECONDARY", RedoQueueKB: 10000, RedoRateKBPS: 0},
	}
	result := domain.ComputeRTO(replicas)
	// 10000 / 100 = 100s redo + 30 = 130s → red
	if result.EstimatedSeconds != 130 {
		t.Errorf("expected 130s estimated RTO, got %d", result.EstimatedSeconds)
	}
	if result.Threshold != domain.RTORed {
		t.Errorf("expected red for 130s, got %s", result.Threshold)
	}
}

func TestComputeRTO_RedThreshold(t *testing.T) {
	// Very large redo queue → well over 120s
	replicas := []domain.ReplicaHealthRow{
		{RoleDesc: "SECONDARY", RedoQueueKB: 200000, RedoRateKBPS: 1000},
	}
	result := domain.ComputeRTO(replicas)
	if result.Threshold != domain.RTORed {
		t.Errorf("expected red threshold for large RTO, got %s", result.Threshold)
	}
}

// ---------------------------------------------------------------------------
// Failover Readiness tests
// ---------------------------------------------------------------------------

func TestFailoverReadiness_AllHealthy_ReturnsReady(t *testing.T) {
	replicas := []domain.ReplicaHealthRow{
		{
			RoleDesc:                 "SECONDARY",
			ReplicaServerName:        "DR1",
			SynchronizationHealth:    "HEALTHY",
			ConnectedStateDesc:       "CONNECTED",
			SecondaryLagSeconds:      2,
			IsFailoverReady:          true,
		},
	}
	dbStates := []domain.DatabaseSyncState{
		{
			DatabaseName:             "AppDB",
			ReplicaServerName:        "DR1",
			SynchronizationStateDesc: "SYNCHRONIZED",
			IsSuspended:              false,
		},
	}
	result := domain.EvaluateFailoverReadiness(replicas, dbStates, 0, "")
	if !result.Ready {
		t.Error("expected failover ready = true for all healthy replica")
	}
	for _, c := range result.Checks {
		if c.Status != domain.ReadinessPass {
			t.Errorf("check '%s' should pass, got %s: %s", c.Name, c.Status, c.Detail)
		}
	}
}

func TestFailoverReadiness_DisconnectedReplica_NotReady(t *testing.T) {
	replicas := []domain.ReplicaHealthRow{
		{
			RoleDesc:              "SECONDARY",
			ReplicaServerName:     "DR1",
			SynchronizationHealth: "HEALTHY",
			ConnectedStateDesc:    "DISCONNECTED",
			SecondaryLagSeconds:   5,
		},
	}
	result := domain.EvaluateFailoverReadiness(replicas, nil, 0, "")
	if result.Ready {
		t.Error("expected failover ready = false for disconnected replica")
	}
	found := false
	for _, c := range result.Checks {
		if c.Name == "Replicas Connected" && c.Status == domain.ReadinessFail {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Replicas Connected' check to fail")
	}
}

func TestFailoverReadiness_SuspendedDatabase_NotReady(t *testing.T) {
	replicas := []domain.ReplicaHealthRow{
		{
			RoleDesc:              "SECONDARY",
			ReplicaServerName:     "DR1",
			SynchronizationHealth: "HEALTHY",
			ConnectedStateDesc:    "CONNECTED",
			SecondaryLagSeconds:   2,
		},
	}
	dbStates := []domain.DatabaseSyncState{
		{DatabaseName: "AppDB", ReplicaServerName: "DR1", IsSuspended: true},
	}
	result := domain.EvaluateFailoverReadiness(replicas, dbStates, 0, "")
	if result.Ready {
		t.Error("expected failover ready = false for suspended database")
	}
	found := false
	for _, c := range result.Checks {
		if c.Name == "No Suspended Databases" && c.Status == domain.ReadinessFail {
			found = true
		}
	}
	if !found {
		t.Error("expected 'No Suspended Databases' check to fail")
	}
}

func TestFailoverReadiness_HighLag_NotReady(t *testing.T) {
	replicas := []domain.ReplicaHealthRow{
		{
			RoleDesc:              "SECONDARY",
			ReplicaServerName:     "DR1",
			SynchronizationHealth: "HEALTHY",
			ConnectedStateDesc:    "CONNECTED",
			SecondaryLagSeconds:   120, // > 30s threshold
		},
	}
	result := domain.EvaluateFailoverReadiness(replicas, nil, 0, "")
	if result.Ready {
		t.Error("expected failover ready = false for high lag")
	}
	found := false
	for _, c := range result.Checks {
		if c.Status == domain.ReadinessFail && c.Name != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected at least one failed check")
	}
}

func TestFailoverReadiness_NotSynchronizedDB_NotReady(t *testing.T) {
	replicas := []domain.ReplicaHealthRow{
		{
			RoleDesc:              "SECONDARY",
			ReplicaServerName:     "DR1",
			SynchronizationHealth: "PARTIALLY_HEALTHY",
			ConnectedStateDesc:    "CONNECTED",
			SecondaryLagSeconds:   0,
		},
	}
	dbStates := []domain.DatabaseSyncState{
		{
			DatabaseName:             "CriticalDB",
			ReplicaServerName:        "DR1",
			SynchronizationStateDesc: "NOT SYNCHRONIZING",
			IsSuspended:              false,
		},
	}
	result := domain.EvaluateFailoverReadiness(replicas, dbStates, 0, "")
	if result.Ready {
		t.Error("expected failover not ready for unsynchronised DB")
	}
}

// ---------------------------------------------------------------------------
// FeatureDetection.ShouldShowPage tests
// ---------------------------------------------------------------------------

func TestShouldShowPage_BothFalse_ReturnsFalse(t *testing.T) {
	f := domain.FeatureDetection{HAEnabled: false, ReplicationEnabled: false}
	if f.ShouldShowPage() {
		t.Error("expected ShouldShowPage false when both disabled")
	}
}

func TestShouldShowPage_HAOnly_ReturnsTrue(t *testing.T) {
	f := domain.FeatureDetection{HAEnabled: true, ReplicationEnabled: false}
	if !f.ShouldShowPage() {
		t.Error("expected ShouldShowPage true when HA enabled")
	}
}

func TestShouldShowPage_ReplicationOnly_ReturnsTrue(t *testing.T) {
	f := domain.FeatureDetection{HAEnabled: false, ReplicationEnabled: true}
	if !f.ShouldShowPage() {
		t.Error("expected ShouldShowPage true when replication enabled")
	}
}

func TestShouldShowPage_BothEnabled_ReturnsTrue(t *testing.T) {
	f := domain.FeatureDetection{HAEnabled: true, ReplicationEnabled: true}
	if !f.ShouldShowPage() {
		t.Error("expected ShouldShowPage true when both enabled")
	}
}
