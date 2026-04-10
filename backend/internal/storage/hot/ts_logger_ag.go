package hot

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

func (tl *TimescaleLogger) LogAGHealth(ctx context.Context, instanceName string, agStats []AGHealthRow) error {
	if len(agStats) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO sqlserver_ag_health (
			capture_timestamp, server_instance_name, ag_name, replica_server_name, database_name,
			replica_role, synchronization_state, synchronization_state_desc, is_primary_replica,
			log_send_queue_kb, redo_queue_kb, log_send_rate_kb, redo_rate_kb,
			last_sent_time, last_received_time, last_hardened_time, last_redone_time, secondary_lag_seconds
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`

	for _, r := range agStats {
		batch.Queue(query,
			r.CaptureTimestamp, r.ServerInstanceName, r.AGName, r.ReplicaServerName, r.DatabaseName,
			r.ReplicaRole, r.SynchronizationState, r.SyncStateDesc, r.IsPrimaryReplica,
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

func (tl *TimescaleLogger) GetAGHealthSummary(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT 
			ag_name,
			replica_server_name,
			database_name,
			replica_role,
			synchronization_state,
			is_primary_replica,
			AVG(log_send_queue_kb) AS avg_log_send_queue_kb,
			AVG(redo_queue_kb) AS avg_redo_queue_kb,
			MAX(log_send_queue_kb) AS max_log_send_queue_kb,
			MAX(redo_queue_kb) AS max_redo_queue_kb,
			COUNT(*) AS sample_count
		FROM sqlserver_ag_health
		WHERE server_instance_name = $1
		  AND capture_timestamp >= NOW() - INTERVAL '1 hour'
		GROUP BY ag_name, replica_server_name, database_name, replica_role, synchronization_state, is_primary_replica
		ORDER BY MAX(log_send_queue_kb) DESC, MAX(redo_queue_kb) DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var agName, replicaServer, dbName, replicaRole, syncState string
		var isPrimary bool
		var avgLogSend, avgRedo, maxLogSend, maxRedo float64
		var sampleCount int

		if err := rows.Scan(&agName, &replicaServer, &dbName, &replicaRole, &syncState, &isPrimary,
			&avgLogSend, &avgRedo, &maxLogSend, &maxRedo, &sampleCount); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"ag_name":               agName,
			"replica_server_name":   replicaServer,
			"database_name":         dbName,
			"replica_role":          replicaRole,
			"synchronization_state": syncState,
			"is_primary_replica":    isPrimary,
			"avg_log_send_queue_kb": avgLogSend,
			"avg_redo_queue_kb":     avgRedo,
			"max_log_send_queue_kb": maxLogSend,
			"max_redo_queue_kb":     maxRedo,
			"sample_count":          sampleCount,
		})
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) LogAGHealthFromMap(ctx context.Context, instanceName string, agStats []map[string]interface{}) error {
	if len(agStats) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	now := time.Now().UTC()

	for _, r := range agStats {
		batch.Queue(`
			INSERT INTO sqlserver_ag_health (
				capture_timestamp, server_instance_name, ag_name, replica_server_name, database_name,
				replica_role, synchronization_state, synchronization_state_desc, is_primary_replica,
				log_send_queue_kb, redo_queue_kb, log_send_rate_kb, redo_rate_kb,
				last_sent_time, last_received_time, last_hardened_time, last_redone_time, secondary_lag_seconds
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
			now, instanceName,
			getStr(r, "ag_name"),
			getStr(r, "replica_server_name"),
			getStr(r, "database_name"),
			getStr(r, "replica_role"),
			getStr(r, "synchronization_state"),
			getStr(r, "synchronization_state_desc"),
			getBool(r, "is_primary_replica"),
			getInt64FromMap(r, "log_send_queue_kb"),
			getInt64FromMap(r, "redo_queue_kb"),
			getInt64FromMap(r, "log_send_rate_kb"),
			getInt64FromMap(r, "redo_rate_kb"),
			getStr(r, "last_sent_time"),
			getStr(r, "last_received_time"),
			getStr(r, "last_hardened_time"),
			getStr(r, "last_redone_time"),
			getInt64FromMap(r, "secondary_lag_seconds"),
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(agStats); i++ {
		if _, err := br.Exec(); err != nil {
			log.Printf("[TSLogger] AG health batch insert failed at row %d: %v", i, err)
		}
	}
	return nil
}

func getInt64FromMap(m map[string]interface{}, key string) int64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int64:
			return val
		case int:
			return int64(val)
		case float64:
			return int64(val)
		}
	}
	return 0
}
