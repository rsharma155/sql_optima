// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server AlwaysOn Availability Group health statistics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"log/slog"
	"context"
	"database/sql"
)

type AGHealthStats struct {
	AGName               string       `json:"ag_name"`
	ReplicaServerName    string       `json:"replica_server_name"`
	DatabaseName         string       `json:"database_name"`
	ReplicaRole          string       `json:"replica_role"`
	OperationalState     string       `json:"operational_state"`
	ConnectedState       string       `json:"connected_state"`
	SynchronizationState string       `json:"synchronization_state"`
	SyncStateDesc        string       `json:"sync_state_desc"`
	IsPrimaryReplica     bool         `json:"is_primary_replica"`
	LogSendQueueKB       int64        `json:"log_send_queue_kb"`
	RedoQueueKB          int64        `json:"redo_queue_kb"`
	LogSendRateKB        int64        `json:"log_send_rate_kb"`
	RedoRateKB           int64        `json:"redo_rate_kb"`
	LastSentTime         sql.NullTime `json:"last_sent_time"`
	LastReceivedTime     sql.NullTime `json:"last_received_time"`
	LastHardenedTime     sql.NullTime `json:"last_hardened_time"`
	LastRedoneTime       sql.NullTime `json:"last_redone_time"`
	SecondaryLagSecs     int64        `json:"secondary_lag_secs"`
}

func (c *SqlServerRepository) FetchAGClusterStatus(ctx context.Context, instanceName string) (map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return map[string]interface{}{"cluster_name": nil}, nil
	}

	query := `/* SQL_OPTIMA */ SELECT cluster_name, quorum_type_desc, quorum_state_desc FROM sys.dm_hadr_cluster`
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	row := db.QueryRowContext(ctx, query)

	var clusterName, quorumType, quorumState string
	if err := row.Scan(&clusterName, &quorumType, &quorumState); err != nil {
		return map[string]interface{}{"cluster_name": nil}, nil
	}

	return map[string]interface{}{
		"cluster_name":      clusterName,
		"quorum_type_desc":  quorumType,
		"quorum_state_desc": quorumState,
	}, nil
}

type AGClusterMember struct {
	MemberName          string `json:"member_name"`
	MemberType          string `json:"member_type"`
	MemberState         string `json:"member_state"`
	NumberOfQuorumVotes int    `json:"number_of_quorum_votes"`
}

func (c *SqlServerRepository) FetchAGClusterMembers(ctx context.Context, instanceName string) ([]AGClusterMember, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return []AGClusterMember{}, nil
	}

	query := `/* SQL_OPTIMA */ SELECT member_name, member_type_desc, member_state_desc, number_of_quorum_votes FROM sys.dm_hadr_cluster_members`
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return []AGClusterMember{}, nil
	}
	defer rows.Close()

	var members []AGClusterMember
	for rows.Next() {
		var m AGClusterMember
		if err := rows.Scan(&m.MemberName, &m.MemberType, &m.MemberState, &m.NumberOfQuorumVotes); err != nil {
			continue
		}
		members = append(members, m)
	}
	return members, nil
}

func (c *SqlServerRepository) FetchAGHealthStats(ctx context.Context, instanceName string) ([]AGHealthStats, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return []AGHealthStats{}, nil
	}

	checkQuery := `SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.availability_groups`
	var agCount int
	ctx, cancel := WithQueryTimeout(ctx, 0)
	if err := db.QueryRowContext(ctx, checkQuery).Scan(&agCount); err != nil {
	cancel()
		slog.Error("[SQLSERVER] FetchAGHealthStats AG metadata check failed", "target", instanceName, "err", err)
		return []AGHealthStats{}, nil
	}

	if agCount == 0 {
		return []AGHealthStats{}, nil
	}

	query := `
		/* SQL_OPTIMA */
		SELECT    
			ag.name AS ag_name,
			ar.replica_server_name,
			ISNULL(DB_NAME(drs.database_id), 'N/A') AS database_name,
			ISNULL(ars.role_desc, 'UNKNOWN') AS replica_role,
			ISNULL(ars.operational_state_desc, 'UNKNOWN') AS operational_state,
			ISNULL(ars.connected_state_desc, 'UNKNOWN') AS connected_state,
			ISNULL(drs.synchronization_state, 0) AS synchronization_state,
			ISNULL(drs.synchronization_state_desc, 'UNKNOWN') AS synchronization_state_desc,
			CASE WHEN ars.role_desc = 'PRIMARY' THEN 1 ELSE 0 END AS is_primary_replica,
			ISNULL(drs.log_send_queue_size, 0) AS log_send_queue_kb,
			ISNULL(drs.redo_queue_size, 0) AS redo_queue_kb,
			ISNULL(drs.log_send_rate, 0) AS log_send_rate_kb,
			ISNULL(drs.redo_rate, 0) AS redo_rate_kb,
			drs.last_sent_time,
			drs.last_received_time,
			drs.last_hardened_time,
			drs.last_redone_time,
			ISNULL(drs.secondary_lag_seconds, 0) AS secondary_lag_seconds
		FROM sys.availability_groups ag
		INNER JOIN sys.availability_replicas ar ON ag.group_id = ar.group_id
		LEFT JOIN sys.dm_hadr_availability_replica_states ars ON ar.replica_id = ars.replica_id
		LEFT JOIN sys.dm_hadr_database_replica_states drs ON ar.replica_id = drs.replica_id
		ORDER BY ag.name, ar.replica_server_name, database_name
	`

	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		slog.Error("[SQLSERVER] FetchAGHealthStats Error", "target", instanceName, "err", err)
		// Fallback to replica-level only if database-level fails
		query = `
			/* SQL_OPTIMA */
			SELECT    
				ag.name AS ag_name,
				ar.replica_server_name,
				'N/A' AS database_name,
				ISNULL(ars.role_desc, 'UNKNOWN') AS replica_role,
				ISNULL(ars.operational_state_desc, 'UNKNOWN') AS operational_state,
				ISNULL(ars.connected_state_desc, 'UNKNOWN') AS connected_state,
				ISNULL(ars.operational_state, 0) AS synchronization_state,
				ISNULL(ars.operational_state_desc, 'UNKNOWN') AS synchronization_state_desc,
				CASE WHEN ars.role_desc = 'PRIMARY' THEN 1 ELSE 0 END AS is_primary_replica,
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
			LEFT JOIN sys.dm_hadr_availability_replica_states ars ON ar.replica_id = ars.replica_id
			ORDER BY ag.name, ar.replica_server_name
		`
		ctx, cancel = WithQueryTimeout(ctx, 0)
		defer cancel()
		rows, err = db.QueryContext(ctx, query)
		if err != nil {
			slog.Error("[SQLSERVER] FetchAGHealthStats Fallback Error", "target", instanceName, "err", err)
			return []AGHealthStats{}, nil
		}
	}
	defer rows.Close()

	var results []AGHealthStats
	for rows.Next() {
		var s AGHealthStats
		var dbName sql.NullString
		var roleDesc, opState, connState, syncState, syncStateDesc sql.NullString
		var isPrimary int
		var logSendQueue, redoQueue, logSendRate, redoRate sql.NullInt64
		var secondaryLag int64

		if err := rows.Scan(&s.AGName, &s.ReplicaServerName, &dbName, &roleDesc, &opState, &connState, &syncState, &syncStateDesc,
			&isPrimary, &logSendQueue, &redoQueue, &logSendRate, &redoRate,
			&s.LastSentTime, &s.LastReceivedTime, &s.LastHardenedTime, &s.LastRedoneTime, &secondaryLag); err != nil {
			slog.Error("[SQLSERVER] FetchAGHealthStats Scan Error", "err", err)
			continue
		}

		if dbName.Valid {
			s.DatabaseName = dbName.String
		}
		if roleDesc.Valid {
			s.ReplicaRole = roleDesc.String
		}
		if opState.Valid {
			s.OperationalState = opState.String
		}
		if connState.Valid {
			s.ConnectedState = connState.String
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
