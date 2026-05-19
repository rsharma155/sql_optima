// Package domain defines value objects for the wait-stats collection pipeline.
// These types travel from the SQL Server DMV → collector → TimescaleDB writer.
// Metadata:
//   - Feature: SQL Server Wait Stats V2
//   - Layer: Domain
package domain

import "time"

// WaitStatsCumulativeSnapshot represents one row from sys.dm_os_wait_stats.
// All counters are cumulative since last SQL Server restart.
type WaitStatsCumulativeSnapshot struct {
	CaptureTimestamp   time.Time
	WaitType           string
	WaitingTasksCount  int64
	WaitTimeMs         int64
	SignalWaitTimeMs   int64
	ResourceWaitTimeMs int64 // derived: WaitTimeMs - SignalWaitTimeMs
}

// WaitStatsDeltaRow is the computed interval delta for one wait type.
// Inserted into sqlserver_wait_stats_delta by the Go collector.
// restart_detected=true when a negative delta was clamped to 0.
type WaitStatsDeltaRow struct {
	CaptureTimestamp    time.Time
	WaitType            string
	WaitCategory        string
	DeltaWaitMs         int64
	DeltaSignalWaitMs   int64
	DeltaResourceWaitMs int64
	DeltaWaitingTasks   int64
	RestartDetected     bool
}

// ActiveWaitSession is one row from the enriched active-waits DMV query.
type ActiveWaitSession struct {
	CaptureTimestamp  time.Time
	SessionID         int
	WaitType          string
	WaitDurationMs    int64
	BlockingSessionID *int
	DatabaseName      string
	HostName          string
	ProgramName       string
	LoginName         string
	QueryText         string
}
