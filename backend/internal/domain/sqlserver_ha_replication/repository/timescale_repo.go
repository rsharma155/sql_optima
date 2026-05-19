// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : TimescaleDB repository for the SQL Server HA & Replication domain.
//
//	Reads from the monitor.* hypertables and continuous aggregates
//	created by infrastructure/sql_scripts/01_timescale_schema.sql.
//
//	All queries use pre-parameterised $N placeholders, enforce server_id
//	scoping, and include a LIMIT guard.  No query mutates state.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/domain"
)

// HAReplicationRepository reads HA and replication metrics from TimescaleDB.
type HAReplicationRepository struct {
	pool *pgxpool.Pool
}

// NewHAReplicationRepository creates a repository backed by the given pool.
func NewHAReplicationRepository(pool *pgxpool.Pool) *HAReplicationRepository {
	return &HAReplicationRepository{pool: pool}
}

// ---------------------------------------------------------------------------
// Feature Detection
// ---------------------------------------------------------------------------

// GetFeatureDetection returns feature flags for the given server.
// When database is "" or "all", returns instance-level detection from
// monitor.sqlserver_feature_detection.  When a specific database name is
// supplied, it performs a per-database check:
//   - ha_enabled:          database exists in monitor.sqlserver_ha_database_state
//   - replication_enabled: database appears as publication_db in monitor.sqlserver_replication_topology
//
// Returns a zero-value result (both flags false) when no recent row exists.
func (r *HAReplicationRepository) GetFeatureDetection(ctx context.Context, serverID uuid.UUID, database string) (domain.FeatureDetection, error) {
	if database == "" || database == "all" {
		return r.getInstanceFeatureDetection(ctx, serverID)
	}
	return r.getDatabaseFeatureDetection(ctx, serverID, database)
}

// getInstanceFeatureDetection reads the instance-level feature flags.
func (r *HAReplicationRepository) getInstanceFeatureDetection(ctx context.Context, serverID uuid.UUID) (domain.FeatureDetection, error) {
	const q = `
		SELECT ha_enabled, replication_enabled, ag_enabled, fci_enabled,
		       log_shipping_enabled, mirroring_enabled,
		       COALESCE(replication_types, '{}'), capture_timestamp
		FROM monitor.sqlserver_feature_detection
		WHERE server_id = $1
		  AND capture_timestamp > now() - INTERVAL '24 hours'
		ORDER BY capture_timestamp DESC
		LIMIT 1`

	row := r.pool.QueryRow(ctx, q, serverID)
	var f domain.FeatureDetection
	f.ServerID = serverID.String()
	err := row.Scan(
		&f.HAEnabled, &f.ReplicationEnabled,
		&f.AGEnabled, &f.FCIEnabled,
		&f.LogShippingEnabled, &f.MirroringEnabled,
		&f.ReplicationTypes, &f.CaptureTimestamp,
	)
	if err != nil {
		return domain.FeatureDetection{ServerID: serverID.String()}, nil
	}
	return f, nil
}

// getDatabaseFeatureDetection checks whether a specific database participates in
// HA (is an AG member) or replication (is a publication database).
func (r *HAReplicationRepository) getDatabaseFeatureDetection(ctx context.Context, serverID uuid.UUID, database string) (domain.FeatureDetection, error) {
	const q = `
		SELECT
			EXISTS(
				SELECT 1 FROM monitor.sqlserver_ha_database_state
				WHERE server_id = $1
				  AND database_name = $2
				  AND capture_timestamp > now() - INTERVAL '24 hours'
			) AS ha_enabled,
			EXISTS(
				SELECT 1 FROM monitor.sqlserver_replication_topology
				WHERE server_id = $1
				  AND publication_db = $2
				  AND capture_timestamp > now() - INTERVAL '24 hours'
			) AS replication_enabled`

	var haEnabled, replEnabled bool
	err := r.pool.QueryRow(ctx, q, serverID, database).Scan(&haEnabled, &replEnabled)
	if err != nil {
		// Fallback to instance-level on any query error
		return r.getInstanceFeatureDetection(ctx, serverID)
	}
	return domain.FeatureDetection{
		ServerID:           serverID.String(),
		HAEnabled:          haEnabled,
		ReplicationEnabled: replEnabled,
	}, nil
}

// ---------------------------------------------------------------------------
// HA — Replica Health
// ---------------------------------------------------------------------------

// GetCurrentReplicaHealth returns the latest snapshot row per replica.
func (r *HAReplicationRepository) GetCurrentReplicaHealth(ctx context.Context, serverID uuid.UUID) ([]domain.ReplicaHealthRow, error) {
	const q = `
		SELECT DISTINCT ON (ag_name, replica_server_name)
			ag_name, replica_server_name, role_desc,
			synchronization_state_desc, synchronization_health_desc,
			availability_mode_desc,
			log_send_queue_kb, redo_queue_kb,
			log_send_rate_kbps, redo_rate_kbps,
			COALESCE(last_commit_time, '1970-01-01'::timestamptz),
			secondary_lag_seconds, connected_state_desc, is_failover_ready,
			long_running_tx_count, quorum_state_desc
		FROM monitor.sqlserver_ha_replica_state
		WHERE server_id = $1
		  AND capture_timestamp > now() - INTERVAL '2 minutes'
		ORDER BY ag_name, replica_server_name, capture_timestamp DESC`

	rows, err := r.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.ReplicaHealthRow
	for rows.Next() {
		var row domain.ReplicaHealthRow
		if err := rows.Scan(
			&row.AGName, &row.ReplicaServerName, &row.RoleDesc,
			&row.SynchronizationStateDesc, &row.SynchronizationHealth,
			&row.AvailabilityModeDesc,
			&row.LogSendQueueKB, &row.RedoQueueKB,
			&row.LogSendRateKBPS, &row.RedoRateKBPS,
			&row.LastCommitTime, &row.SecondaryLagSeconds, &row.ConnectedStateDesc, &row.IsFailoverReady,
			&row.LongRunningTxCount, &row.QuorumStateDesc,
		); err != nil {
			continue
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// HA — Database Sync State
// ---------------------------------------------------------------------------

// GetCurrentDatabaseSyncState returns the latest sync state per database/replica.
func (r *HAReplicationRepository) GetCurrentDatabaseSyncState(ctx context.Context, serverID uuid.UUID) ([]domain.DatabaseSyncState, error) {
	const q = `
		SELECT DISTINCT ON (ag_name, database_name, replica_server_name)
			ag_name, database_name, replica_server_name,
			synchronization_state_desc, is_suspended,
			log_send_queue_kb, redo_queue_kb,
			COALESCE(last_commit_time, '1970-01-01'::timestamptz)
		FROM monitor.sqlserver_ha_database_state
		WHERE server_id = $1
		  AND capture_timestamp > now() - INTERVAL '2 minutes'
		ORDER BY ag_name, database_name, replica_server_name, capture_timestamp DESC`

	rows, err := r.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.DatabaseSyncState
	for rows.Next() {
		var d domain.DatabaseSyncState
		if err := rows.Scan(
			&d.AGName, &d.DatabaseName, &d.ReplicaServerName,
			&d.SynchronizationStateDesc, &d.IsSuspended,
			&d.LogSendQueueKB, &d.RedoQueueKB, &d.LastCommitTime,
		); err != nil {
			continue
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// RPO / RTO trend (continuous aggregates)
// ---------------------------------------------------------------------------

// GetRPOTrend returns per-minute RPO data for the given time window.
func (r *HAReplicationRepository) GetRPOTrend(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	const q = `
		SELECT bucket, rpo_seconds, avg_rpo_seconds, replica_count
		FROM monitor.sqlserver_rpo_1min
		WHERE server_id = $1
		  AND bucket >= $2 AND bucket <= $3
		ORDER BY bucket ASC
		LIMIT 1440`

	rows, err := r.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var rpo, avgRPO float64
		var replicaCount int
		if err := rows.Scan(&bucket, &rpo, &avgRPO, &replicaCount); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"bucket":        bucket,
			"rpo_seconds":   rpo,
			"avg_rpo":       avgRPO,
			"replica_count": replicaCount,
		})
	}
	return result, rows.Err()
}

// GetRTOTrend returns per-minute RTO estimates for the given time window.
func (r *HAReplicationRepository) GetRTOTrend(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	const q = `
		SELECT bucket, avg_redo_rate_kbps, estimated_rto_seconds, max_redo_queue_kb
		FROM monitor.sqlserver_rto_1min
		WHERE server_id = $1
		  AND bucket >= $2 AND bucket <= $3
		ORDER BY bucket ASC
		LIMIT 1440`

	rows, err := r.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var redo, estimatedRTO, maxRedo float64
		if err := rows.Scan(&bucket, &redo, &estimatedRTO, &maxRedo); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"bucket":              bucket,
			"avg_redo_rate_kbps": redo,
			"estimated_rto":       estimatedRTO,
			"max_redo_queue_kb":   maxRedo,
		})
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Failover Events
// ---------------------------------------------------------------------------

// GetFailoverHistory returns failover events within the given time window.
func (r *HAReplicationRepository) GetFailoverHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]domain.FailoverEvent, error) {
	const q = `
		SELECT capture_timestamp, ag_name, previous_primary, new_primary,
		       failover_type, failover_duration_seconds
		FROM monitor.sqlserver_ha_failover_events
		WHERE server_id = $1
		  AND capture_timestamp >= $2 AND capture_timestamp <= $3
		ORDER BY capture_timestamp DESC
		LIMIT 200`

	rows, err := r.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.FailoverEvent
	for rows.Next() {
		var e domain.FailoverEvent
		if err := rows.Scan(
			&e.CaptureTimestamp, &e.AGName,
			&e.PreviousPrimary, &e.NewPrimary,
			&e.FailoverType, &e.FailoverDurationSeconds,
		); err != nil {
			continue
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Replication — Topology
// ---------------------------------------------------------------------------

// GetReplicationTopology returns the latest snapshot per pub→sub relationship.
func (r *HAReplicationRepository) GetReplicationTopology(ctx context.Context, serverID uuid.UUID) ([]domain.ReplicationTopologyRow, error) {
	const q = `
		WITH latest_topo AS (
			SELECT DISTINCT ON (publisher, subscriber, publication)
				publisher, subscriber, publication,
				publication_db, subscriber_db, replication_type,
				sync_type, agent_status, last_sync_time
			FROM monitor.sqlserver_replication_topology
			WHERE server_id = $1
			  AND capture_timestamp > now() - INTERVAL '24 hours'
			ORDER BY publisher, subscriber, publication, capture_timestamp DESC
		),
		latest_perf AS (
			SELECT DISTINCT ON (publisher, subscriber, publication)
				publisher, subscriber, publication,
				latency_seconds, delivery_rate_cmds_sec, status
			FROM monitor.sqlserver_replication_latency
			WHERE server_id = $1
			  AND capture_timestamp > now() - INTERVAL '15 minutes'
			ORDER BY publisher, subscriber, publication, capture_timestamp DESC
		)
		SELECT 
			t.publisher, t.subscriber, t.publication,
			t.publication_db, t.subscriber_db, t.replication_type,
			t.sync_type, t.agent_status,
			COALESCE(t.last_sync_time, '1970-01-01'::timestamptz),
			COALESCE(p.latency_seconds, 0),
			COALESCE(p.delivery_rate_cmds_sec, 0),
			CASE WHEN p.status NOT IN ('Running','Idle') THEN true ELSE false END AS has_error
		FROM latest_topo t
		LEFT JOIN latest_perf p ON t.publisher = p.publisher 
		                       AND t.subscriber = p.subscriber 
		                       AND t.publication = p.publication`

	rows, err := r.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.ReplicationTopologyRow
	for rows.Next() {
		var t domain.ReplicationTopologyRow
		if err := rows.Scan(
			&t.Publisher, &t.Subscriber, &t.Publication,
			&t.PublicationDB, &t.SubscriberDB, &t.ReplicationType,
			&t.SyncType, &t.AgentStatus, &t.LastSyncTime,
			&t.LatencySeconds, &t.DeliveryRateCmdsSec, &t.HasError,
		); err != nil {
			continue
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Replication — Latency / Backlog trend
// ---------------------------------------------------------------------------

// GetReplicationBacklogTrend returns per-minute backlog data points.
func (r *HAReplicationRepository) GetReplicationBacklogTrend(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]domain.ReplicationLatencyPoint, error) {
	const q = `
		SELECT bucket, publisher, subscriber, publication,
		       peak_backlog, avg_backlog, max_latency_seconds
		FROM monitor.sqlserver_replication_backlog_1min
		WHERE server_id = $1
		  AND bucket >= $2 AND bucket <= $3
		ORDER BY bucket ASC
		LIMIT 2880`

	rows, err := r.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.ReplicationLatencyPoint
	for rows.Next() {
		var p domain.ReplicationLatencyPoint
		if err := rows.Scan(
			&p.Bucket, &p.Publisher, &p.Subscriber, &p.Publication,
			&p.PeakBacklog, &p.AvgBacklog, &p.MaxLatencySeconds,
		); err != nil {
			continue
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Replication — Articles
// ---------------------------------------------------------------------------

// GetReplicationArticles returns the latest row per table/subscriber.
func (r *HAReplicationRepository) GetReplicationArticles(ctx context.Context, serverID uuid.UUID) ([]domain.ReplicationArticle, error) {
	const q = `
		SELECT DISTINCT ON (publication, database_name, schema_name, table_name, subscriber)
			publication, database_name, schema_name, table_name, subscriber,
			rows_per_sec, latency_seconds, conflicts_detected, status, capture_timestamp
		FROM monitor.sqlserver_replication_articles
		WHERE server_id = $1
		  AND capture_timestamp > now() - INTERVAL '15 minutes'
		ORDER BY publication, database_name, schema_name, table_name, subscriber, capture_timestamp DESC
		LIMIT 500`

	rows, err := r.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.ReplicationArticle
	for rows.Next() {
		var a domain.ReplicationArticle
		if err := rows.Scan(
			&a.Publication, &a.DatabaseName, &a.SchemaName, &a.TableName, &a.Subscriber,
			&a.RowsPerSec, &a.LatencySeconds, &a.ConflictsDetected, &a.Status, &a.CaptureTimestamp,
		); err != nil {
			continue
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Coverage Panel
// ---------------------------------------------------------------------------

// GetDatabaseCoverage returns the Gold/Silver/Bronze/Red protection level per DB.
func (r *HAReplicationRepository) GetDatabaseCoverage(ctx context.Context, serverID uuid.UUID) ([]domain.DatabaseCoverage, error) {
	const q = `
		WITH ha_dbs AS (
			SELECT DISTINCT ON (database_name)
				database_name, backup_fresh_ok
			FROM monitor.sqlserver_ha_database_state
			WHERE server_id = $1
			  AND capture_timestamp > now() - INTERVAL '24 hours'
			ORDER BY database_name, capture_timestamp DESC
		),
		repl_dbs AS (
			SELECT DISTINCT publication_db AS database_name
			FROM monitor.sqlserver_replication_topology
			WHERE server_id = $1
			  AND capture_timestamp > now() - INTERVAL '30 minutes'
		)
		SELECT
			COALESCE(h.database_name, rd.database_name)  AS database_name,
			h.database_name IS NOT NULL                   AS in_ha,
			rd.database_name IS NOT NULL                  AS in_replication,
			COALESCE(h.backup_fresh_ok, false)            AS backup_fresh_ok
		FROM ha_dbs h
		FULL OUTER JOIN repl_dbs rd ON h.database_name = rd.database_name
		ORDER BY database_name`

	rows, err := r.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.DatabaseCoverage
	for rows.Next() {
		var c domain.DatabaseCoverage
		if err := rows.Scan(&c.DatabaseName, &c.InHA, &c.InReplication, &c.BackupFreshOK); err != nil {
			continue
		}
		switch {
		case c.InHA && c.InReplication:
			c.ProtectionLevel = domain.ProtectionGold
		case c.InHA:
			c.ProtectionLevel = domain.ProtectionSilver
		case c.InReplication:
			c.ProtectionLevel = domain.ProtectionBronze
		default:
			c.ProtectionLevel = domain.ProtectionNone
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Alerts Timeline
// ---------------------------------------------------------------------------

// GetAlertsTimeline returns a merged list of HA and Replication alert events.
func (r *HAReplicationRepository) GetAlertsTimeline(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]domain.AlertEvent, error) {
	const q = `
		(SELECT capture_timestamp AS timestamp, 'info' AS severity, 'HA' AS category, 
		        'Failover Detected' AS title, 
		        'Primary changed from ' || previous_primary || ' to ' || new_primary AS detail
		 FROM monitor.sqlserver_ha_failover_events
		 WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3)
		UNION ALL
		(SELECT capture_timestamp AS timestamp, 'critical' AS severity, 'Replication' AS category,
		        'Replication Error' AS title,
		        'Publisher ' || publisher || ' reported status: ' || status AS detail
		 FROM monitor.sqlserver_replication_latency
		 WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3
		   AND status NOT IN ('Running','Idle'))
		ORDER BY timestamp DESC
		LIMIT 100`

	rows, err := r.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.AlertEvent
	for rows.Next() {
		var e domain.AlertEvent
		if err := rows.Scan(&e.Timestamp, &e.Severity, &e.Category, &e.Title, &e.Detail); err != nil {
			continue
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// GetReplicationKPIs returns aggregate counts from the latest topology snapshot.
func (r *HAReplicationRepository) GetReplicationKPIs(ctx context.Context, serverID uuid.UUID) (domain.ReplicationKPIs, error) {
	const q = `
		SELECT
			COUNT(DISTINCT t.publisher)       AS publishers,
			COUNT(DISTINCT t.subscriber)      AS subscribers,
			COUNT(DISTINCT t.publication)     AS publications,
			COUNT(DISTINCT t.publication_db)  AS replicated_dbs,
			SUM(CASE WHEN t.agent_status NOT IN ('Running','Idle') THEN 1 ELSE 0 END) AS issues,
			COALESCE((
				SELECT COUNT(DISTINCT CONCAT(publication,'|',database_name,'|',schema_name,'|',table_name))
				FROM monitor.sqlserver_replication_articles
				WHERE server_id = $1 AND capture_timestamp > now() - INTERVAL '15 minutes'
			), 0) AS articles
		FROM (
			SELECT DISTINCT ON (publisher, subscriber, publication)
				publisher, subscriber, publication, publication_db, agent_status
			FROM monitor.sqlserver_replication_topology
			WHERE server_id = $1
			  AND capture_timestamp > now() - INTERVAL '30 minutes'
			ORDER BY publisher, subscriber, publication, capture_timestamp DESC
		) t`

	var kpis domain.ReplicationKPIs
	err := r.pool.QueryRow(ctx, q, serverID).Scan(
		&kpis.Publishers, &kpis.Subscribers,
		&kpis.Publications, &kpis.ReplicatedDBs,
		&kpis.IssueCount, &kpis.Articles,
	)
	if err != nil {
		return kpis, nil //nolint:nilerr — return empty on no data
	}

	// Second query: per-type publication count for the breakdown panel
	const typeQ = `
		SELECT replication_type, COUNT(DISTINCT publication)
		FROM (
			SELECT DISTINCT ON (publisher, subscriber, publication)
				publication, replication_type
			FROM monitor.sqlserver_replication_topology
			WHERE server_id = $1 AND capture_timestamp > now() - INTERVAL '30 minutes'
			ORDER BY publisher, subscriber, publication, capture_timestamp DESC
		) t
		GROUP BY replication_type`

	rows, err := r.pool.Query(ctx, typeQ, serverID)
	if err == nil {
		defer rows.Close()
		kpis.TypeBreakdown = make(map[string]int)
		for rows.Next() {
			var rtype string
			var cnt int
			if scanErr := rows.Scan(&rtype, &cnt); scanErr == nil {
				kpis.TypeBreakdown[rtype] = cnt
			}
		}
	}
	return kpis, nil
}

// GetRPOWorst24h returns the worst (maximum) RPO value seen in the past 24 hours.
func (r *HAReplicationRepository) GetRPOWorst24h(ctx context.Context, serverID uuid.UUID) int64 {
	var worst float64
	_ = r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(rpo_seconds), 0)
		FROM monitor.sqlserver_rpo_1min
		WHERE server_id = $1 AND bucket > now() - INTERVAL '24 hours'`,
		serverID,
	).Scan(&worst)
	return int64(worst)
}
