// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server AlwaysOn Availability Group health statistics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

type AGHealthStats struct {
	AGName               string
	ReplicaServerName    string
	DatabaseName         string
	ReplicaRole          string
	SynchronizationState string
	SyncStateDesc        string
	IsPrimaryReplica     bool
	LogSendQueueKB       int64
	RedoQueueKB          int64
	LogSendRateKB        int64
	RedoRateKB           int64
	LastSentTime         sql.NullTime
	LastReceivedTime     sql.NullTime
	LastHardenedTime     sql.NullTime
	LastRedoneTime       sql.NullTime
	SecondaryLagSecs     int64
}

func (c *SqlServerRepository) FetchAGHealthStats(instanceName string) ([]AGHealthStats, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	checkQuery := `SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.availability_groups`
	var agCount int
	if err := db.QueryRow(checkQuery).Scan(&agCount); err != nil {
		log.Printf("[SQLSERVER] FetchAGHealthStats AG metadata check failed for %s: %v (likely not configured or no permissions to sys.availability_groups)", instanceName, err)
		return []AGHealthStats{}, nil
	}

	if agCount == 0 {
		return []AGHealthStats{}, nil
	}

	hasDbStates := true
	var dbStatesCheck int
	if err := db.QueryRow(`SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.all_objects WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_database_states')`).Scan(&dbStatesCheck); err != nil || dbStatesCheck == 0 {
		if err != nil {
			log.Printf("[SQLSERVER] FetchAGHealthStats: dm_hadr_availability_database_states check failed for %s: %v", instanceName, err)
		} else {
			log.Printf("[SQLSERVER] FetchAGHealthStats: dm_hadr_availability_database_states not available for %s", instanceName)
		}
		hasDbStates = false
	}

	hasSecondaryLag := true
	var lagCheck int
	if err := db.QueryRow(`SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_replica_states') AND name = 'secondary_lag_seconds'`).Scan(&lagCheck); err != nil || lagCheck == 0 {
		log.Printf("[SQLSERVER] FetchAGHealthStats: secondary_lag_seconds not available for %s (pre-2016 SP1)", instanceName)
		hasSecondaryLag = false
	}

	hasLastRedoneTime := true
	var redoCheck int
	if err := db.QueryRow(`SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_replica_states') AND name = 'last_redone_time'`).Scan(&redoCheck); err != nil || redoCheck == 0 {
		log.Printf("[SQLSERVER] FetchAGHealthStats: last_redone_time not available for %s (pre-2016)", instanceName)
		hasLastRedoneTime = false
	}

	hasLastHardenedTime := true
	var hardenedCheck int
	if err := db.QueryRow(`SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_replica_states') AND name = 'last_hardened_time'`).Scan(&hardenedCheck); err != nil || hardenedCheck == 0 {
		log.Printf("[SQLSERVER] FetchAGHealthStats: last_hardened_time not available for %s (pre-2016 SP1 CU6)", instanceName)
		hasLastHardenedTime = false
	}

	hasLogSendRate := true
	var logRateCheck int
	if err := db.QueryRow(`SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_replica_states') AND name = 'log_send_rate'`).Scan(&logRateCheck); err != nil || logRateCheck == 0 {
		log.Printf("[SQLSERVER] FetchAGHealthStats: log_send_rate not available for %s (pre-2016)", instanceName)
		hasLogSendRate = false
	}

	hasUndoRate := true
	var undoRateCheck int
	if err := db.QueryRow(`SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_replica_states') AND name = 'undo_rate'`).Scan(&undoRateCheck); err != nil || undoRateCheck == 0 {
		log.Printf("[SQLSERVER] FetchAGHealthStats: undo_rate not available for %s (pre-2016)", instanceName)
		hasUndoRate = false
	}

	hasUndoQueueSize := true
	var undoQueueCheck int
	if err := db.QueryRow(`SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_replica_states') AND name = 'undo_queue_size'`).Scan(&undoQueueCheck); err != nil || undoQueueCheck == 0 {
		log.Printf("[SQLSERVER] FetchAGHealthStats: undo_queue_size not available for %s (pre-2016)", instanceName)
		hasUndoQueueSize = false
	}

	var query string
	if !hasSecondaryLag || !hasLastRedoneTime || !hasLastHardenedTime || !hasLogSendRate || !hasUndoRate || !hasUndoQueueSize {
		log.Printf("[SQLSERVER] FetchAGHealthStats: Using minimal fallback query for %s (missing columns detected)", instanceName)
		query = `
			SELECT /* SQL_OPTIMA */   
				ag.name AS ag_name,
				ar.replica_server_name,
				'N/A' AS database_name,
				'UNKNOWN' AS replica_role,
				0 AS synchronization_state,
				'UNKNOWN' AS synchronization_state_desc,
				0 AS is_primary_replica,
				CAST(0 AS BIGINT) AS log_send_queue_kb,
				CAST(0 AS BIGINT) AS redo_queue_kb,
				CAST(0 AS BIGINT) AS log_send_rate_kb,
				CAST(0 AS BIGINT) AS redo_rate_kb,
				CAST(NULL AS DATETIME) AS last_sent_time,
				CAST(NULL AS DATETIME) AS last_received_time,
				CAST(NULL AS DATETIME) AS last_hardened_time,
				CAST(NULL AS DATETIME) AS last_redone_time,
				CAST(0 AS BIGINT) AS secondary_lag_seconds
			FROM sys.availability_groups ag
			INNER JOIN sys.availability_replicas ar ON ag.group_id = ar.group_id
			ORDER BY ag.name, ar.replica_server_name
		`
	} else if hasDbStates {
		query = `
			SELECT /* SQL_OPTIMA */   
				ag.name AS ag_name,
				ar.replica_server_name,
				COALESCE(DB_NAME(dbs.database_id), 'N/A') AS database_name,
				ISNULL(rs.role_desc, 'UNKNOWN') AS replica_role,
				COALESCE(drs.synchronization_state, 0) AS synchronization_state,
				COALESCE(drs.synchronization_state_desc, 'UNKNOWN') AS synchronization_state_desc,
				CASE WHEN rs.role_desc = 'PRIMARY' THEN 1 ELSE 0 END AS is_primary_replica,
				ISNULL(drs.log_send_queue_size, 0) / 1024 AS log_send_queue_kb,
				ISNULL(drs.undo_queue_size, 0) / 1024 AS redo_queue_kb,
				ISNULL(drs.log_send_rate, 0) / 1024 AS log_send_rate_kb,
				ISNULL(drs.undo_rate, 0) / 1024 AS redo_rate_kb,
				drs.last_sent_time,
				drs.last_received_time,
				drs.last_hardened_time,
				drs.last_redone_time,
				ISNULL(drs.secondary_lag_seconds, 0) AS secondary_lag_seconds
			FROM sys.availability_groups ag
			INNER JOIN sys.availability_replicas ar ON ag.group_id = ar.group_id
			LEFT JOIN sys.dm_hadr_availability_group_states rs ON ag.group_id = rs.group_id
			LEFT JOIN sys.dm_hadr_availability_replica_states drs ON ar.replica_id = drs.replica_id
			LEFT JOIN sys.dm_hadr_availability_database_states dbs ON ar.replica_id = dbs.replica_id AND dbs.database_id IS NOT NULL
			ORDER BY ag.name, ar.replica_server_name
		`
	} else {
		query = `
			SELECT /* SQL_OPTIMA */   
				ag.name AS ag_name,
				ar.replica_server_name,
				'N/A' AS database_name,
				ISNULL(rs.role_desc, 'UNKNOWN') AS replica_role,
				COALESCE(drs.synchronization_state, 0) AS synchronization_state,
				COALESCE(drs.synchronization_state_desc, 'UNKNOWN') AS synchronization_state_desc,
				CASE WHEN rs.role_desc = 'PRIMARY' THEN 1 ELSE 0 END AS is_primary_replica,
				ISNULL(drs.log_send_queue_size, 0) / 1024 AS log_send_queue_kb,
				ISNULL(drs.undo_queue_size, 0) / 1024 AS redo_queue_kb,
				ISNULL(drs.log_send_rate, 0) / 1024 AS log_send_rate_kb,
				ISNULL(drs.undo_rate, 0) / 1024 AS redo_rate_kb,
				drs.last_sent_time,
				drs.last_received_time,
				drs.last_hardened_time,
				drs.last_redone_time,
				ISNULL(drs.secondary_lag_seconds, 0) AS secondary_lag_seconds
			FROM sys.availability_groups ag
			INNER JOIN sys.availability_replicas ar ON ag.group_id = ar.group_id
			LEFT JOIN sys.dm_hadr_availability_group_states rs ON ag.group_id = rs.group_id
			LEFT JOIN sys.dm_hadr_availability_replica_states drs ON ar.replica_id = drs.replica_id
			ORDER BY ag.name, ar.replica_server_name
		`
	}

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[SQLSERVER] FetchAGHealthStats Error for %s: %v", instanceName, err)
		if strings.Contains(err.Error(), "Invalid column name") {
			log.Printf("[SQLSERVER] FetchAGHealthStats: Retrying with ultra-minimal fallback query for %s", instanceName)
			query = `
				SELECT /* SQL_OPTIMA */   
					ag.name AS ag_name,
					ar.replica_server_name,
					'N/A' AS database_name,
					'UNKNOWN' AS replica_role,
					0 AS synchronization_state,
					'UNKNOWN' AS synchronization_state_desc,
					0 AS is_primary_replica,
					CAST(0 AS BIGINT) AS log_send_queue_kb,
					CAST(0 AS BIGINT) AS redo_queue_kb,
					CAST(0 AS BIGINT) AS log_send_rate_kb,
					CAST(0 AS BIGINT) AS redo_rate_kb,
					CAST(NULL AS DATETIME) AS last_sent_time,
					CAST(NULL AS DATETIME) AS last_received_time,
					CAST(NULL AS DATETIME) AS last_hardened_time,
					CAST(NULL AS DATETIME) AS last_redone_time,
					CAST(0 AS BIGINT) AS secondary_lag_seconds
				FROM sys.availability_groups ag
				INNER JOIN sys.availability_replicas ar ON ag.group_id = ar.group_id
				ORDER BY ag.name, ar.replica_server_name
			`
			rows, err = db.Query(query)
			if err != nil {
				log.Printf("[SQLSERVER] FetchAGHealthStats Fallback Error for %s: %v", instanceName, err)
				return []AGHealthStats{}, nil
			}
		} else {
			return nil, err
		}
	}
	defer rows.Close()

	var results []AGHealthStats
	for rows.Next() {
		var s AGHealthStats
		var dbName sql.NullString
		var roleDesc, syncState, syncStateDesc sql.NullString
		var isPrimary int
		var logSendQueue, redoQueue, logSendRate, redoRate sql.NullInt64
		var secondaryLag int64

		if err := rows.Scan(&s.AGName, &s.ReplicaServerName, &dbName, &roleDesc, &syncState, &syncStateDesc,
			&isPrimary, &logSendQueue, &redoQueue, &logSendRate, &redoRate,
			&s.LastSentTime, &s.LastReceivedTime, &s.LastHardenedTime, &s.LastRedoneTime, &secondaryLag); err != nil {
			log.Printf("[SQLSERVER] FetchAGHealthStats Scan Error: %v", err)
			continue
		}

		if dbName.Valid {
			s.DatabaseName = dbName.String
		}
		if roleDesc.Valid {
			s.ReplicaRole = roleDesc.String
		}
		if syncState.Valid {
			s.SynchronizationState = syncState.String
		}
		if syncStateDesc.Valid {
			s.SyncStateDesc = syncStateDesc.String
		}
		s.IsPrimaryReplica = isPrimary == 1
		if logSendQueue.Valid {
			s.LogSendQueueKB = logSendQueue.Int64
		}
		if redoQueue.Valid {
			s.RedoQueueKB = redoQueue.Int64
		}
		if logSendRate.Valid {
			s.LogSendRateKB = logSendRate.Int64
		}
		if redoRate.Valid {
			s.RedoRateKB = redoRate.Int64
		}
		s.SecondaryLagSecs = secondaryLag

		results = append(results, s)
	}

	return results, rows.Err()
}
