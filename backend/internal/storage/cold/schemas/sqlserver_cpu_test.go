// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/sqlserver_cpu_test.go
// Purpose: Comprehensive unit tests for all typed Parquet schemas, ensuring correct serialization and round-trip integrity.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

import (
	"os"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/assert"
)

func TestSQLServerCPUSchema(t *testing.T) {
	rows := []SQLServerCPURow{
		{
			CaptureTimestampMs:   123456789,
			ServerID:             "server1",
			ServerName:           "prod-sql-01",
			SQLCPUUtilization:    45.5,
			SystemCPUUtilization: 50.0,
			IdleCPU:              50.0,
			SchedulerCount:       8,
		},
	}

	tmpFile := "test_cpu.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerCPURow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	err = writer.Close()
	assert.NoError(t, err)
	f.Close()

	// Read back
	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerCPURow](f)
	readRows := make([]SQLServerCPURow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestSQLServerWaitSchema(t *testing.T) {
	rows := []SQLServerWaitRow{
		{
			CaptureTimestampMs:  123456789,
			ServerID:            "server1",
			DiskReadMsPerSec:    12.5,
			BlockingMsPerSec:    3.0,
			ParallelismMsPerSec: 8.0,
			OtherMsPerSec:       1.5,
		},
	}

	tmpFile := "test_wait.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerWaitRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerWaitRow](f)
	readRows := make([]SQLServerWaitRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestSQLServerMemorySchema(t *testing.T) {
	rows := []SQLServerMemoryRow{
		{
			CaptureTimestampMs:   123456789,
			ServerID:             "server1",
			TotalServerMemoryMB:  16384,
			TargetServerMemoryMB: 16384,
			SQLCacheMemoryMB:     2048,
			FreeMemoryMB:         4096,
			MemoryUtilizationPct: 75.0,
		},
	}

	tmpFile := "test_memory.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerMemoryRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerMemoryRow](f)
	readRows := make([]SQLServerMemoryRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestPGWaitEventSchema(t *testing.T) {
	rows := []PGWaitEventRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			WaitEventType:      "Lock",
			WaitEvent:          "relation",
			Count:              5,
			TotalWaitMs:        50.5,
		},
	}

	tmpFile := "test_pg_wait.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[PGWaitEventRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[PGWaitEventRow](f)
	readRows := make([]PGWaitEventRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestSQLServerMetricsSchema(t *testing.T) {
	rows := []SQLServerMetricsRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			AvgCPULoad:         10.5,
			MemoryUsage:        2048,
			ActiveUsers:        5,
			TotalLocks:         100,
			Deadlocks:          0,
			DataDiskMB:         10240,
			LogDiskMB:          1024,
			FreeDiskMB:         50000,
		},
	}

	tmpFile := "test_metrics.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerMetricsRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerMetricsRow](f)
	readRows := make([]SQLServerMetricsRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestSQLServerConnectionSchema(t *testing.T) {
	rows := []SQLServerConnectionRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			LoginName:          "sa",
			DatabaseName:       "master",
			ActiveConnections:  10,
			ActiveRequests:     2,
		},
	}

	tmpFile := "test_conn.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerConnectionRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerConnectionRow](f)
	readRows := make([]SQLServerConnectionRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestPGDBIOStatsSchema(t *testing.T) {
	rows := []PGDBIOStatsRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			DatabaseName:       "postgres",
			BlksRead:           100,
			BlksHit:            500,
			TempFiles:          2,
			TempBytes:          1024,
		},
	}

	tmpFile := "test_pg_io.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[PGDBIOStatsRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[PGDBIOStatsRow](f)
	readRows := make([]PGDBIOStatsRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestSQLServerMemoryMetricsSchema(t *testing.T) {
	rows := []SQLServerMemoryMetricsRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			SQLMemoryUsedMB:    8192,
			SQLMemoryTargetMB:  16384,
			OSTotalMemoryMB:    32768,
			OSAvailableMemoryMB: 8192,
			ProcessPhysicalLow: false,
			ProcessVirtualLow:  false,
			MemoryGrantsPending: 0,
			ActiveMemoryGrants:  5,
			WaitingMemoryGrants: 0,
			GrantedWorkspaceMB:  2048,
			RequestedWorkspaceMB: 1024,
			PLESeconds:          3000,
			PlanCacheMB:         512,
			SortWarningsTotal:   10,
			HashWarningsTotal:   5,
			SortWarningsPerSec:  0.1,
			HashWarningsPerSec:  0.05,
		},
	}

	tmpFile := "test_mem_metrics.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerMemoryMetricsRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerMemoryMetricsRow](f)
	readRows := make([]SQLServerMemoryMetricsRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestPGBasebackupHistorySchema(t *testing.T) {
	rows := []PGBasebackupHistoryRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			CheckpointTimeMs:   123456000,
			CheckpointWriteTime: 45.5,
		},
	}

	tmpFile := "test_pg_backup_hist.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[PGBasebackupHistoryRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[PGBasebackupHistoryRow](f)
	readRows := make([]PGBasebackupHistoryRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestPGFailedLoginSchema(t *testing.T) {
	rows := []PGFailedLoginRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			Username:           "attacker",
			ClientAddr:         "192.168.1.100",
			Message:            "password authentication failed",
		},
	}

	tmpFile := "test_pg_failed_login.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[PGFailedLoginRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[PGFailedLoginRow](f)
	readRows := make([]PGFailedLoginRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestSQLServerProcedureStatsSchema(t *testing.T) {
	rows := []SQLServerProcedureStatsRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			DatabaseName:       "AdventureWorks",
			SchemaName:         "dbo",
			ObjectName:         "uspGetEmployee",
			QueryHash:          12345,
			ExecutionCount:     100,
			TotalWorkerTimeMs:  500.5,
			TotalElapsedTimeMs: 1000.0,
			TotalLogicalReads:  5000,
			TotalPhysicalReads: 100,
		},
	}

	tmpFile := "test_proc_stats.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerProcedureStatsRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerProcedureStatsRow](f)
	readRows := make([]SQLServerProcedureStatsRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestPGQueryWaitProfileSchema(t *testing.T) {
	rows := []PGQueryWaitProfileRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			QueryID:            98765,
			Calls:              50,
			TotalExecTime:      200.5,
			MeanExecTime:       4.1,
			Rows:               1000,
			SharedBlksHit:      500,
			SharedBlksRead:     10,
			TempBlksWritten:    0,
			Query:              "SELECT * FROM users",
			Username:           "app_user",
		},
	}

	tmpFile := "test_pg_query_profile.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[PGQueryWaitProfileRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[PGQueryWaitProfileRow](f)
	readRows := make([]PGQueryWaitProfileRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestPGDDLActivitySchema(t *testing.T) {
	rows := []PGDDLActivityRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			SchemaName:         "public",
			RelName:            "orders",
			NTupIns:            10,
			NTupUpd:            5,
			NTupDel:            2,
		},
	}

	tmpFile := "test_pg_ddl.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[PGDDLActivityRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[PGDDLActivityRow](f)
	readRows := make([]PGDDLActivityRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestPGSessionActivitySchema(t *testing.T) {
	rows := []PGSessionActivityRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			DBName:             "postgres",
			PID:                1234,
			Username:           "app_user",
			AppName:            "sql_optima",
			ClientAddr:         "127.0.0.1",
			State:              "active",
			WaitEventType:      "CPU",
			WaitEvent:          "CPU",
			BackendType:        "client backend",
			QueryID:            555,
			Query:              "SELECT 1",
			XactStartMs:        123456000,
			QueryStartMs:       123456500,
			StateChangeMs:      123456700,
			BackendStartMs:     123450000,
		},
	}

	tmpFile := "test_pg_session.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[PGSessionActivityRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[PGSessionActivityRow](f)
	readRows := make([]PGSessionActivityRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestPGWaitEventSummarySchema(t *testing.T) {
	rows := []PGWaitEventSummaryRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			WaitEventType:      "IO",
			WaitEvent:          "DataFileRead",
			Sessions:           10,
			State:              "waiting",
		},
	}

	tmpFile := "test_pg_wait_summary.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[PGWaitEventSummaryRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[PGWaitEventSummaryRow](f)
	readRows := make([]PGWaitEventSummaryRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestPGDBLoadSchema(t *testing.T) {
	rows := []PGDBLoadRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			ActiveSessions:     20,
			CPUSessions:        5,
			WaitingSessions:    10,
			IOSessions:         3,
			LockSessions:       2,
			IdleInTxn:          1,
		},
	}

	tmpFile := "test_pg_load.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[PGDBLoadRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[PGDBLoadRow](f)
	readRows := make([]PGDBLoadRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestSQLServerJobMetricsSchema(t *testing.T) {
	rows := []SQLServerJobMetricsRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			TotalJobs:          50,
			EnabledJobs:         45,
			DisabledJobs:        5,
			RunningJobs:         2,
			FailedJobs24h:       1,
			CriticalJobsDisabled: 0,
			ErrorMessage:        "",
		},
	}

	tmpFile := "test_job_metrics.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerJobMetricsRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerJobMetricsRow](f)
	readRows := make([]SQLServerJobMetricsRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestSQLServerAGClusterSchema(t *testing.T) {
	rows := []SQLServerAGClusterRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			ClusterName:        "CLUS01",
			QuorumType:         "Node and Disk Majority",
			QuorumState:        "Normal Quorum",
			MembersJSON:        "{\"node1\": \"up\"}",
		},
	}

	tmpFile := "test_ag_cluster.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerAGClusterRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerAGClusterRow](f)
	readRows := make([]SQLServerAGClusterRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestSQLServerMemoryHistorySchema(t *testing.T) {
	rows := []SQLServerMemoryHistoryRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			PageLifeExpectancy: 3600.0,
		},
	}

	tmpFile := "test_mem_hist.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerMemoryHistoryRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerMemoryHistoryRow](f)
	readRows := make([]SQLServerMemoryHistoryRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestSQLServerQSYSnapshotSchema(t *testing.T) {
	rows := []SQLServerQSYSnapshotRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			DatabaseName:       "db1",
			QueryHash:          111,
			QueryText:          "SELECT *",
			PlanID:             222,
			RuntimeStatsID:     333,
			TotalExecutions:    1000,
			TotalCPUMs:         500.0,
			TotalDurationMs:    1000.0,
			TotalLogicalReads:  5000.0,
		},
	}

	tmpFile := "test_qsy_snap.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerQSYSnapshotRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerQSYSnapshotRow](f)
	readRows := make([]SQLServerQSYSnapshotRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestSQLServerQSYIntervalSchema(t *testing.T) {
	rows := []SQLServerQSYIntervalRow{
		{
			BucketStartMs:     123450000,
			BucketEndMs:       123456000,
			ServerID:          "server1",
			DatabaseName:      "db1",
			QueryHash:         111,
			QueryText:         "SELECT *",
			PlanID:            222,
			RuntimeStatsID:    333,
			DeltaExecutions:   10,
			DeltaCPUMs:        5.0,
			DeltaDurationMs:   10.0,
			DeltaLogicalReads: 50.0,
		},
	}

	tmpFile := "test_qsy_int.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerQSYIntervalRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerQSYIntervalRow](f)
	readRows := make([]SQLServerQSYIntervalRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestPGSSDeltaSchema(t *testing.T) {
	rows := []PGSSDeltaRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			QueryID:            101,
			DBName:             "postgres",
			Username:           "admin",
			AppName:            "app",
			QueryType:          "S",
			Calls:              100,
			TotalExecTime:      50.5,
			Rows:               500,
			SharedBlksHit:      1000,
			SharedBlksRead:     10,
			TempBlksWritten:    0,
			WALBytes:           1024.0,
			TotalPlanTime:      5.0,
			MeanExecTime:       0.5,
		},
	}

	tmpFile := "test_pgss_delta.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[PGSSDeltaRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[PGSSDeltaRow](f)
	readRows := make([]PGSSDeltaRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0], readRows[0])
}

func TestSQLServerSchedulerSchema(t *testing.T) {
	rows := []SQLServerSchedulerRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			SchedulerCount:     8,
			CPUCount:           8,
			ActiveWorkersCount: 100,
		},
	}

	tmpFile := "test_scheduler.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerSchedulerRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerSchedulerRow](f)
	readRows := make([]SQLServerSchedulerRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0].ServerID, readRows[0].ServerID)
}

func TestSQLServerMemoryGrantWaiterSchema(t *testing.T) {
	rows := []SQLServerMemoryGrantWaiterRow{
		{
			CaptureTimestampMs: 123456789,
			ServerID:           "server1",
			SessionID:          55,
			RequestedMemoryKB:  102400,
			WaitTimeMs:         5000,
			QueryText:          "SELECT * FROM big_table",
		},
	}

	tmpFile := "test_mem_wait.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[SQLServerMemoryGrantWaiterRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[SQLServerMemoryGrantWaiterRow](f)
	readRows := make([]SQLServerMemoryGrantWaiterRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0].ServerID, readRows[0].ServerID)
}

func TestCollectorRunSchema(t *testing.T) {
	rows := []CollectorRunRow{
		{
			RunID:              1,
			CollectorName:      "Pulse",
			ServerID:           "server1",
			CaptureTimestampMs: 123456789,
			EndTimeMs:          123456889,
			Status:             "success",
			RowsInserted:       10,
			DurationMs:         100,
		},
	}

	tmpFile := "test_coll_run.parquet"
	defer os.Remove(tmpFile)

	f, err := os.Create(tmpFile)
	assert.NoError(t, err)

	writer := parquet.NewGenericWriter[CollectorRunRow](f)
	_, err = writer.Write(rows)
	assert.NoError(t, err)
	writer.Close()
	f.Close()

	f, err = os.Open(tmpFile)
	assert.NoError(t, err)
	defer f.Close()

	reader := parquet.NewGenericReader[CollectorRunRow](f)
	readRows := make([]CollectorRunRow, 1)
	n, err := reader.Read(readRows)
	if err != nil && err.Error() != "EOF" {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, rows[0].CollectorName, readRows[0].CollectorName)
}
