// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : TimescaleDB batch writers for the SQL Server HA & Replication
//           monitoring domain.  Each function writes to one of the
//           monitor.sqlserver_* hypertables created by 01_timescale_schema.sql.
//
//           All writes use pgx batch mode for efficiency and ON CONFLICT 
//           DO NOTHING to prevent duplicate metric insertion for the same pulse.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package hot

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Feature Detection
// ---------------------------------------------------------------------------

type HAFeatureDetectionRow struct {
	ServerID           uuid.UUID
	CaptureTimestamp   time.Time
	HAEnabled          bool
	ReplicationEnabled bool
	AGEnabled          bool
	FCIEnabled         bool
	LogShippingEnabled bool
	MirroringEnabled   bool
	ReplicationTypes   []string
}

func (tl *TimescaleLogger) WriteHAFeatureDetection(ctx context.Context, row HAFeatureDetectionRow) error {
	const q = `
		INSERT INTO monitor.sqlserver_feature_detection (
			server_id, capture_timestamp,
			ha_enabled, replication_enabled,
			ag_enabled, fci_enabled, log_shipping_enabled, mirroring_enabled,
			replication_types
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (server_id, capture_timestamp) DO NOTHING`

	_, err := tl.pool.Exec(ctx, q,
		row.ServerID, row.CaptureTimestamp,
		row.HAEnabled, row.ReplicationEnabled,
		row.AGEnabled, row.FCIEnabled, row.LogShippingEnabled, row.MirroringEnabled,
		row.ReplicationTypes,
	)
	return err
}

// ---------------------------------------------------------------------------
// HA Replica State
// ---------------------------------------------------------------------------

type HAReplicaStateRow struct {
	CaptureTimestamp           time.Time
	ServerID                   uuid.UUID
	AGName                     string
	ReplicaServerName          string
	RoleDesc                   string
	SynchronizationStateDesc   string
	SynchronizationHealthDesc  string
	AvailabilityModeDesc       string
	LogSendQueueKB             int64
	RedoQueueKB                int64
	LogSendRateKBPS            int64
	RedoRateKBPS               int64
	LastCommitTime             *time.Time
	SecondaryLagSeconds        int64
	ConnectedStateDesc         string
	IsFailoverReady            bool
	LongRunningTxCount         int
	QuorumStateDesc            string
}

func (tl *TimescaleLogger) WriteHAReplicaState(ctx context.Context, rows []HAReplicaStateRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO monitor.sqlserver_ha_replica_state (
			capture_timestamp, server_id, ag_name, replica_server_name,
			role_desc, synchronization_state_desc, synchronization_health_desc,
			availability_mode_desc,
			log_send_queue_kb, redo_queue_kb, log_send_rate_kbps, redo_rate_kbps,
			last_commit_time, secondary_lag_seconds,
			connected_state_desc, is_failover_ready,
			long_running_tx_count, quorum_state_desc
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		ON CONFLICT (server_id, capture_timestamp, ag_name, replica_server_name) DO NOTHING`

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q,
			r.CaptureTimestamp, r.ServerID, r.AGName, r.ReplicaServerName,
			r.RoleDesc, r.SynchronizationStateDesc, r.SynchronizationHealthDesc,
			r.AvailabilityModeDesc,
			r.LogSendQueueKB, r.RedoQueueKB, r.LogSendRateKBPS, r.RedoRateKBPS,
			r.LastCommitTime, r.SecondaryLagSeconds,
			r.ConnectedStateDesc, r.IsFailoverReady,
			r.LongRunningTxCount, r.QuorumStateDesc,
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("ha_replica_state batch row %d: %w", i, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// HA Database State
// ---------------------------------------------------------------------------

type HADatabaseStateRow struct {
	CaptureTimestamp          time.Time
	ServerID                  uuid.UUID
	AGName                    string
	DatabaseName              string
	ReplicaServerName         string
	SynchronizationStateDesc  string
	IsSuspended               bool
	LogSendQueueKB            int64
	RedoQueueKB               int64
	LastCommitTime            *time.Time
	BackupFreshOK             bool
}

func (tl *TimescaleLogger) WriteHADatabaseState(ctx context.Context, rows []HADatabaseStateRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO monitor.sqlserver_ha_database_state (
			capture_timestamp, server_id, ag_name, database_name,
			replica_server_name, synchronization_state_desc, is_suspended,
			log_send_queue_kb, redo_queue_kb, last_commit_time, backup_fresh_ok
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (server_id, capture_timestamp, ag_name, database_name, replica_server_name) DO NOTHING`

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q,
			r.CaptureTimestamp, r.ServerID, r.AGName, r.DatabaseName,
			r.ReplicaServerName, r.SynchronizationStateDesc, r.IsSuspended,
			r.LogSendQueueKB, r.RedoQueueKB, r.LastCommitTime, r.BackupFreshOK,
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("ha_database_state batch row %d: %w", i, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Failover Events
// ---------------------------------------------------------------------------

type HAFailoverEventRow struct {
	CaptureTimestamp        time.Time
	ServerID                uuid.UUID
	AGName                  string
	PreviousPrimary         string
	NewPrimary              string
	FailoverType            string
	FailoverDurationSeconds int
}

func (tl *TimescaleLogger) WriteHAFailoverEvent(ctx context.Context, row HAFailoverEventRow) error {
	const q = `
		INSERT INTO monitor.sqlserver_ha_failover_events (
			capture_timestamp, server_id, ag_name,
			previous_primary, new_primary, failover_type, failover_duration_seconds
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (server_id, capture_timestamp, ag_name, previous_primary, new_primary) DO NOTHING`

	_, err := tl.pool.Exec(ctx, q,
		row.CaptureTimestamp, row.ServerID, row.AGName,
		row.PreviousPrimary, row.NewPrimary, row.FailoverType, row.FailoverDurationSeconds,
	)
	return err
}

// ---------------------------------------------------------------------------
// Replication Topology
// ---------------------------------------------------------------------------

type ReplicationTopologyRow struct {
	CaptureTimestamp time.Time
	ServerID         uuid.UUID
	Publisher        string
	Subscriber       string
	Publication      string
	PublicationDB    string
	SubscriberDB     string
	ReplicationType  string
	SyncType         string
	AgentStatus      string
	LastSyncTime     *time.Time
}

func (tl *TimescaleLogger) WriteReplicationTopology(ctx context.Context, rows []ReplicationTopologyRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO monitor.sqlserver_replication_topology (
			capture_timestamp, server_id, publisher, subscriber, publication,
			publication_db, subscriber_db, replication_type, sync_type,
			agent_status, last_sync_time
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (server_id, capture_timestamp, publisher, subscriber, publication) DO NOTHING`

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q,
			r.CaptureTimestamp, r.ServerID, r.Publisher, r.Subscriber, r.Publication,
			r.PublicationDB, r.SubscriberDB, r.ReplicationType, r.SyncType,
			r.AgentStatus, r.LastSyncTime,
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("replication_topology batch row %d: %w", i, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Replication Latency
// ---------------------------------------------------------------------------

type ReplicationLatencyRow struct {
	CaptureTimestamp      time.Time
	ServerID              uuid.UUID
	Publisher             string
	Subscriber            string
	Publication           string
	LatencySeconds        int64
	UndistributedCommands int64
	DeliveryRateCmdsSec   int64
	Status                string
}

func (tl *TimescaleLogger) WriteReplicationLatency(ctx context.Context, rows []ReplicationLatencyRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO monitor.sqlserver_replication_latency (
			capture_timestamp, server_id, publisher, subscriber, publication,
			latency_seconds, undistributed_commands, delivery_rate_cmds_sec, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (server_id, capture_timestamp, publisher, subscriber, publication) DO NOTHING`

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q,
			r.CaptureTimestamp, r.ServerID, r.Publisher, r.Subscriber, r.Publication,
			r.LatencySeconds, r.UndistributedCommands, r.DeliveryRateCmdsSec, r.Status,
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("replication_latency batch row %d: %w", i, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Replication Articles
// ---------------------------------------------------------------------------

type ReplicationArticleRow struct {
	CaptureTimestamp  time.Time
	ServerID          uuid.UUID
	Publication       string
	DatabaseName      string
	SchemaName        string
	TableName         string
	Subscriber        string
	RowsPerSec        int64
	LatencySeconds    int64
	ConflictsDetected int64
	Status            string
}

func (tl *TimescaleLogger) WriteReplicationArticles(ctx context.Context, rows []ReplicationArticleRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO monitor.sqlserver_replication_articles (
			capture_timestamp, server_id, publication, database_name,
			schema_name, table_name, subscriber,
			rows_per_sec, latency_seconds, conflicts_detected, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (server_id, capture_timestamp, publication, database_name, schema_name, table_name, subscriber) DO NOTHING`

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q,
			r.CaptureTimestamp, r.ServerID, r.Publication, r.DatabaseName,
			r.SchemaName, r.TableName, r.Subscriber,
			r.RowsPerSec, r.LatencySeconds, r.ConflictsDetected, r.Status,
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("replication_articles batch row %d: %w", i, err)
		}
	}
	return nil
}
