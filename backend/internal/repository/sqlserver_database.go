// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server database listing, catalog snapshot, and utilities.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DatabaseCatalogRow holds a single row from the sys.databases catalog snapshot.
type DatabaseCatalogRow struct {
	DatabaseID                     int
	Name                           string
	CreateDate                     time.Time
	CompatibilityLevel             int
	CollationName                  string
	StateDesc                      string
	UserAccessDesc                 string
	IsReadOnly                     bool
	IsCleanlyShutdown              bool
	RecoveryModelDesc              string
	LogReuseWaitDesc               string
	DelayedDurabilityDesc          string
	TargetRecoveryTimeInSeconds    int
	IsAcceleratedDatabaseRecovery  bool
	IsAutoCloseOn                  bool
	IsAutoShrinkOn                 bool
	PageVerifyOptionDesc           string
	IsReadCommittedSnapshotOn      bool
	SnapshotIsolationStateDesc     string
	IsEncrypted                    bool
	IsCdcEnabled                   bool
	IsBrokerEnabled                bool
	IsFulltextEnabled              bool
	IsMemoryOptimizedEnabled       bool
	OwnerName                      *string
	ContainmentDesc                string
	IsTrustworthyOn                bool
	IsPublished                    bool
	IsSubscribed                   bool
	IsDistributor                  bool
	GroupDatabaseID                *string
}

func (c *SqlServerRepository) ListSQLServerUserDatabases(ctx context.Context, instanceName string) ([]string, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	const q = `
		/* SQL_OPTIMA */ SELECT d.name
		FROM sys.databases AS d
		WHERE d.database_id > 4
		AND d.state_desc = N'ONLINE'
		AND d.name <> N'distribution'
		AND d.source_database_id IS NULL
		AND HAS_DBACCESS(d.name) = 1
		ORDER BY d.name;
	`
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			continue
		}
		n = strings.TrimSpace(n)
		if n != "" {
			names = append(names, n)
		}
	}
	return names, rows.Err()
}

// FetchDatabaseCatalog queries sys.databases for all accessible online user
// databases and returns the full configuration snapshot. This is the canonical
// source of database metadata for a SQL Server instance.
func (c *SqlServerRepository) FetchDatabaseCatalog(ctx context.Context, instanceName string) ([]DatabaseCatalogRow, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found for instance %q", instanceName)
	}

	// is_accelerated_database_recovery_on was added in SQL Server 2019; use CAST(0) on older
	// versions to keep one query that works across all supported versions.
	const q = `/* SQL_OPTIMA */
SELECT
    d.database_id,
    d.name,
    d.create_date,
    d.compatibility_level,
    d.collation_name,
    d.state_desc,
    d.user_access_desc,
    d.is_read_only,
    d.is_cleanly_shutdown,
    d.recovery_model_desc,
    d.log_reuse_wait_desc,
    d.delayed_durability_desc,
    d.target_recovery_time_in_seconds,
    CASE WHEN CAST(SERVERPROPERTY('ProductMajorVersion') AS INT) >= 15
         THEN (SELECT TOP 1 dc2.is_accelerated_database_recovery_on
               FROM sys.databases dc2 WHERE dc2.database_id = d.database_id)
         ELSE CAST(0 AS BIT)
    END AS is_accelerated_database_recovery_on,
    d.is_auto_close_on,
    d.is_auto_shrink_on,
    d.page_verify_option_desc,
    d.is_read_committed_snapshot_on,
    d.snapshot_isolation_state_desc,
    d.is_encrypted,
    d.is_cdc_enabled,
    d.is_broker_enabled,
    d.is_fulltext_enabled,
    d.is_memory_optimized_enabled,
    SUSER_SNAME(d.owner_sid)  AS owner_name,
    d.containment_desc,
    d.is_trustworthy_on,
    d.is_published,
    d.is_subscribed,
    d.is_distributor,
    CONVERT(NVARCHAR(36), d.group_database_id) AS group_database_id
FROM sys.databases AS d
WHERE d.database_id > 4
  AND d.state_desc   = N'ONLINE'
  AND d.name        <> N'distribution'
  AND d.source_database_id IS NULL
  AND HAS_DBACCESS(d.name) = 1
ORDER BY d.name;`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("FetchDatabaseCatalog query failed: %w", err)
	}
	defer rows.Close()

	var out []DatabaseCatalogRow
	for rows.Next() {
		var r DatabaseCatalogRow
		var ownerName, groupDBID sql.NullString
		if err := rows.Scan(
			&r.DatabaseID,
			&r.Name,
			&r.CreateDate,
			&r.CompatibilityLevel,
			&r.CollationName,
			&r.StateDesc,
			&r.UserAccessDesc,
			&r.IsReadOnly,
			&r.IsCleanlyShutdown,
			&r.RecoveryModelDesc,
			&r.LogReuseWaitDesc,
			&r.DelayedDurabilityDesc,
			&r.TargetRecoveryTimeInSeconds,
			&r.IsAcceleratedDatabaseRecovery,
			&r.IsAutoCloseOn,
			&r.IsAutoShrinkOn,
			&r.PageVerifyOptionDesc,
			&r.IsReadCommittedSnapshotOn,
			&r.SnapshotIsolationStateDesc,
			&r.IsEncrypted,
			&r.IsCdcEnabled,
			&r.IsBrokerEnabled,
			&r.IsFulltextEnabled,
			&r.IsMemoryOptimizedEnabled,
			&ownerName,
			&r.ContainmentDesc,
			&r.IsTrustworthyOn,
			&r.IsPublished,
			&r.IsSubscribed,
			&r.IsDistributor,
			&groupDBID,
		); err != nil {
			continue
		}
		if ownerName.Valid {
			r.OwnerName = &ownerName.String
		}
		if groupDBID.Valid {
			r.GroupDatabaseID = &groupDBID.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func sqlServerQuoteBracket(ident string) string {
	if ident == "" {
		return "[]"
	}
	return "[" + strings.ReplaceAll(ident, "]", "]]") + "]"
}
