package timescaledb

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rsharma155/sql_optima/internal/collectors/domain"
)

func (w *TimescaleWriter) GetInstanceState(ctx context.Context, instanceID string) (lastPoll time.Time, startTime time.Time, err error) {
	query := `
		SELECT last_poll_time_utc, sqlserver_start_time
		FROM sqlserver_collector_instance_state
		WHERE instance_id = $1
	`
	err = w.pool.QueryRow(ctx, query, instanceID).Scan(&lastPoll, &startTime)
	if err == pgx.ErrNoRows {
		return time.Time{}, time.Time{}, nil
	}
	return lastPoll, startTime, err
}

func (w *TimescaleWriter) SaveMetrics(ctx context.Context, instanceID string, snapshots []domain.MSSQLQuerySnapshot, pollTime time.Time, sqlStartTime time.Time) error {
	if len(snapshots) == 0 {
		// Even if no snapshots, we should update the watermark to move forward
		watermarkSQL := `
INSERT INTO sqlserver_collector_instance_state (instance_id, last_poll_time_utc, sqlserver_start_time, last_successful_run)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (instance_id) DO UPDATE SET
    last_poll_time_utc = EXCLUDED.last_poll_time_utc,
    sqlserver_start_time = EXCLUDED.sqlserver_start_time,
    last_successful_run = EXCLUDED.last_successful_run;
`
		_, err := w.pool.Exec(ctx, watermarkSQL, instanceID, pollTime, sqlStartTime)
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
			poll_time_utc, sqlserver_start_time, instance_id, db_id, database_name,
			query_hash, plan_handle, query_text_raw, statement_text,
			total_worker_time, total_logical_reads, total_logical_writes,
			execution_count, total_rows, total_grant_kb,
			max_worker_time, max_logical_reads, max_dop, max_grant_kb, max_rows,
			last_execution_time
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)`,
			pollTime, sqlStartTime, instanceID, s.DBID, s.DatabaseName,
			qHash, s.PlanHandle, s.QueryTextRaw, s.StatementText,
			s.TotalCPUMs, s.TotalLogicalReads, s.TotalLogicalWrites,
			s.TotalExecutions, s.TotalRows, s.TotalGrantKB,
			s.MaxWorkerTime, s.MaxLogicalReads, s.MaxDOP, s.MaxGrantKB, s.MaxRows,
			s.LastExecutionTime,
		)
	}

	br := tx.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		return fmt.Errorf("send batch to staging: %w", err)
	}
// 2. Production Merge Statement
mergeSQL := `
WITH previous AS (
SELECT *
FROM sqlserver_query_stats_snapshot_v2
WHERE instance_id = $1
),
aggregated_staging AS (
SELECT
instance_id, db_id, query_hash, plan_handle,
MAX(poll_time_utc) as poll_time_utc,
MAX(sqlserver_start_time) as sqlserver_start_time,
MAX(database_name) as database_name,
MAX(query_text_raw) as query_text_raw,
MAX(statement_text) as statement_text,
SUM(total_worker_time) as total_worker_time,
SUM(total_logical_reads) as total_logical_reads,
SUM(total_logical_writes) as total_logical_writes,
SUM(execution_count) as execution_count,
SUM(total_rows) as total_rows,
SUM(total_grant_kb) as total_grant_kb,
MAX(max_worker_time) as max_worker_time,
MAX(max_logical_reads) as max_logical_reads,
MAX(max_dop) as max_dop,
MAX(max_grant_kb) as max_grant_kb,
MAX(max_rows) as max_rows,
MAX(last_execution_time) as last_execution_time
FROM sqlserver_query_stats_staging_v2
WHERE instance_id = $1
GROUP BY instance_id, db_id, query_hash, plan_handle
),
joined AS (
SELECT
s.*,
p.last_total_worker_time,
p.last_total_logical_reads,
p.last_total_logical_writes,
p.last_total_execution_count,
p.last_total_rows
FROM aggregated_staging s
LEFT JOIN previous p
ON p.instance_id = s.instance_id
AND p.db_id = s.db_id
AND p.query_hash = s.query_hash
AND p.plan_handle = s.plan_handle
),
deltas AS (
    SELECT
        poll_time_utc AS ts,
        instance_id,
        db_id,
        query_hash,
        CASE 
            WHEN last_total_worker_time IS NULL THEN 0
            WHEN total_worker_time < last_total_worker_time THEN total_worker_time
            ELSE total_worker_time - last_total_worker_time
        END AS cpu_delta_ms,
        CASE 
            WHEN last_total_execution_count IS NULL THEN 0
            WHEN execution_count < last_total_execution_count THEN execution_count
            ELSE execution_count - last_total_execution_count
        END AS exec_delta,
        CASE 
            WHEN last_total_logical_reads IS NULL THEN 0
            WHEN total_logical_reads < last_total_logical_reads THEN total_logical_reads
            ELSE total_logical_reads - last_total_logical_reads
        END AS reads_delta,
        CASE 
            WHEN last_total_logical_writes IS NULL THEN 0
            WHEN total_logical_writes < last_total_logical_writes THEN total_logical_writes
            ELSE total_logical_writes - last_total_logical_writes
        END AS writes_delta,
        CASE 
            WHEN last_total_rows IS NULL THEN 0
            WHEN total_rows < last_total_rows THEN total_rows
            ELSE total_rows - last_total_rows
        END AS rows_delta,
        max_worker_time AS period_max_cpu_ms,
        max_logical_reads AS period_max_reads,
        max_grant_kb AS period_max_grant_kb,
        max_dop AS period_max_dop
    FROM joined
)
INSERT INTO sqlserver_query_stats_history (
    ts, instance_id, db_id, query_hash,
    cpu_delta_ms, exec_delta, reads_delta, writes_delta, rows_delta,
    period_max_cpu_ms, period_max_reads, period_max_grant_kb, period_max_dop
)
SELECT ts, instance_id, db_id, query_hash,
       cpu_delta_ms, exec_delta, reads_delta, writes_delta, rows_delta,
       period_max_cpu_ms, period_max_reads, period_max_grant_kb, period_max_dop
FROM deltas
WHERE exec_delta > 0 OR cpu_delta_ms > 0;
`
	if _, err := tx.Exec(ctx, mergeSQL, instanceID); err != nil {
		return fmt.Errorf("merge deltas to history: %w", err)
	}
// 3. Upsert Snapshot
upsertSQL := `
INSERT INTO sqlserver_query_stats_snapshot_v2 (
instance_id, db_id, query_hash, plan_handle,
database_name, query_text_raw, statement_text,
last_total_worker_time, last_total_logical_reads, last_total_logical_writes,
last_total_execution_count, last_total_rows, last_total_grant_kb,
max_worker_time, max_logical_reads, max_dop, max_grant_kb, max_rows,
last_execution_time, last_seen_poll_time
)
SELECT
instance_id, db_id, query_hash, plan_handle,
MAX(database_name), MAX(query_text_raw), MAX(statement_text),
SUM(total_worker_time), SUM(total_logical_reads), SUM(total_logical_writes),
SUM(execution_count), SUM(total_rows), SUM(total_grant_kb),
MAX(max_worker_time), MAX(max_logical_reads), MAX(max_dop), MAX(max_grant_kb), MAX(max_rows),
MAX(last_execution_time), MAX(poll_time_utc)
FROM sqlserver_query_stats_staging_v2
WHERE instance_id = $1
GROUP BY instance_id, db_id, query_hash, plan_handle
ON CONFLICT (instance_id, db_id, query_hash, plan_handle)
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
    last_execution_time = EXCLUDED.last_execution_time,
    last_seen_poll_time = EXCLUDED.last_seen_poll_time,
    max_worker_time = GREATEST(sqlserver_query_stats_snapshot_v2.max_worker_time, EXCLUDED.max_worker_time),
    max_logical_reads = GREATEST(sqlserver_query_stats_snapshot_v2.max_logical_reads, EXCLUDED.max_logical_reads),
    max_dop = GREATEST(sqlserver_query_stats_snapshot_v2.max_dop, EXCLUDED.max_dop),
    max_grant_kb = GREATEST(sqlserver_query_stats_snapshot_v2.max_grant_kb, EXCLUDED.max_grant_kb),
    max_rows = GREATEST(sqlserver_query_stats_snapshot_v2.max_rows, EXCLUDED.max_rows);
`
	if _, err := tx.Exec(ctx, upsertSQL, instanceID); err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}

	// 4. Update Watermark
	watermarkSQL := `
INSERT INTO sqlserver_collector_instance_state (instance_id, last_poll_time_utc, sqlserver_start_time, last_successful_run)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (instance_id) DO UPDATE SET
    last_poll_time_utc = EXCLUDED.last_poll_time_utc,
    sqlserver_start_time = EXCLUDED.sqlserver_start_time,
    last_successful_run = EXCLUDED.last_successful_run;
`
	if _, err := tx.Exec(ctx, watermarkSQL, instanceID, pollTime, sqlStartTime); err != nil {
		return fmt.Errorf("update watermark: %w", err)
	}

	// 5. Truncate Staging for this instance
	if _, err := tx.Exec(ctx, "DELETE FROM sqlserver_query_stats_staging_v2 WHERE instance_id = $1", instanceID); err != nil {
		return fmt.Errorf("truncate staging: %w", err)
	}

	return tx.Commit(ctx)
}

func (w *TimescaleWriter) WriteMSSQLSessionEnrichment(ctx context.Context, instanceID string, enrichments []domain.MSSQLSessionEnrichment) error {
	if len(enrichments) == 0 {
		return nil
	}

	query := `
		INSERT INTO sqlserver_plan_enrichment (
			instance_id, plan_handle, login_name, application_name, database_name, is_user_workload, last_seen
		) VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (instance_id, plan_handle) DO UPDATE SET
			login_name = EXCLUDED.login_name,
			application_name = EXCLUDED.application_name,
			database_name = EXCLUDED.database_name,
			is_user_workload = EXCLUDED.is_user_workload,
			last_seen = NOW();
	`

	for _, e := range enrichments {
		_, err := w.pool.Exec(ctx, query,
			instanceID, e.PlanHandle, e.LoginName, e.ApplicationName, e.DatabaseName, e.IsUserWorkload,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// ReadMSSQLPlanEnrichment returns enrichment rows for an instance from
// sqlserver_plan_enrichment, scoped to the recent past so stale plan handles
// from long-evicted sessions don't pollute the join. The 30s
// session-enrichment collector keeps last_seen current for active plans.
//
// This is the read side that lets the 1-minute query-snapshot collector
// avoid re-querying sys.dm_exec_requests / dm_exec_sessions every cycle.
func (w *TimescaleWriter) ReadMSSQLPlanEnrichment(ctx context.Context, instanceID string) ([]domain.MSSQLSessionEnrichment, error) {
	const query = `
		SELECT plan_handle, login_name, application_name, database_name, is_user_workload
		FROM sqlserver_plan_enrichment
		WHERE instance_id = $1
		  AND last_seen >= NOW() - INTERVAL '10 minutes'
	`

	rows, err := w.pool.Query(ctx, query, instanceID)
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
