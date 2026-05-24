// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : Live DMV fetching logic for SQL Server HA & Replication collectors.
//           This logic is strictly for data collection and is not accessible 
//           to dashboard handlers to ensure all UI data comes from TimescaleDB.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package collectors

import (
	"context"
	dbsql "database/sql"
	"fmt"
	"strings"

	"github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/domain"
	"github.com/rsharma155/sql_optima/internal/repository"
)

// SourceQuerier provides live DMV access via the core SqlServerRepository.
type SourceQuerier struct {
	msRepo *repository.SqlServerRepository
}

// NewSourceQuerier constructs the live collector querier.
func NewSourceQuerier(msRepo *repository.SqlServerRepository) *SourceQuerier {
	return &SourceQuerier{msRepo: msRepo}
}

// FetchFeatureDetection runs the detection query directly on the monitored SQL Server.
func (r *SourceQuerier) FetchFeatureDetection(ctx context.Context, instanceName string) (domain.FeatureDetection, error) {
	const sql = `
		/* SQL_OPTIMA — feature_detection_live */
		SET NOCOUNT ON;
		DECLARE @ag BIT = 0, @fci BIT = CAST(SERVERPROPERTY('IsClustered') AS BIT), @ls BIT = 0, @mir BIT = 0, @repl BIT = 0;

		-- Always On Availability Groups detection
		IF CAST(SERVERPROPERTY('IsHadrEnabled') AS BIT) = 1
		BEGIN
			-- If HADR is enabled at the instance level, we consider HA active.
			-- We also check for actual AGs or replica states to be sure.
			IF OBJECT_ID('sys.availability_groups') IS NOT NULL
			BEGIN
				IF EXISTS (SELECT 1 FROM sys.availability_groups) SET @ag = 1;
			END
			
			-- Fallback: if IsHadrEnabled is 1, but sys.availability_groups is empty,
			-- we might be on a secondary or in a state where AGs aren't visible yet.
			-- Setting @ag = 1 here ensures the dashboard link is visible if the feature is ON.
			IF @ag = 0 SET @ag = 1; 
		END

		IF OBJECT_ID('msdb.dbo.log_shipping_primary_databases') IS NOT NULL AND HAS_PERMS_BY_NAME('msdb.dbo.log_shipping_primary_databases', 'OBJECT', 'SELECT') = 1
		BEGIN
			EXEC sp_executesql N'IF EXISTS (SELECT 1 FROM msdb.dbo.log_shipping_primary_databases) SET @ls = 1', N'@ls BIT OUTPUT', @ls = @ls OUTPUT;
		END
		
		IF @ls = 0 AND OBJECT_ID('msdb.dbo.log_shipping_secondary_databases') IS NOT NULL AND HAS_PERMS_BY_NAME('msdb.dbo.log_shipping_secondary_databases', 'OBJECT', 'SELECT') = 1
		BEGIN
			EXEC sp_executesql N'IF EXISTS (SELECT 1 FROM msdb.dbo.log_shipping_secondary_databases) SET @ls = 1', N'@ls BIT OUTPUT', @ls = @ls OUTPUT;
		END

		IF EXISTS (SELECT 1 FROM sys.database_mirroring WHERE mirroring_state IS NOT NULL) SET @mir = 1;

		-- Replication detection: check if published, subscribed, or is a distributor
		IF EXISTS (SELECT 1 FROM sys.databases WHERE is_published = 1 OR is_subscribed = 1 OR is_merge_published = 1 OR is_distributor = 1) 
		   OR CAST(SERVERPROPERTY('IsDistributor') AS BIT) = 1
		   OR EXISTS (SELECT 1 FROM sys.servers WHERE server_id = 0 AND (is_publisher = 1 OR is_subscriber = 1 OR is_distributor = 1))
		BEGIN
			SET @repl = 1;
		END

		-- Detect replication types if possible
		DECLARE @repl_types TABLE (ptype INT);
		IF EXISTS (SELECT 1 FROM sys.databases WHERE name = 'distribution' OR is_distributor = 1)
		BEGIN
			-- 1 = Transactional, 2 = Snapshot, 3 = Merge
			BEGIN TRY
				-- We try to find the distribution database name dynamically
				DECLARE @dist_db NVARCHAR(256) = (SELECT TOP 1 name FROM sys.databases WHERE is_distributor = 1);
				IF @dist_db IS NULL SET @dist_db = 'distribution';

				DECLARE @dist_sql NVARCHAR(MAX) = N'SELECT DISTINCT publication_type FROM ' + QUOTENAME(@dist_db) + N'.dbo.MSpublications';
				INSERT INTO @repl_types (ptype)
				EXEC sp_executesql @dist_sql;
			END TRY BEGIN CATCH END CATCH
		END

		SELECT 
			GETUTCDATE() AS capture_timestamp, @ag, @fci, @ls, @mir, @repl,
			(SELECT CAST(ptype AS VARCHAR) + ',' FROM @repl_types FOR XML PATH('')) AS types;`

	db, ok := r.msRepo.GetConn(instanceName)
	if !ok {
		return domain.FeatureDetection{}, fmt.Errorf("instance not found: %s", instanceName)
	}

	var f domain.FeatureDetection
	var ag, fci, ls, mir, repl bool
	var typeStr *string
	err := db.QueryRowContext(ctx, sql).Scan(
		&f.CaptureTimestamp, &ag, &fci, &ls, &mir, &repl, &typeStr,
	)
	if err != nil {
		return domain.FeatureDetection{}, err
	}

	f.AGEnabled = ag
	f.FCIEnabled = fci
	f.LogShippingEnabled = ls
	f.MirroringEnabled = mir
	f.ReplicationEnabled = repl
	f.HAEnabled = ag || fci || ls || mir
	
	f.ReplicationTypes = []string{}
	if typeStr != nil && *typeStr != "" {
		for _, s := range strings.Split(strings.TrimSuffix(*typeStr, ","), ",") {
			switch s {
			case "0": f.ReplicationTypes = append(f.ReplicationTypes, "Transactional")
			case "1": f.ReplicationTypes = append(f.ReplicationTypes, "Snapshot")
			case "2": f.ReplicationTypes = append(f.ReplicationTypes, "Merge")
			}
		}
	}

	return f, nil
}

// FetchReplicaHealth queries sys.dm_hadr_availability_replica_states directly.
func (r *SourceQuerier) FetchReplicaHealth(ctx context.Context, instanceName string) ([]domain.ReplicaHealthRow, error) {
	const sql = `
		/* SQL_OPTIMA — ha_replica_state_live */
		SELECT
			ag.name                                                          AS ag_name,
			ar.replica_server_name,
			ISNULL(ars.role_desc, 'RESOLVING')                               AS role_desc,
			ISNULL(MAX(drs.synchronization_state_desc), 'NOT SYNCHRONIZING') AS synchronization_state_desc,
			ISNULL(ars.synchronization_health_desc, 'NOT_HEALTHY')           AS synchronization_health_desc,
			ar.availability_mode_desc,
			ISNULL(SUM(CAST(drs.log_send_queue_size AS BIGINT)), 0)           AS log_send_queue_kb,
			ISNULL(SUM(CAST(drs.redo_queue_size AS BIGINT)), 0)              AS redo_queue_kb,
			ISNULL(AVG(CAST(drs.log_send_rate AS BIGINT)), 0)                AS log_send_rate_kbps,
			ISNULL(AVG(CAST(drs.redo_rate AS BIGINT)), 0)                    AS redo_rate_kbps,
			MAX(drs.last_commit_time)                                        AS last_commit_time,
			ISNULL(MAX(drs.secondary_lag_seconds), 0)                        AS secondary_lag_seconds,
			ISNULL(ars.connected_state_desc, 'DISCONNECTED')                 AS connected_state_desc,
			CASE WHEN ars.synchronization_health_desc = 'HEALTHY'
					  AND ars.connected_state_desc = 'CONNECTED'
					  AND ars.role_desc = 'SECONDARY'
				 THEN CAST(1 AS BIT) ELSE CAST(0 AS BIT) END                AS is_failover_ready
		FROM sys.availability_groups ag
		JOIN sys.availability_replicas ar
			ON ag.group_id = ar.group_id
		LEFT JOIN sys.dm_hadr_availability_replica_states ars
			ON ar.replica_id = ars.replica_id
		LEFT JOIN sys.dm_hadr_database_replica_states drs
			ON ar.replica_id = drs.replica_id
		GROUP BY 
			ag.name, ar.replica_server_name, ars.role_desc, 
			ars.synchronization_health_desc, ar.availability_mode_desc, 
			ars.connected_state_desc
		ORDER BY ag.name, ar.replica_server_name;`

	db, ok := r.msRepo.GetConn(instanceName)
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceName)
	}

	rows, err := db.QueryContext(ctx, sql)
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
			&row.LastCommitTime,
			&row.SecondaryLagSeconds, &row.ConnectedStateDesc, &row.IsFailoverReady,
		); err != nil {
			continue
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// FetchDatabaseSyncState queries sys.dm_hadr_database_replica_states directly.
func (r *SourceQuerier) FetchDatabaseSyncState(ctx context.Context, instanceName string) ([]domain.DatabaseSyncState, error) {
	const sql = `
		/* SQL_OPTIMA — ha_database_state_live */
		SELECT
			ag.name AS ag_name,
			db_name(drs.database_id) AS database_name,
			ar.replica_server_name,
			drs.synchronization_state_desc,
			drs.is_suspended,
			ISNULL(drs.log_send_queue_size, 0) AS log_send_queue_kb,
			ISNULL(drs.redo_queue_size, 0) AS redo_queue_kb,
			drs.last_commit_time
		FROM sys.dm_hadr_database_replica_states drs
		JOIN sys.availability_groups ag ON drs.group_id = ag.group_id
		JOIN sys.availability_replicas ar ON drs.replica_id = ar.replica_id
		ORDER BY ag.name, database_name, ar.replica_server_name;`

	db, ok := r.msRepo.GetConn(instanceName)
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceName)
	}

	rows, err := db.QueryContext(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.DatabaseSyncState
	for rows.Next() {
		var row domain.DatabaseSyncState
		if err := rows.Scan(
			&row.AGName, &row.DatabaseName, &row.ReplicaServerName,
			&row.SynchronizationStateDesc, &row.IsSuspended,
			&row.LogSendQueueKB, &row.RedoQueueKB, &row.LastCommitTime,
		); err != nil {
			continue
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// FetchFailoverReadinessMetrics fetches additional checklist metrics.
func (r *SourceQuerier) FetchFailoverReadinessMetrics(ctx context.Context, instanceName string) (int, string, error) {
	const sql = `
		/* SQL_OPTIMA — failover_readiness_extra */
		SELECT 
			(SELECT COUNT(*) FROM sys.dm_exec_requests WHERE total_elapsed_time > 60000 AND blocking_session_id = 0) AS long_running_tx,
			(SELECT TOP 1 quorum_state_desc FROM sys.dm_hadr_cluster) AS quorum_state;`

	db, ok := r.msRepo.GetConn(instanceName)
	if !ok {
		return 0, "", fmt.Errorf("instance not found: %s", instanceName)
	}

	var count int
	var state string
	err := db.QueryRowContext(ctx, sql).Scan(&count, &state)
	return count, state, err
}

// FetchBackupFreshness checks if databases have a full backup within the last 24 hours.
func (r *SourceQuerier) FetchBackupFreshness(ctx context.Context, instanceName string) (map[string]bool, error) {
	const sql = `
		/* SQL_OPTIMA — backup_freshness_live */
		SELECT 
			database_name, 
			CASE WHEN MAX(backup_finish_date) > DATEADD(hour, -24, GETUTCDATE()) THEN 1 ELSE 0 END AS is_fresh
		FROM msdb.dbo.backupset
		WHERE type = 'D'
		GROUP BY database_name;`

	db, ok := r.msRepo.GetConn(instanceName)
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceName)
	}

	rows, err := db.QueryContext(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var name string
		var fresh bool
		if err := rows.Scan(&name, &fresh); err == nil {
			result[name] = fresh
		}
	}
	return result, rows.Err()
}

// FetchReplicationTopology fetches replication topology. Falls back to local metadata if distribution DB is missing.
func (r *SourceQuerier) FetchReplicationTopology(ctx context.Context, instanceName string) ([]domain.ReplicationTopologyRow, error) {
	const sql = `
		/* SQL_OPTIMA — replication_topology_live */
		SET NOCOUNT ON;
		DECLARE @dist_db NVARCHAR(256) = (SELECT TOP 1 name FROM sys.databases WHERE is_distributor = 1);
		IF @dist_db IS NULL AND EXISTS (SELECT 1 FROM sys.databases WHERE name = 'distribution') SET @dist_db = 'distribution';

		IF @dist_db IS NOT NULL
		BEGIN
			DECLARE @q NVARCHAR(MAX) = N'
			SELECT
				ISNULL(mss2.srvname, @@SERVERNAME)              AS publisher,
				ISNULL(mss.srvname,  '''')                        AS subscriber,
				ISNULL(da.publication, '''')                      AS publication,
				ISNULL(da.publisher_db, '''')                     AS publication_db,
				ISNULL(da.subscriber_db, '''')                    AS subscriber_db,
				CASE mp.publication_type
					WHEN 0 THEN ''Transactional''
					WHEN 1 THEN ''Snapshot''
					WHEN 2 THEN ''Merge''
					ELSE ''Unknown''
				END                                             AS replication_type,
				''Continuous''                                    AS sync_type,
				ISNULL(ag.agent_status, ''Unknown'')              AS agent_status,
				ag.last_start_time                              AS last_sync_time
			FROM ' + QUOTENAME(@dist_db) + N'.dbo.MSdistribution_agents da
			LEFT JOIN ' + QUOTENAME(@dist_db) + N'.dbo.MSpublications mp 
				ON mp.publisher_db = da.publisher_db 
				AND mp.publication = da.publication
			LEFT JOIN master.sys.sysservers mss  ON mss.srvid  = da.subscriber_id
			LEFT JOIN master.sys.sysservers mss2 ON mss2.srvid = da.publisher_id
			OUTER APPLY (
				SELECT TOP 1
					CASE h.runstatus
						WHEN 1 THEN ''Idle''
						WHEN 2 THEN ''Running''
						WHEN 3 THEN ''Error''
						WHEN 4 THEN ''Idle''
						WHEN 5 THEN ''Retrying''
						WHEN 6 THEN ''Failed''
						ELSE ''Unknown''
					END AS agent_status,
					h.start_time AS last_start_time
				FROM ' + QUOTENAME(@dist_db) + N'.dbo.MSdistribution_history h 
				WHERE h.agent_id = da.id
				ORDER BY h.timestamp DESC
			) ag
			WHERE da.subscriber_id >= 0';

			IF OBJECT_ID(QUOTENAME(@dist_db) + '.dbo.MSmerge_agents') IS NOT NULL
			   AND HAS_PERMS_BY_NAME(QUOTENAME(@dist_db) + '.dbo.MSmerge_sessions', 'OBJECT', 'SELECT') = 1
			BEGIN
				SET @q = @q + N' UNION ALL
				SELECT
					ISNULL(mss2.srvname, @@SERVERNAME)              AS publisher,
					ISNULL(mss.srvname,  '''')                        AS subscriber,
					ISNULL(ma.publication, '''')                      AS publication,
					ISNULL(ma.publisher_db, '''')                     AS publication_db,
					ISNULL(ma.subscriber_db, '''')                    AS subscriber_db,
					''Merge''                                         AS replication_type,
					''Continuous''                                    AS sync_type,
					ISNULL(mh.agent_status, ''Unknown'')              AS agent_status,
					mh.last_start_time                              AS last_sync_time
				FROM ' + QUOTENAME(@dist_db) + N'.dbo.MSmerge_agents ma
				LEFT JOIN master.sys.sysservers mss  ON mss.srvid  = ma.subscriber_id
				LEFT JOIN master.sys.sysservers mss2 ON mss2.srvid = ma.publisher_id
				OUTER APPLY (
					SELECT TOP 1
						CASE h.runstatus
							WHEN 1 THEN ''Idle''
							WHEN 2 THEN ''Running''
							WHEN 3 THEN ''Error''
							WHEN 4 THEN ''Idle''
							WHEN 5 THEN ''Retrying''
							WHEN 6 THEN ''Failed''
							ELSE ''Unknown''
						END AS agent_status,
						h.start_time AS last_start_time
					FROM ' + QUOTENAME(@dist_db) + N'.dbo.MSmerge_sessions h 
					WHERE h.agent_id = ma.id
					ORDER BY h.start_time DESC
				) mh
				WHERE ma.subscriber_id >= 0';
			END
			EXEC sp_executesql @q;
		END
		ELSE
		BEGIN
			-- Fallback: use local sys.publications + sys.subscriptions (valid when connected as publisher).
			SELECT
			        @@SERVERNAME AS publisher,
			        ISNULL(srv.name, s.subscriber_server) AS subscriber,
			        p.name AS publication,
			        DB_NAME() AS publication_db,
			        ISNULL(s.db_name, '') AS subscriber_db,
			        CASE p.repl_freq
			                WHEN 0 THEN 'Snapshot'
			                WHEN 1 THEN 'Transactional'
			                ELSE 'Unknown'
			        END AS replication_type,
			        CASE s.subscription_type
			                WHEN 0 THEN 'Push'
			                WHEN 1 THEN 'Pull'
			                ELSE 'Unknown'
			        END AS sync_type,
			        'Active'  AS agent_status,
			        NULL AS last_sync_time
			FROM sys.publications p
			LEFT JOIN sys.subscriptions s ON p.pubid = s.pubid
			LEFT JOIN sys.servers srv ON s.srvid = srv.server_id
			UNION ALL
			SELECT
			        @@SERVERNAME AS publisher,
			        ISNULL(srv.name, s.subscriber_server) AS subscriber,
			        p.name AS publication,
			        DB_NAME() AS publication_db,
			        ISNULL(s.db_name, '') AS subscriber_db,
			        'Merge' AS replication_type,
			        CASE s.subscription_type
			                WHEN 0 THEN 'Push'
			                WHEN 1 THEN 'Pull'
			                ELSE 'Unknown'
			        END AS sync_type,
			        'Active'  AS agent_status,
			        NULL AS last_sync_time
			FROM sys.mergepublications p
			LEFT JOIN sys.mergesubscriptions s ON p.pubid = s.pubid
			LEFT JOIN sys.servers srv ON s.srvid = srv.server_id;
		END`

	db, ok := r.msRepo.GetConn(instanceName)
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceName)
	}

	rows, err := db.QueryContext(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.ReplicationTopologyRow
	for rows.Next() {
		var row domain.ReplicationTopologyRow
		var lastSync dbsql.NullTime
		if err := rows.Scan(
			&row.Publisher, &row.Subscriber, &row.Publication,
			&row.PublicationDB, &row.SubscriberDB, &row.ReplicationType,
			&row.SyncType, &row.AgentStatus, &lastSync,
		); err != nil {
			continue
		}
		if lastSync.Valid {
			row.LastSyncTime = lastSync.Time
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// FetchReplicationLatency fetches replication performance metrics.
func (r *SourceQuerier) FetchReplicationLatency(ctx context.Context, instanceName string) ([]domain.ReplicationLatencyPoint, error) {
	const sql = `
		/* SQL_OPTIMA — replication_latency_live */
		SET NOCOUNT ON;
		DECLARE @dist_db NVARCHAR(256) = (SELECT TOP 1 name FROM sys.databases WHERE is_distributor = 1);
		IF @dist_db IS NULL AND EXISTS (SELECT 1 FROM sys.databases WHERE name = 'distribution') SET @dist_db = 'distribution';

		IF @dist_db IS NOT NULL
		BEGIN
			DECLARE @q NVARCHAR(MAX) = N'
			SELECT
				ISNULL(mss2.srvname, @@SERVERNAME)              AS publisher,
				ISNULL(mss.srvname, '''')                         AS subscriber,
				ISNULL(mda.publication, '''')                     AS publication,
				ISNULL(mda_hist.delivery_latency / 1000, 0)     AS latency_seconds,
				ISNULL(mds.UndelivCmdsInDistDB, 0)              AS undistributed_commands,
				ISNULL(mda_hist.delivery_rate, 0)               AS delivery_rate_cmds_sec,
				CASE mda_hist.runstatus
					WHEN 1 THEN ''Idle''
					WHEN 2 THEN ''Running''
					WHEN 3 THEN ''Error''
					WHEN 4 THEN ''Idle''
					WHEN 5 THEN ''Retrying''
					WHEN 6 THEN ''Failed''
					ELSE ''Unknown''
				END                                             AS status
			FROM ' + QUOTENAME(@dist_db) + N'.dbo.MSdistribution_agents mda
			OUTER APPLY (
				SELECT TOP 1 h.delivery_latency, h.delivery_rate, h.runstatus
				FROM ' + QUOTENAME(@dist_db) + N'.dbo.MSdistribution_history h
				WHERE h.agent_id = mda.id
				ORDER BY h.timestamp DESC
			) mda_hist
			LEFT JOIN ' + QUOTENAME(@dist_db) + N'.dbo.MSdistribution_status mds
				ON mds.agent_id = mda.id
			LEFT JOIN master.sys.sysservers mss
				ON mss.srvid = mda.subscriber_id
			LEFT JOIN master.sys.sysservers mss2
				ON mss2.srvid = mda.publisher_id';

			IF OBJECT_ID(QUOTENAME(@dist_db) + '.dbo.MSmerge_agents') IS NOT NULL
			   AND HAS_PERMS_BY_NAME(QUOTENAME(@dist_db) + '.dbo.MSmerge_sessions', 'OBJECT', 'SELECT') = 1
			BEGIN
				SET @q = @q + N' UNION ALL
				SELECT
					ISNULL(mss2.srvname, @@SERVERNAME)              AS publisher,
					ISNULL(mss.srvname, '''')                         AS subscriber,
					ISNULL(ma.publication, '''')                      AS publication,
					0                                               AS latency_seconds,
					0                                               AS undistributed_commands,
					ISNULL(mh.delivery_rate, 0)                      AS delivery_rate_cmds_sec,
					CASE mh.runstatus
						WHEN 1 THEN ''Idle''
						WHEN 2 THEN ''Running''
						WHEN 3 THEN ''Error''
						WHEN 4 THEN ''Idle''
						WHEN 5 THEN ''Retrying''
						WHEN 6 THEN ''Failed''
						ELSE ''Unknown''
					END                                             AS status
				FROM ' + QUOTENAME(@dist_db) + N'.dbo.MSmerge_agents ma
				OUTER APPLY (
					SELECT TOP 1 h.delivery_rate, h.runstatus
					FROM ' + QUOTENAME(@dist_db) + N'.dbo.MSmerge_sessions h
					WHERE h.agent_id = ma.id
					ORDER BY h.start_time DESC
				) mh
				LEFT JOIN master.sys.sysservers mss
					ON mss.srvid = ma.subscriber_id
				LEFT JOIN master.sys.sysservers mss2
					ON mss2.srvid = ma.publisher_id';
			END
			EXEC sp_executesql @q;
		END
		ELSE
		BEGIN
			-- If we are on a Publisher but don't have access to the Distributor, we return basic rows
			-- so the charts at least show placeholders or last known status.
			SELECT 
				@@SERVERNAME AS publisher, 
				ISNULL(srv.name, s.subscriber_server) AS subscriber, 
				p.name AS publication, 
				0 AS latency_seconds, 
				0 AS undistributed_commands, 
				0 AS delivery_rate_cmds_sec, 
				'Active' AS status
			FROM sys.publications p
			LEFT JOIN sys.subscriptions s ON p.pubid = s.pubid
			LEFT JOIN sys.servers srv ON s.srvid = srv.server_id
			UNION ALL
			SELECT 
				@@SERVERNAME AS publisher, 
				ISNULL(srv.name, s.subscriber_server) AS subscriber, 
				p.name AS publication, 
				0 AS latency_seconds, 
				0 AS undistributed_commands, 
				0 AS delivery_rate_cmds_sec, 
				'Active' AS status
			FROM sys.mergepublications p
			LEFT JOIN sys.mergesubscriptions s ON p.pubid = s.pubid
			LEFT JOIN sys.servers srv ON s.srvid = srv.server_id;
		END`

	db, ok := r.msRepo.GetConn(instanceName)
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceName)
	}

	rows, err := db.QueryContext(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.ReplicationLatencyPoint
	for rows.Next() {
		var row domain.ReplicationLatencyPoint
		if err := rows.Scan(
			&row.Publisher, &row.Subscriber, &row.Publication,
			&row.MaxLatencySeconds, &row.PeakBacklog, &row.DeliveryRateCmdsSec, &row.Status,
		); err != nil {
			continue
		}
		row.AvgBacklog = float64(row.PeakBacklog)
		result = append(result, row)
	}
	return result, rows.Err()
}

// FetchReplicationArticles fetches table-level replication stats.
func (r *SourceQuerier) FetchReplicationArticles(ctx context.Context, instanceName string) ([]domain.ReplicationArticle, error) {
	const sql = `
		/* SQL_OPTIMA — replication_articles_live */
		SET NOCOUNT ON;
		DECLARE @dist_db NVARCHAR(256) = (SELECT TOP 1 name FROM sys.databases WHERE is_distributor = 1);
		IF @dist_db IS NULL AND EXISTS (SELECT 1 FROM sys.databases WHERE name = 'distribution') SET @dist_db = 'distribution';

		IF @dist_db IS NOT NULL
		BEGIN
			DECLARE @q NVARCHAR(MAX) = N'
			-- Transactional/Snapshot articles
			SELECT
				p.publication                       AS publication,
				ma.publisher_db                     AS database_name,
				ma.source_owner                     AS schema_name,
				ma.source_object                    AS table_name,
				ISNULL(s.srvname, ''No Subscriber'')  AS subscriber,
				ISNULL(da_hist.delivery_rate, 0)    AS rows_per_sec,
				ISNULL(da_hist.delivery_latency / 1000, 0) AS latency_seconds,
				0                                   AS conflicts_detected,
				CASE da_hist.runstatus
					WHEN 1 THEN ''Starting'' WHEN 2 THEN ''Succeeded''
					WHEN 3 THEN ''Active''   WHEN 4 THEN ''Idle''
					WHEN 5 THEN ''Retry''    WHEN 6 THEN ''Failed''
					ELSE ''Unknown''
				END AS status
			FROM ' + QUOTENAME(@dist_db) + N'.dbo.MSpublications p
			JOIN ' + QUOTENAME(@dist_db) + N'.dbo.MSarticles ma 
				ON ma.publisher_id = p.publisher_id 
				AND ma.publisher_db = p.publisher_db
				AND ma.publication_id = p.publication_id
			LEFT JOIN ' + QUOTENAME(@dist_db) + N'.dbo.MSdistribution_agents da
				ON p.publisher_id = da.publisher_id
				AND p.publisher_db = da.publisher_db
				AND p.publication = da.publication
			LEFT JOIN master.sys.sysservers s ON s.srvid = da.subscriber_id
			OUTER APPLY (
				SELECT TOP 1 h.delivery_rate, h.delivery_latency, h.runstatus
				FROM ' + QUOTENAME(@dist_db) + N'.dbo.MSdistribution_history h 
				WHERE h.agent_id = da.id
				ORDER BY h.timestamp DESC
			) da_hist';

			IF OBJECT_ID(QUOTENAME(@dist_db) + '.dbo.MSmerge_articles') IS NOT NULL
			   AND HAS_PERMS_BY_NAME(QUOTENAME(@dist_db) + '.dbo.MSmerge_sessions', 'OBJECT', 'SELECT') = 1
			BEGIN
				SET @q = @q + N' UNION ALL
				-- Merge articles
				SELECT
					p.publication                       AS publication,
					ma.publisher_db                     AS database_name,
					ma.source_owner                     AS schema_name,
					ma.source_object                    AS table_name,
					ISNULL(s.srvname, ''No Subscriber'')  AS subscriber,
					ISNULL(mh.delivery_rate, 0)         AS rows_per_sec,
					0                                   AS latency_seconds,
					ISNULL(mh.conflicts, 0)             AS conflicts_detected,
					CASE mh.runstatus
						WHEN 1 THEN ''Starting'' WHEN 2 THEN ''Succeeded''
						WHEN 3 THEN ''Active''   WHEN 4 THEN ''Idle''
						WHEN 5 THEN ''Retry''    WHEN 6 THEN ''Failed''
						ELSE ''Unknown''
					END AS status
				FROM ' + QUOTENAME(@dist_db) + N'.dbo.MSpublications p
				JOIN ' + QUOTENAME(@dist_db) + N'.dbo.MSmerge_articles ma 
					ON ma.publisher_id = p.publisher_id 
					AND ma.publisher_db = p.publisher_db
					AND ma.publication_id = p.publication_id
				LEFT JOIN ' + QUOTENAME(@dist_db) + N'.dbo.MSmerge_agents da
					ON p.publisher_id = da.publisher_id
					AND p.publisher_db = da.publisher_db
					AND p.publication = da.publication
				LEFT JOIN master.sys.sysservers s ON s.srvid = da.subscriber_id
				OUTER APPLY (
					SELECT TOP 1 
						h.delivery_rate, 
						h.runstatus,
						(h.upload_conflicts + h.download_conflicts) AS conflicts
					FROM ' + QUOTENAME(@dist_db) + N'.dbo.MSmerge_sessions h 
					WHERE h.agent_id = da.id
					ORDER BY h.start_time DESC
				) mh';
			END
			EXEC sp_executesql @q;
		END
		ELSE
		BEGIN
			-- Fallback for Publisher side: list articles from sys.articles/sys.mergearticles
			SELECT 
				p.name AS publication,
				DB_NAME() AS database_name,
				SCHEMA_NAME(o.schema_id) AS schema_name,
				o.name AS table_name,
				ISNULL(srv.name, s.subscriber_server) AS subscriber,
				0 AS rows_per_sec,
				0 AS latency_seconds,
				0 AS conflicts_detected,
				'Active' AS status
			FROM sys.articles a
			JOIN sys.publications p ON a.pubid = p.pubid
			JOIN sys.objects o ON a.objid = o.object_id
			LEFT JOIN sys.subscriptions s ON p.pubid = s.pubid AND a.artid = s.artid
			LEFT JOIN sys.servers srv ON srv.server_id = s.srvid
			UNION ALL
			SELECT 
				p.name AS publication,
				DB_NAME() AS database_name,
				SCHEMA_NAME(o.schema_id) AS schema_name,
				o.name AS table_name,
				ISNULL(srv.name, s.subscriber_server) AS subscriber,
				0 AS rows_per_sec,
				0 AS latency_seconds,
				0 AS conflicts_detected,
				'Active' AS status
			FROM sys.mergearticles a
			JOIN sys.mergepublications p ON a.pubid = p.pubid
			JOIN sys.objects o ON a.objid = o.objid
			LEFT JOIN sys.mergesubscriptions s ON p.pubid = s.pubid
			LEFT JOIN sys.servers srv ON srv.server_id = s.srvid;
		END`

	db, ok := r.msRepo.GetConn(instanceName)
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceName)
	}

	rows, err := db.QueryContext(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.ReplicationArticle
	for rows.Next() {
		var row domain.ReplicationArticle
		if err := rows.Scan(
			&row.Publication, &row.DatabaseName, &row.SchemaName, &row.TableName, &row.Subscriber,
			&row.RowsPerSec, &row.LatencySeconds, &row.ConflictsDetected, &row.Status,
		); err != nil {
			continue
		}
		result = append(result, row)
	}
	return result, rows.Err()
}


