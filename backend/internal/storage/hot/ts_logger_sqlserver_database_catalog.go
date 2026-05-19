// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB writer and reader for the SQL Server database catalog snapshot.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// LogDatabaseCatalog batch-inserts the sys.databases snapshot rows into
// sqlserver_database_catalog. Each call represents one collection cycle for a
// single SQL Server instance.
func (tl *TimescaleLogger) LogDatabaseCatalog(ctx context.Context, rows []SQLServerDatabaseCatalogRow) error {
	if len(rows) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_database_catalog (
				capture_timestamp, server_id,
				database_id, database_name, create_date,
				compatibility_level, collation_name,
				state_desc, user_access_desc,
				is_read_only, is_cleanly_shutdown,
				recovery_model_desc, log_reuse_wait_desc,
				delayed_durability_desc, target_recovery_time_in_seconds,
				is_accelerated_database_recovery_on,
				is_auto_close_on, is_auto_shrink_on, page_verify_option_desc,
				is_read_committed_snapshot_on, snapshot_isolation_state_desc,
				is_encrypted, is_cdc_enabled, is_broker_enabled,
				is_fulltext_enabled, is_memory_optimized_enabled,
				owner_name, containment_desc, is_trustworthy_on,
				is_published, is_subscribed, is_distributor,
				group_database_id
			) VALUES (
				$1,  $2,  $3,  $4,  $5,
				$6,  $7,  $8,  $9,  $10,
				$11, $12, $13, $14, $15,
				$16, $17, $18, $19, $20,
				$21, $22, $23, $24, $25,
				$26, $27, $28, $29, $30,
				$31, $32, $33
			)`,
			r.CaptureTimestamp, r.ServerID,
			r.DatabaseID, r.DatabaseName, r.CreateDate,
			r.CompatibilityLevel, r.CollationName,
			r.StateDesc, r.UserAccessDesc,
			r.IsReadOnly, r.IsCleanlyShutdown,
			r.RecoveryModelDesc, r.LogReuseWaitDesc,
			r.DelayedDurabilityDesc, r.TargetRecoveryTimeInSeconds,
			r.IsAcceleratedDatabaseRecovery,
			r.IsAutoCloseOn, r.IsAutoShrinkOn, r.PageVerifyOptionDesc,
			r.IsReadCommittedSnapshotOn, r.SnapshotIsolationStateDesc,
			r.IsEncrypted, r.IsCdcEnabled, r.IsBrokerEnabled,
			r.IsFulltextEnabled, r.IsMemoryOptimizedEnabled,
			r.OwnerName, r.ContainmentDesc, r.IsTrustworthyOn,
			r.IsPublished, r.IsSubscribed, r.IsDistributor,
			r.GroupDatabaseID,
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("database catalog batch insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

// GetLatestDatabaseCatalog returns the most-recent catalog snapshot row for each
// database belonging to the given server. Uses DISTINCT ON so that only one row
// per database_name is returned (the most recent capture_timestamp).
func (tl *TimescaleLogger) GetLatestDatabaseCatalog(ctx context.Context, serverID uuid.UUID) ([]SQLServerDatabaseCatalogRow, error) {
	const q = `
		SELECT DISTINCT ON (database_name)
			capture_timestamp, server_id,
			database_id, database_name, create_date,
			compatibility_level, collation_name,
			state_desc, user_access_desc,
			is_read_only, is_cleanly_shutdown,
			recovery_model_desc, log_reuse_wait_desc,
			delayed_durability_desc, target_recovery_time_in_seconds,
			is_accelerated_database_recovery_on,
			is_auto_close_on, is_auto_shrink_on, page_verify_option_desc,
			is_read_committed_snapshot_on, snapshot_isolation_state_desc,
			is_encrypted, is_cdc_enabled, is_broker_enabled,
			is_fulltext_enabled, is_memory_optimized_enabled,
			owner_name, containment_desc, is_trustworthy_on,
			is_published, is_subscribed, is_distributor,
			group_database_id
		FROM sqlserver_database_catalog
		WHERE server_id = $1
		ORDER BY database_name, capture_timestamp DESC`

	rows, err := tl.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, fmt.Errorf("GetLatestDatabaseCatalog query failed: %w", err)
	}
	defer rows.Close()

	var out []SQLServerDatabaseCatalogRow
	for rows.Next() {
		var r SQLServerDatabaseCatalogRow
		var captureTS, createDate time.Time
		var sid uuid.UUID
		if err := rows.Scan(
			&captureTS, &sid,
			&r.DatabaseID, &r.DatabaseName, &createDate,
			&r.CompatibilityLevel, &r.CollationName,
			&r.StateDesc, &r.UserAccessDesc,
			&r.IsReadOnly, &r.IsCleanlyShutdown,
			&r.RecoveryModelDesc, &r.LogReuseWaitDesc,
			&r.DelayedDurabilityDesc, &r.TargetRecoveryTimeInSeconds,
			&r.IsAcceleratedDatabaseRecovery,
			&r.IsAutoCloseOn, &r.IsAutoShrinkOn, &r.PageVerifyOptionDesc,
			&r.IsReadCommittedSnapshotOn, &r.SnapshotIsolationStateDesc,
			&r.IsEncrypted, &r.IsCdcEnabled, &r.IsBrokerEnabled,
			&r.IsFulltextEnabled, &r.IsMemoryOptimizedEnabled,
			&r.OwnerName, &r.ContainmentDesc, &r.IsTrustworthyOn,
			&r.IsPublished, &r.IsSubscribed, &r.IsDistributor,
			&r.GroupDatabaseID,
		); err != nil {
			return nil, fmt.Errorf("GetLatestDatabaseCatalog scan failed: %w", err)
		}
		r.CaptureTimestamp = captureTS
		r.ServerID = sid
		r.CreateDate = createDate
		out = append(out, r)
	}
	return out, rows.Err()
}
