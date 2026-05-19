// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : Failover readiness checklist evaluation.
//           Pure function — takes replica and database state snapshots and
//           returns a structured checklist used by DBAs during incidents and
//           planned maintenance windows.
//
// Checks performed (in order displayed on the dashboard):
//   1. All DBs Synchronized   — no SECONDARY DB in NOT_SYNCHRONIZING state
//   2. No Suspended Replicas  — no replica with is_suspended on its databases
//   3. Replicas Connected     — all SECONDARYs show CONNECTED
//   4. All Replicas Healthy   — synchronization_health == HEALTHY for all
//   5. Lag Acceptable         — no secondary_lag_seconds > 30 s
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package domain

import "fmt"

const lagThresholdSec int64 = 30

// EvaluateFailoverReadiness builds the checklist from live replica and
// database state snapshots.  Returns FailoverReadiness{Ready: true} only
// when every check passes.
func EvaluateFailoverReadiness(replicas []ReplicaHealthRow, dbStates []DatabaseSyncState, longRunningCount int, quorumState string) FailoverReadiness {
	checks := []FailoverReadinessCheck{
		checkAllDBsSynchronized(dbStates),
		checkNoSuspendedDatabases(dbStates),
		checkReplicasConnected(replicas),
		checkReplicasHealthy(replicas),
		checkLagAcceptable(replicas),
		checkNoLongRunningTransactions(longRunningCount),
		checkQuorumHealthy(quorumState),
	}

	allPass := true
	for _, c := range checks {
		if c.Status != ReadinessPass {
			allPass = false
			break
		}
	}

	return FailoverReadiness{
		Ready:                       allPass,
		Checks:                      checks,
		LongRunningTransactionsCount: longRunningCount,
		QuorumStateDesc:              quorumState,
	}
}

// checkNoLongRunningTransactions verifies that no long-running transactions are active
// on the primary, as they can delay failover.
func checkNoLongRunningTransactions(count int) FailoverReadinessCheck {
	if count > 0 {
		return FailoverReadinessCheck{
			Name:   "No Long-Running Transactions",
			Status: ReadinessFail,
			Detail: fmt.Sprintf("%d transactions running > 60s", count),
		}
	}
	return FailoverReadinessCheck{Name: "No Long-Running Transactions", Status: ReadinessPass}
}

// checkQuorumHealthy verifies that the cluster has normal quorum.
func checkQuorumHealthy(state string) FailoverReadinessCheck {
	if state != "" && state != "NORMAL_QUORUM" {
		return FailoverReadinessCheck{
			Name:   "Quorum Healthy",
			Status: ReadinessFail,
			Detail: fmt.Sprintf("Quorum state: %s", state),
		}
	}
	return FailoverReadinessCheck{Name: "Quorum Healthy", Status: ReadinessPass}
}

// checkAllDBsSynchronized verifies that no secondary database is in the
// NOT SYNCHRONIZING state.
func checkAllDBsSynchronized(dbStates []DatabaseSyncState) FailoverReadinessCheck {
	for _, d := range dbStates {
		if d.SynchronizationStateDesc == "NOT SYNCHRONIZING" {
			return FailoverReadinessCheck{
				Name:   "All DBs Synchronized",
				Status: ReadinessFail,
				Detail: fmt.Sprintf("%s on %s is NOT SYNCHRONIZING", d.DatabaseName, d.ReplicaServerName),
			}
		}
	}
	return FailoverReadinessCheck{Name: "All DBs Synchronized", Status: ReadinessPass}
}

// checkNoSuspendedDatabases verifies no database replica is suspended.
func checkNoSuspendedDatabases(dbStates []DatabaseSyncState) FailoverReadinessCheck {
	for _, d := range dbStates {
		if d.IsSuspended {
			return FailoverReadinessCheck{
				Name:   "No Suspended Databases",
				Status: ReadinessFail,
				Detail: fmt.Sprintf("%s on %s is suspended", d.DatabaseName, d.ReplicaServerName),
			}
		}
	}
	return FailoverReadinessCheck{Name: "No Suspended Databases", Status: ReadinessPass}
}

// checkReplicasConnected verifies all secondary replicas report CONNECTED.
func checkReplicasConnected(replicas []ReplicaHealthRow) FailoverReadinessCheck {
	for _, r := range replicas {
		if r.RoleDesc == "SECONDARY" && !r.IsConnected() {
			return FailoverReadinessCheck{
				Name:   "Replicas Connected",
				Status: ReadinessFail,
				Detail: fmt.Sprintf("%s is %s", r.ReplicaServerName, r.ConnectedStateDesc),
			}
		}
	}
	return FailoverReadinessCheck{Name: "Replicas Connected", Status: ReadinessPass}
}

// checkReplicasHealthy verifies all replicas are HEALTHY.
func checkReplicasHealthy(replicas []ReplicaHealthRow) FailoverReadinessCheck {
	for _, r := range replicas {
		if r.RoleDesc == "SECONDARY" && !r.IsHealthy() {
			return FailoverReadinessCheck{
				Name:   "All Replicas Healthy",
				Status: ReadinessFail,
				Detail: fmt.Sprintf("%s health: %s", r.ReplicaServerName, r.SynchronizationHealth),
			}
		}
	}
	return FailoverReadinessCheck{Name: "All Replicas Healthy", Status: ReadinessPass}
}

// checkLagAcceptable verifies no secondary has secondary_lag_seconds > 30 s.
func checkLagAcceptable(replicas []ReplicaHealthRow) FailoverReadinessCheck {
	for _, r := range replicas {
		if r.RoleDesc == "SECONDARY" && r.SecondaryLagSeconds > lagThresholdSec {
			return FailoverReadinessCheck{
				Name:   fmt.Sprintf("Lag Acceptable (< %ds)", lagThresholdSec),
				Status: ReadinessFail,
				Detail: fmt.Sprintf("%s lag: %ds", r.ReplicaServerName, r.SecondaryLagSeconds),
			}
		}
	}
	return FailoverReadinessCheck{
		Name:   fmt.Sprintf("Lag Acceptable (< %ds)", lagThresholdSec),
		Status: ReadinessPass,
	}
}
