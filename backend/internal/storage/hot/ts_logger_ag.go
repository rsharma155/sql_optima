// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Availability Group health metrics logger for SQL Server AG monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"log/slog"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (tl *TimescaleLogger) LogAGHealth(ctx context.Context, serverID uuid.UUID, agStats []AGHealthRow) error {
	if len(agStats) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO sqlserver_ag_health (
			capture_timestamp, server_id, ag_name, replica_server_name, database_name,
			replica_role, operational_state, connected_state, synchronization_state, synchronization_state_desc, is_primary_replica,
			log_send_queue_kb, redo_queue_kb, log_send_rate_kb, redo_rate_kb,
			last_sent_time, last_received_time, last_hardened_time, last_redone_time, secondary_lag_seconds
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`

	for _, r := range agStats {
		batch.Queue(query,
			r.CaptureTimestamp, serverID, r.AGName, r.ReplicaServerName, r.DatabaseName,
			r.ReplicaRole, r.OperationalState, r.ConnectedState, r.SynchronizationState, r.SyncStateDesc, r.IsPrimaryReplica,
			r.LogSendQueueKB, r.RedoQueueKB, r.LogSendRateKB, r.RedoRateKB,
			r.LastSentTime, r.LastReceivedTime, r.LastHardenedTime, r.LastRedoneTime, r.SecondaryLagSecs)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(agStats); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("AG health batch insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetAGHealthSummary(ctx context.Context, serverID uuid.UUID, from, to string, limit int) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRange(from, to)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT
			ag_name,
			replica_server_name,
			role_desc,
			COALESCE(connected_state_desc, 'UNKNOWN'),
			synchronization_state_desc,
			AVG(log_send_queue_kb)     AS avg_log_send_queue_kb,
			AVG(redo_queue_kb)         AS avg_redo_queue_kb,
			MAX(log_send_queue_kb)     AS max_log_send_queue_kb,
			MAX(redo_queue_kb)         AS max_redo_queue_kb,
			MAX(secondary_lag_seconds) AS max_secondary_lag_secs,
			COUNT(*)                   AS sample_count
		FROM monitor.sqlserver_ha_replica_state
		WHERE server_id = $1
		  AND capture_timestamp >= $2 AND capture_timestamp <= $3
		GROUP BY ag_name, replica_server_name, role_desc, connected_state_desc, synchronization_state_desc
		ORDER BY MAX(log_send_queue_kb) DESC, MAX(redo_queue_kb) DESC
		LIMIT $4
	`

	rows, err := tl.pool.Query(ctx, query, serverID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var agName, replicaServer, roleDesc, connState, syncState string
		var avgLogSend, avgRedo, maxLogSend, maxRedo, maxLagSecs float64
		var sampleCount int

		if err := rows.Scan(&agName, &replicaServer, &roleDesc, &connState, &syncState,
			&avgLogSend, &avgRedo, &maxLogSend, &maxRedo, &maxLagSecs, &sampleCount); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"ag_name":               agName,
			"replica_server_name":   replicaServer,
			"replica_role":          roleDesc,
			"connected_state":       connState,
			"synchronization_state": syncState,
			"avg_log_send_queue_kb": avgLogSend,
			"avg_redo_queue_kb":     avgRedo,
			"max_log_send_queue_kb": maxLogSend,
			"max_redo_queue_kb":     maxRedo,
			"secondary_lag_secs":    maxLagSecs,
			"sample_count":          sampleCount,
		})
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetAGHealthTimeSeries(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRange(from, to)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT
			time_bucket('5 minutes', capture_timestamp) AS bucket,
			AVG(log_send_queue_kb) AS avg_log_send_queue_kb,
			AVG(redo_queue_kb)     AS avg_redo_queue_kb,
			MAX(secondary_lag_seconds) AS max_lag_sec
		FROM monitor.sqlserver_ha_replica_state
		WHERE server_id = $1
		  AND capture_timestamp >= $2 AND capture_timestamp <= $3
		GROUP BY bucket
		ORDER BY bucket ASC
	`
	rows, err := tl.pool.Query(ctx, query, serverID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var avgLog, avgRedo, maxLag float64
		if err := rows.Scan(&bucket, &avgLog, &avgRedo, &maxLag); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"timestamp":             bucket,
			"avg_log_send_queue_kb": avgLog,
			"avg_redo_queue_kb":     avgRedo,
			"max_lag_sec":           maxLag,
		})
	}
	return results, nil
}

// LogAGHealthFromMap writes per-database AG health rows collected by the
// legacy health worker into the canonical monitor schema tables:
//   - monitor.sqlserver_ha_database_state  (one row per database per replica)
//   - monitor.sqlserver_ha_replica_state   (one row per replica, aggregated from the database rows)
//
// The legacy sqlserver_ag_health table (public schema) no longer receives new writes.
func (tl *TimescaleLogger) LogAGHealthFromMap(ctx context.Context, serverID uuid.UUID, agStats []map[string]interface{}) error {
	if len(agStats) == 0 {
		return nil
	}

	now := time.Now().UTC()

	// — per-database inserts —————————————————————————————————————————————
	dbBatch := &pgx.Batch{}
	for _, r := range agStats {
		dbBatch.Queue(`
			INSERT INTO monitor.sqlserver_ha_database_state (
				capture_timestamp, server_id, ag_name, database_name,
				replica_server_name, synchronization_state_desc, is_suspended,
				log_send_queue_kb, redo_queue_kb, last_commit_time, backup_fresh_ok
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (server_id, capture_timestamp, ag_name, database_name, replica_server_name) DO NOTHING`,
			now, serverID,
			getStr(r, "ag_name"),
			getStr(r, "database_name"),
			getStr(r, "replica_server_name"),
			getStr(r, "synchronization_state_desc"),
			false, // is_suspended not available from this query
			getInt64FromMap(r, "log_send_queue_kb"),
			getInt64FromMap(r, "redo_queue_kb"),
			nil,   // last_commit_time not available
			false, // backup_fresh_ok not available
		)
	}
	dbBR := tl.pool.SendBatch(ctx, dbBatch)
	for i := 0; i < len(agStats); i++ {
		if _, err := dbBR.Exec(); err != nil {
			slog.Error("ha_database_state batch insert failed", "row", i, "err", err)
		}
	}
	_ = dbBR.Close()

	// — per-replica aggregation ——————————————————————————————————————————
	// Group by (ag_name, replica_server_name); take MAX for queue/lag sizes.
	type replicaKey struct{ agName, replicaName string }
	type replicaAgg struct {
		logSendQueueKB    int64
		redoQueueKB       int64
		logSendRateKBPS   int64
		redoRateKBPS      int64
		secondaryLagSecs  int64
		syncStateDesc     string
		isPrimary         bool
	}
	agg := make(map[replicaKey]*replicaAgg)
	for _, r := range agStats {
		key := replicaKey{
			agName:      getStr(r, "ag_name"),
			replicaName: getStr(r, "replica_server_name"),
		}
		a, ok := agg[key]
		if !ok {
			a = &replicaAgg{syncStateDesc: getStr(r, "synchronization_state_desc")}
			agg[key] = a
		}
		if v := getInt64FromMap(r, "log_send_queue_kb"); v > a.logSendQueueKB {
			a.logSendQueueKB = v
		}
		if v := getInt64FromMap(r, "redo_queue_kb"); v > a.redoQueueKB {
			a.redoQueueKB = v
		}
		if v := getInt64FromMap(r, "log_send_rate_kb"); v > a.logSendRateKBPS {
			a.logSendRateKBPS = v
		}
		if v := getInt64FromMap(r, "redo_rate_kb"); v > a.redoRateKBPS {
			a.redoRateKBPS = v
		}
		if v := getInt64FromMap(r, "secondary_lag_seconds"); v > a.secondaryLagSecs {
			a.secondaryLagSecs = v
		}
		if getBool(r, "is_primary_replica") {
			a.isPrimary = true
		}
	}

	repBatch := &pgx.Batch{}
	for key, a := range agg {
		roleDesc := "SECONDARY"
		if a.isPrimary {
			roleDesc = "PRIMARY"
		}
		repBatch.Queue(`
			INSERT INTO monitor.sqlserver_ha_replica_state (
				capture_timestamp, server_id, ag_name, replica_server_name,
				role_desc, synchronization_state_desc, synchronization_health_desc,
				availability_mode_desc,
				log_send_queue_kb, redo_queue_kb, log_send_rate_kbps, redo_rate_kbps,
				last_commit_time, secondary_lag_seconds,
				connected_state_desc, is_failover_ready,
				long_running_tx_count, quorum_state_desc
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
			ON CONFLICT (server_id, capture_timestamp, ag_name, replica_server_name) DO NOTHING`,
			now, serverID, key.agName, key.replicaName,
			roleDesc, a.syncStateDesc, "UNKNOWN", "UNKNOWN",
			a.logSendQueueKB, a.redoQueueKB, a.logSendRateKBPS, a.redoRateKBPS,
			nil, a.secondaryLagSecs,
			"UNKNOWN", false, 0, "UNKNOWN",
		)
	}
	repBR := tl.pool.SendBatch(ctx, repBatch)
	defer repBR.Close()
	i := 0
	for range agg {
		if _, err := repBR.Exec(); err != nil {
			slog.Error("ha_replica_state batch insert failed", "row", i, "err", err)
		}
		i++
	}
	return nil
}


