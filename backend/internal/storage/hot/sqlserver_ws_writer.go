// Package hot provides TimescaleDB storage implementation for high-frequency metrics.
// sqlserver_ws_writer.go handles batch inserts for the Wait Stats V2 pipeline.
// Metadata:
//   - Feature: SQL Server Wait Stats V2
//   - Layer: Infrastructure (Storage Write)
package hot

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rsharma155/sql_optima/internal/collectors/domain"
)

// cumulativeBatchHash computes a stable FNV hash over all rows in a snapshot batch.
func cumulativeBatchHash(rows []domain.WaitStatsCumulativeSnapshot) uint64 {
	// Sort by wait_type for stability before hashing.
	sorted := make([]domain.WaitStatsCumulativeSnapshot, len(rows))
	copy(sorted, rows)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].WaitType < sorted[j].WaitType })
	h := fnv.New64a()
	for _, r := range sorted {
		_, _ = h.Write([]byte(r.WaitType))
		b := [8]byte{
			byte(r.WaitTimeMs), byte(r.WaitTimeMs >> 8), byte(r.WaitTimeMs >> 16), byte(r.WaitTimeMs >> 24),
			byte(r.SignalWaitTimeMs), byte(r.SignalWaitTimeMs >> 8),
			byte(r.WaitingTasksCount), byte(r.WaitingTasksCount >> 8),
		}
		_, _ = h.Write(b[:])
	}
	return h.Sum64()
}

// WriteCumulativeSnapshot batch-inserts raw cumulative rows.
// Skips write if the batch content is identical to the last snapshot for this server.
// On first call after process restart (hash == 0), seeds the in-memory hash from the last DB row
// so a restart doesn't re-insert an unchanged snapshot.
func (tl *TimescaleLogger) WriteCumulativeSnapshot(
	ctx context.Context,
	serverID uuid.UUID,
	rows []domain.WaitStatsCumulativeSnapshot,
) error {
	if len(rows) == 0 {
		return nil
	}

	newHash := cumulativeBatchHash(rows)

	// On first call after startup, seed the in-memory hash from the last DB snapshot so a process
	// restart doesn't re-insert an unchanged cumulative snapshot.
	tl.mu.Lock()
	needsSeed := tl.prevCumulSnapHash[serverID] == 0
	tl.mu.Unlock()
	if needsSeed {
		if dbRows, err := tl.fetchLastCumulativeSnapshotRows(ctx, serverID); err == nil && len(dbRows) > 0 {
			tl.mu.Lock()
			if tl.prevCumulSnapHash[serverID] == 0 {
				tl.prevCumulSnapHash[serverID] = cumulativeBatchHash(dbRows)
			}
			tl.mu.Unlock()
		}
	}

	tl.mu.Lock()
	if tl.prevCumulSnapHash[serverID] == newHash {
		tl.mu.Unlock()
		return nil // identical snapshot — no change since last write
	}
	tl.prevCumulSnapHash[serverID] = newHash
	tl.mu.Unlock()

	now := time.Now().UTC()
	b := &pgx.Batch{}
	const q = `
        INSERT INTO sqlserver_wait_stats_cumulative
            (capture_timestamp, server_id, wait_type,
             waiting_tasks_count, wait_time_ms, signal_wait_time_ms)
        VALUES ($1,$2,$3,$4,$5,$6)
        ON CONFLICT (server_id, wait_type, capture_timestamp) DO NOTHING`
	for _, r := range rows {
		b.Queue(q, now, serverID, r.WaitType,
			r.WaitingTasksCount, r.WaitTimeMs, r.SignalWaitTimeMs)
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("WriteCumulativeSnapshot: %w", err)
		}
	}
	return nil
}

// WriteWaitStatsDelta batch-inserts pre-computed delta rows.
func (tl *TimescaleLogger) WriteWaitStatsDelta(
	ctx context.Context,
	serverID uuid.UUID,
	rows []domain.WaitStatsDeltaRow,
) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
        INSERT INTO sqlserver_wait_stats_delta
            (capture_timestamp, server_id, wait_type, wait_category,
             delta_wait_ms, delta_signal_wait_ms, delta_resource_wait_ms,
             delta_waiting_tasks, restart_detected)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, r.CaptureTimestamp, serverID, r.WaitType, r.WaitCategory,
			r.DeltaWaitMs, r.DeltaSignalWaitMs, r.DeltaResourceWaitMs,
			r.DeltaWaitingTasks, r.RestartDetected)
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("WriteWaitStatsDelta: %w", err)
		}
	}
	return nil
}

// WriteActiveWaitSessions batch-inserts enriched active session rows.
func (tl *TimescaleLogger) WriteActiveWaitSessions(
	ctx context.Context,
	serverID uuid.UUID,
	rows []domain.ActiveWaitSession,
) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
        INSERT INTO sqlserver_active_wait_sessions
            (capture_timestamp, server_id, session_id, wait_type,
             wait_duration_ms, blocking_session_id, database_name,
             host_name, program_name, login_name, query_text)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`
	now := time.Now().UTC()
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, now, serverID, r.SessionID, r.WaitType,
			r.WaitDurationMs, r.BlockingSessionID, r.DatabaseName,
			r.HostName, r.ProgramName, r.LoginName, truncateText(r.QueryText, 2000))
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("WriteActiveWaitSessions: %w", err)
		}
	}
	return nil
}

// fetchLastCumulativeSnapshotRows returns the most-recent snapshot rows as a slice so
// cumulativeBatchHash can be applied to seed the in-memory dedup hash after a restart.
func (tl *TimescaleLogger) fetchLastCumulativeSnapshotRows(ctx context.Context, serverID uuid.UUID) ([]domain.WaitStatsCumulativeSnapshot, error) {
	const q = `
        SELECT DISTINCT ON (wait_type)
            wait_type, wait_time_ms, signal_wait_time_ms, waiting_tasks_count
        FROM sqlserver_wait_stats_cumulative
        WHERE server_id = $1
        ORDER BY wait_type, capture_timestamp DESC`
	rows, err := tl.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WaitStatsCumulativeSnapshot
	for rows.Next() {
		var s domain.WaitStatsCumulativeSnapshot
		if err := rows.Scan(&s.WaitType, &s.WaitTimeMs, &s.SignalWaitTimeMs, &s.WaitingTasksCount); err != nil {
			continue
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetPrevCumulativeSnapshot fetches the most-recent cumulative snapshot
// for a given server from sqlserver_wait_stats_cumulative.
// Returns nil map (not an error) if no prior snapshot exists (first run).
func (tl *TimescaleLogger) GetPrevCumulativeSnapshot(
	ctx context.Context,
	serverID uuid.UUID,
) (map[string][3]int64, error) {
	const q = `
        SELECT DISTINCT ON (wait_type)
            wait_type, wait_time_ms, signal_wait_time_ms, waiting_tasks_count
        FROM sqlserver_wait_stats_cumulative
        WHERE server_id = $1
        ORDER BY wait_type, capture_timestamp DESC`
	rows, err := tl.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, fmt.Errorf("GetPrevCumulativeSnapshot: %w", err)
	}
	defer rows.Close()
	out := make(map[string][3]int64)
	for rows.Next() {
		var wt string
		var wms, sms, tasks int64
		if err := rows.Scan(&wt, &wms, &sms, &tasks); err != nil {
			continue
		}
		out[wt] = [3]int64{wms, sms, tasks}
	}
	if len(out) == 0 {
		return nil, nil // first run
	}
	return out, rows.Err()
}

func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
