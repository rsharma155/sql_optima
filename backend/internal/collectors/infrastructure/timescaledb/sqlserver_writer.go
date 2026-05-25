package timescaledb

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rsharma155/sql_optima/internal/collectors/domain"
)

func (w *TimescaleWriter) GetInstanceState(ctx context.Context, serverID uuid.UUID) (lastPoll time.Time, startTime time.Time, err error) {
	query := `
		SELECT last_poll_time_utc, sqlserver_start_time
		FROM sqlserver_collector_instance_state
		WHERE server_id = $1
	`
	err = w.pool.QueryRow(ctx, query, serverID).Scan(&lastPoll, &startTime)
	if err == pgx.ErrNoRows {
		return time.Time{}, time.Time{}, nil
	}
	return lastPoll, startTime, err
}

func (w *TimescaleWriter) SaveMetrics(ctx context.Context, serverID uuid.UUID, snapshots []domain.MSSQLQuerySnapshot, pollTime time.Time, sqlStartTime time.Time) error {
	if len(snapshots) == 0 {
		// Even if no snapshots, we should update the watermark to move forward
		watermarkSQL := `
INSERT INTO sqlserver_collector_instance_state (server_id, last_poll_time_utc, sqlserver_start_time, last_successful_run, last_error)
VALUES ($1, $2, $3, NOW(), NULL)
ON CONFLICT (server_id) DO UPDATE SET
    last_poll_time_utc = EXCLUDED.last_poll_time_utc,
    sqlserver_start_time = EXCLUDED.sqlserver_start_time,
    last_successful_run = EXCLUDED.last_successful_run,
    last_error = NULL;
`
		_, err := w.pool.Exec(ctx, watermarkSQL, serverID, pollTime, sqlStartTime)
		return err
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Bulk Insert into Staging
	// We use COPY for high performance if possible, but for simplicity here we use a batch insert or multiple inserts.
	// Given it's a small number of rows usually, batch is fine.
	batch := &pgx.Batch{}
	for _, s := range snapshots {
		// Convert binary(8) query_hash to int64 for Postgres BIGINT
		var qHash int64
		if len(s.QueryHash) >= 8 {
			qHash = int64(binary.BigEndian.Uint64(s.QueryHash))
		}

		batch.Queue(`INSERT INTO sqlserver_query_stats_staging_v2 (
			poll_time_utc, sqlserver_start_time, server_id, db_id, database_name,
			query_hash, plan_handle, query_text_raw, statement_text,
			total_worker_time, total_logical_reads, total_logical_writes,
			execution_count, total_rows, total_grant_kb,
			max_worker_time, max_logical_reads, max_dop, max_grant_kb, max_rows,
			last_execution_time, total_elapsed_ms, total_physical_reads
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)`,
			pollTime, sqlStartTime, serverID, s.DBID, s.DatabaseName,
			qHash, s.PlanHandle, s.QueryTextRaw, s.StatementText,
			s.TotalCPUMs, s.TotalLogicalReads, s.TotalLogicalWrites,
			s.TotalExecutions, s.TotalRows, s.TotalGrantKB,
			s.MaxWorkerTime, s.MaxLogicalReads, s.MaxDOP, s.MaxGrantKB, s.MaxRows,
			s.LastExecutionTime, s.TotalElapsedMs, s.TotalPhysicalReads,
		)
	}

	br := tx.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		return fmt.Errorf("send batch to staging: %w", err)
	}
	// 2a. Insert deltas into history table.
	// CTEs compute deltas between current staging rows and the previous snapshot.
	// Split into two separate Exec calls because pgx does not allow multiple commands
	// in a single prepared statement (SQLSTATE 42601).
	const deltaCTEs = `
WITH previous AS (
    SELECT * FROM sqlserver_query_stats_snapshot_v2 WHERE server_id = $1
),
aggregated_staging AS (
    SELECT
        server_id, db_id, query_hash, plan_handle,
        MAX(poll_time_utc)          AS poll_time_utc,
        MAX(sqlserver_start_time)   AS sqlserver_start_time,
        MAX(database_name)          AS database_name,
        MAX(query_text_raw)         AS query_text_raw,
        MAX(statement_text)         AS statement_text,
        SUM(total_worker_time)      AS total_worker_time,
        SUM(total_logical_reads)    AS total_logical_reads,
        SUM(total_logical_writes)   AS total_logical_writes,
        SUM(execution_count)        AS execution_count,
        SUM(total_rows)             AS total_rows,
        SUM(total_grant_kb)         AS total_grant_kb,
        SUM(total_elapsed_ms)       AS total_elapsed_ms,
        SUM(total_physical_reads)   AS total_physical_reads,
        MAX(max_worker_time)        AS max_worker_time,
        MAX(max_logical_reads)      AS max_logical_reads,
        MAX(max_dop)                AS max_dop,
        MAX(max_grant_kb)           AS max_grant_kb,
        MAX(max_rows)               AS max_rows,
        MAX(last_execution_time)    AS last_execution_time
    FROM sqlserver_query_stats_staging_v2
    WHERE server_id = $1
    GROUP BY server_id, db_id, query_hash, plan_handle
),
joined AS (
    SELECT
        s.*,
        p.last_total_worker_time,
        p.last_total_logical_reads,
        p.last_total_logical_writes,
        p.last_total_execution_count,
        p.last_total_rows,
        p.last_total_elapsed_ms,
        p.last_total_physical_reads
    FROM aggregated_staging s
    LEFT JOIN previous p
        ON p.server_id = s.server_id
        AND p.db_id    = s.db_id
        AND p.query_hash  = s.query_hash
        AND p.plan_handle = s.plan_handle
),
deltas AS (
    SELECT
        poll_time_utc AS ts,
        server_id, db_id, query_hash, plan_handle,
        CASE WHEN last_total_worker_time     IS NULL THEN total_worker_time     WHEN total_worker_time     < last_total_worker_time     THEN total_worker_time     ELSE total_worker_time     - last_total_worker_time     END AS cpu_delta_ms,
        CASE WHEN last_total_execution_count IS NULL THEN execution_count        WHEN execution_count        < last_total_execution_count THEN execution_count        ELSE execution_count        - last_total_execution_count END AS exec_delta,
        CASE WHEN last_total_logical_reads   IS NULL THEN total_logical_reads    WHEN total_logical_reads    < last_total_logical_reads   THEN total_logical_reads    ELSE total_logical_reads    - last_total_logical_reads   END AS reads_delta,
        CASE WHEN last_total_logical_writes  IS NULL THEN total_logical_writes   WHEN total_logical_writes   < last_total_logical_writes  THEN total_logical_writes   ELSE total_logical_writes   - last_total_logical_writes  END AS writes_delta,
        CASE WHEN last_total_rows            IS NULL THEN total_rows             WHEN total_rows             < last_total_rows            THEN total_rows             ELSE total_rows             - last_total_rows            END AS rows_delta,
        CASE WHEN last_total_elapsed_ms      IS NULL THEN total_elapsed_ms       WHEN total_elapsed_ms       < last_total_elapsed_ms      THEN total_elapsed_ms       ELSE total_elapsed_ms       - last_total_elapsed_ms      END AS elapsed_delta_ms,
        CASE WHEN last_total_physical_reads  IS NULL THEN total_physical_reads   WHEN total_physical_reads   < last_total_physical_reads  THEN total_physical_reads   ELSE total_physical_reads   - last_total_physical_reads  END AS physical_reads_delta,
        max_worker_time   AS period_max_cpu_ms,
        max_logical_reads AS period_max_reads,
        max_grant_kb      AS period_max_grant_kb,
        max_dop           AS period_max_dop,
        database_name, statement_text, query_text_raw, last_execution_time
    FROM joined
)
`
	mergeHistorySQL := deltaCTEs + `
INSERT INTO sqlserver_query_stats_history (
    capture_timestamp, server_id, db_id, query_hash,
    cpu_delta_ms, exec_delta, reads_delta, writes_delta, rows_delta,
    period_max_cpu_ms, period_max_reads, period_max_grant_kb, period_max_dop
)
SELECT ts, server_id, db_id, query_hash,
       cpu_delta_ms, exec_delta, reads_delta, writes_delta, rows_delta,
       period_max_cpu_ms, period_max_reads, period_max_grant_kb, period_max_dop
FROM deltas
WHERE exec_delta > 0 OR cpu_delta_ms > 0
`
	if _, err := tx.Exec(ctx, mergeHistorySQL, serverID); err != nil {
		return fmt.Errorf("merge deltas to history: %w", err)
	}

	// 2b. Populate enriched metrics table (separate statement — pgx forbids multi-command strings).
	mergeMetricsSQL := deltaCTEs + `
INSERT INTO sqlserver_query_metrics_v2 (
    capture_timestamp, server_id, database_name, login_name, application_name,
    query_hash, plan_handle, total_executions, total_cpu_ms, total_elapsed_ms,
    total_logical_reads, total_physical_reads, total_rows, statement_text,
    query_text_raw, last_execution_time, is_user_workload
)
SELECT
    d.ts, d.server_id, d.database_name,
    CASE
        WHEN COALESCE(d.query_text_raw, '') ILIKE '%/* SQL_OPTIMA%'
            THEN COALESCE(NULLIF(TRIM(e.login_name), ''), 'sql-optima')
        ELSE COALESCE(e.login_name, 'unknown')
    END,
    CASE
        WHEN COALESCE(d.query_text_raw, '') ILIKE '%/* SQL_OPTIMA%'
            THEN COALESCE(NULLIF(TRIM(e.application_name), ''), 'sql-optima')
        ELSE COALESCE(e.application_name, 'unknown')
    END,
    d.query_hash, d.plan_handle, d.exec_delta, d.cpu_delta_ms, d.elapsed_delta_ms,
    d.reads_delta, d.physical_reads_delta, d.rows_delta, d.statement_text,
    d.query_text_raw, d.last_execution_time,
    CASE
        WHEN COALESCE(d.query_text_raw, '') ILIKE '%/* SQL_OPTIMA%' THEN 0
        WHEN COALESCE(e.is_user_workload, 1) = 0 THEN 0
        ELSE 1
    END
FROM deltas d
LEFT JOIN sqlserver_plan_enrichment e
    ON e.server_id = d.server_id AND e.plan_handle = d.plan_handle
WHERE d.exec_delta > 0 OR d.cpu_delta_ms > 0 OR d.reads_delta > 0
`
	if _, err := tx.Exec(ctx, mergeMetricsSQL, serverID); err != nil {
		return fmt.Errorf("merge deltas to metrics_v2: %w", err)
	}
	// 3. Upsert Snapshot
	upsertSQL := `
INSERT INTO sqlserver_query_stats_snapshot_v2 (
server_id, db_id, query_hash, plan_handle,
database_name, query_text_raw, statement_text,
last_total_worker_time, last_total_logical_reads, last_total_logical_writes,
last_total_execution_count, last_total_rows, last_total_grant_kb,
last_total_elapsed_ms, last_total_physical_reads,
max_worker_time, max_logical_reads, max_dop, max_grant_kb, max_rows,
last_execution_time, last_seen_poll_time
)
SELECT
server_id, db_id, query_hash, plan_handle,
MAX(database_name), MAX(query_text_raw), MAX(statement_text),
SUM(total_worker_time), SUM(total_logical_reads), SUM(total_logical_writes),
SUM(execution_count), SUM(total_rows), SUM(total_grant_kb),
SUM(total_elapsed_ms), SUM(total_physical_reads),
MAX(max_worker_time), MAX(max_logical_reads), MAX(max_dop), MAX(max_grant_kb), MAX(max_rows),
MAX(last_execution_time), MAX(poll_time_utc)
FROM sqlserver_query_stats_staging_v2
WHERE server_id = $1
GROUP BY server_id, db_id, query_hash, plan_handle
ON CONFLICT (server_id, db_id, query_hash, plan_handle)
DO UPDATE SET
    database_name = EXCLUDED.database_name,
    query_text_raw = EXCLUDED.query_text_raw,
    statement_text = EXCLUDED.statement_text,
    last_total_worker_time = EXCLUDED.last_total_worker_time,
    last_total_logical_reads = EXCLUDED.last_total_logical_reads,
    last_total_logical_writes = EXCLUDED.last_total_logical_writes,
    last_total_execution_count = EXCLUDED.last_total_execution_count,
    last_total_rows = EXCLUDED.last_total_rows,
    last_total_grant_kb = EXCLUDED.last_total_grant_kb,
    last_total_elapsed_ms = EXCLUDED.last_total_elapsed_ms,
    last_total_physical_reads = EXCLUDED.last_total_physical_reads,
    last_execution_time = EXCLUDED.last_execution_time,
    last_seen_poll_time = EXCLUDED.last_seen_poll_time,
    max_worker_time = GREATEST(sqlserver_query_stats_snapshot_v2.max_worker_time, EXCLUDED.max_worker_time),
    max_logical_reads = GREATEST(sqlserver_query_stats_snapshot_v2.max_logical_reads, EXCLUDED.max_logical_reads),
    max_dop = GREATEST(sqlserver_query_stats_snapshot_v2.max_dop, EXCLUDED.max_dop),
    max_grant_kb = GREATEST(sqlserver_query_stats_snapshot_v2.max_grant_kb, EXCLUDED.max_grant_kb),
    max_rows = GREATEST(sqlserver_query_stats_snapshot_v2.max_rows, EXCLUDED.max_rows);
`
	if _, err := tx.Exec(ctx, upsertSQL, serverID); err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}

	// 4. Update Watermark
	watermarkSQL := `
INSERT INTO sqlserver_collector_instance_state (server_id, last_poll_time_utc, sqlserver_start_time, last_successful_run, last_error)
VALUES ($1, $2, $3, NOW(), NULL)
ON CONFLICT (server_id) DO UPDATE SET
    last_poll_time_utc = EXCLUDED.last_poll_time_utc,
    sqlserver_start_time = EXCLUDED.sqlserver_start_time,
    last_successful_run = EXCLUDED.last_successful_run,
    last_error = NULL;
`
	if _, err := tx.Exec(ctx, watermarkSQL, serverID, pollTime, sqlStartTime); err != nil {
		return fmt.Errorf("update watermark: %w", err)
	}

	// 5. Truncate Staging for this serverID
	if _, err := tx.Exec(ctx, "DELETE FROM sqlserver_query_stats_staging_v2 WHERE server_id = $1", serverID); err != nil {
		return fmt.Errorf("truncate staging: %w", err)
	}

	return tx.Commit(ctx)
}

func (w *TimescaleWriter) WriteMSSQLSessionEnrichment(ctx context.Context, serverID uuid.UUID, enrichments []domain.MSSQLSessionEnrichment) error {
	if len(enrichments) == 0 {
		return nil
	}

	const q = `
		INSERT INTO sqlserver_plan_enrichment (
			server_id, plan_handle, login_name, application_name, database_name, is_user_workload, last_seen
		) VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (server_id, plan_handle) DO UPDATE SET
			login_name = EXCLUDED.login_name,
			application_name = EXCLUDED.application_name,
			database_name = EXCLUDED.database_name,
			is_user_workload = EXCLUDED.is_user_workload,
			last_seen = NOW();
	`

	batch := &pgx.Batch{}
	for _, e := range enrichments {
		batch.Queue(q,
			serverID, e.PlanHandle, e.LoginName, e.ApplicationName, e.DatabaseName, e.IsUserWorkload,
		)
	}

	br := w.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range enrichments {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// ReadMSSQLPlanEnrichment returns enrichment rows for an serverID from
// sqlserver_plan_enrichment, scoped to the recent past so stale plan handles
// from long-evicted sessions don't pollute the join. The 30s
// session-enrichment collector keeps last_seen current for active plans.
//
// This is the read side that lets the 1-minute query-snapshot collector
// avoid re-querying sys.dm_exec_requests / dm_exec_sessions every cycle.
func (w *TimescaleWriter) ReadMSSQLPlanEnrichment(ctx context.Context, serverID uuid.UUID) ([]domain.MSSQLSessionEnrichment, error) {
	const query = `
		SELECT plan_handle, login_name, application_name, database_name, is_user_workload
		FROM sqlserver_plan_enrichment
		WHERE server_id = $1
		  AND last_seen >= NOW() - INTERVAL '10 minutes'
	`

	rows, err := w.pool.Query(ctx, query, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var enrichments []domain.MSSQLSessionEnrichment
	for rows.Next() {
		var e domain.MSSQLSessionEnrichment
		var login, app, db *string
		var isUser *int
		if err := rows.Scan(&e.PlanHandle, &login, &app, &db, &isUser); err != nil {
			return nil, err
		}
		if login != nil {
			e.LoginName = *login
		}
		if app != nil {
			e.ApplicationName = *app
		}
		if db != nil {
			e.DatabaseName = *db
		}
		if isUser != nil {
			e.IsUserWorkload = *isUser
		}
		enrichments = append(enrichments, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return enrichments, nil
}
