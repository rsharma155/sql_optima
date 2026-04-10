package hot

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (tl *TimescaleLogger) LogPlanCacheHealth(ctx context.Context, instanceName string, row map[string]interface{}) error {
	if row == nil {
		return nil
	}
	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, `
		INSERT INTO sqlserver_plan_cache_health (
			capture_timestamp, server_instance_name,
			total_cache_mb, single_use_cache_mb, single_use_cache_pct,
			adhoc_cache_mb, prepared_cache_mb, proc_cache_mb
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, now, instanceName,
		getFloat64(row, "total_cache_mb"),
		getFloat64(row, "single_use_cache_mb"),
		getFloat64(row, "single_use_cache_pct"),
		getFloat64(row, "adhoc_cache_mb"),
		getFloat64(row, "prepared_cache_mb"),
		getFloat64(row, "proc_cache_mb"),
	)
	return err
}

func (tl *TimescaleLogger) GetPlanCacheHealth(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 60
	}
	q := `
		SELECT capture_timestamp, total_cache_mb, single_use_cache_mb, single_use_cache_pct,
		       adhoc_cache_mb, prepared_cache_mb, proc_cache_mb
		FROM sqlserver_plan_cache_health
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var ts time.Time
		var total, single, pct, adhoc, prep, proc float64
		if err := rows.Scan(&ts, &total, &single, &pct, &adhoc, &prep, &proc); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"capture_timestamp":     ts,
			"total_cache_mb":        total,
			"single_use_cache_mb":   single,
			"single_use_cache_pct":  pct,
			"adhoc_cache_mb":        adhoc,
			"prepared_cache_mb":     prep,
			"proc_cache_mb":         proc,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogMemoryGrantWaiters(ctx context.Context, instanceName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_memory_grant_waiters (
				capture_timestamp, server_instance_name,
				session_id, request_id, database_name, login_name,
				requested_memory_kb, granted_memory_kb, required_memory_kb, wait_time_ms, dop, query_text
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		`, now, instanceName,
			int32(getInt64FromMap(r, "session_id")),
			int32(getInt64FromMap(r, "request_id")),
			getStr(r, "database_name"),
			getStr(r, "login_name"),
			getInt64FromMap(r, "requested_memory_kb"),
			getInt64FromMap(r, "granted_memory_kb"),
			getInt64FromMap(r, "required_memory_kb"),
			getInt64FromMap(r, "wait_time_ms"),
			int32(getInt64FromMap(r, "dop")),
			getStr(r, "query_text"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("memory grant waiters insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetMemoryGrantWaiters(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT session_id, request_id, database_name, login_name,
		       requested_memory_kb, granted_memory_kb, required_memory_kb, wait_time_ms, dop, query_text
		FROM sqlserver_memory_grant_waiters
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var sid, rid int32
		var db, login, qtxt string
		var reqKB, grKB, needKB, waitMs int64
		var dop int32
		if err := rows.Scan(&sid, &rid, &db, &login, &reqKB, &grKB, &needKB, &waitMs, &dop, &qtxt); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"session_id":          sid,
			"request_id":          rid,
			"database_name":       db,
			"login_name":          login,
			"requested_memory_kb": reqKB,
			"granted_memory_kb":   grKB,
			"required_memory_kb":  needKB,
			"wait_time_ms":        waitMs,
			"dop":                 dop,
			"query_text":          qtxt,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogTempdbTopConsumers(ctx context.Context, instanceName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_tempdb_top_consumers (
				capture_timestamp, server_instance_name,
				session_id, database_name, login_name, host_name, program_name,
				tempdb_mb, user_objects_mb, internal_objects_mb, query_text
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		`, now, instanceName,
			int32(getInt64FromMap(r, "session_id")),
			getStr(r, "database_name"),
			getStr(r, "login_name"),
			getStr(r, "host_name"),
			getStr(r, "program_name"),
			getFloat64(r, "tempdb_mb"),
			getFloat64(r, "user_objects_mb"),
			getFloat64(r, "internal_objects_mb"),
			getStr(r, "query_text"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("tempdb consumers insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetTempdbTopConsumers(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT session_id, database_name, login_name, host_name, program_name,
		       tempdb_mb, user_objects_mb, internal_objects_mb, query_text
		FROM sqlserver_tempdb_top_consumers
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var sid int32
		var db, login, host, program, qtxt string
		var total, user, internal float64
		if err := rows.Scan(&sid, &db, &login, &host, &program, &total, &user, &internal, &qtxt); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"session_id":          sid,
			"database_name":       db,
			"login_name":          login,
			"host_name":           host,
			"program_name":        program,
			"tempdb_mb":           total,
			"user_objects_mb":     user,
			"internal_objects_mb": internal,
			"query_text":          qtxt,
		})
	}
	return out, rows.Err()
}

