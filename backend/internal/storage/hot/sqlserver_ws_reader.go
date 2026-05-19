// Package hot provides TimescaleDB storage implementation for high-frequency metrics.
// sqlserver_ws_reader.go implements read-only queries for the Wait Stats V2 dashboard.
// Metadata:
//   - Feature: SQL Server Wait Stats V2
//   - Layer: Infrastructure (Storage Read)
package hot

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/models"
)

// GetWaitKPIsV2 returns aggregate wait stats for a time window.
func (tl *TimescaleLogger) GetWaitKPIsV2(
	ctx context.Context,
	serverID uuid.UUID,
	from, to time.Time,
) (models.WaitStatsKPIs, error) {

	const q = `
        WITH stats AS (
            SELECT
                SUM(delta_wait_ms)          AS total_wait_ms,
                SUM(delta_signal_wait_ms)   AS total_signal_ms,
                SUM(delta_waiting_tasks)    AS total_tasks,
                BOOL_OR(restart_detected)   AS restart_detected
            FROM sqlserver_wait_stats_delta
            WHERE server_id = $1
              AND capture_timestamp BETWEEN $2 AND $3
        ),
        top_cat AS (
            SELECT wait_category
            FROM sqlserver_wait_stats_delta
            WHERE server_id = $1
              AND capture_timestamp BETWEEN $2 AND $3
            GROUP BY wait_category
            ORDER BY SUM(delta_wait_ms) DESC
            LIMIT 1
        )
        SELECT 
            s.total_wait_ms, s.total_signal_ms, s.total_tasks, s.restart_detected,
            (CAST(s.total_signal_ms AS FLOAT) / NULLIF(s.total_wait_ms, 0)) * 100 AS signal_pct,
            tc.wait_category
        FROM stats s
        LEFT JOIN top_cat tc ON true`

	var k models.WaitStatsKPIs
	var total, signal, tasks *int64
	var restart *bool
	var pct *float64
	var topCat *string

	err := tl.pool.QueryRow(ctx, q, serverID, from, to).Scan(&total, &signal, &tasks, &restart, &pct, &topCat)
	if err != nil {
		return k, fmt.Errorf("GetWaitKPIsV2: %w", err)
	}

	if total != nil {
		k.TotalWaitTimeMs = *total
	}
	if signal != nil {
		k.SignalWaitTimeMs = *signal
	}
	if tasks != nil {
		k.TotalWaitingTasks = *tasks
	}
	if restart != nil {
		k.RestartDetected = *restart
	}
	if pct != nil {
		k.SignalWaitPct = *pct
	}
	if topCat != nil {
		k.TopWaitCategory = *topCat
	}
	k.ResourceWaitTimeMs = k.TotalWaitTimeMs - k.SignalWaitTimeMs
	if k.ResourceWaitTimeMs < 0 {
		k.ResourceWaitTimeMs = 0
	}

	return k, nil
}

// GetCPUPressureTrend relates signal waits to scheduler yields.
func (tl *TimescaleLogger) GetCPUPressureTrend(
	ctx context.Context,
	serverID uuid.UUID,
	from, to time.Time,
) ([]models.CPUPressurePoint, error) {

	const q = `
        SELECT
            time_bucket('5 minutes', capture_timestamp) AS bucket,
            SUM(delta_signal_wait_ms) AS signal_wait_ms,
            SUM(delta_wait_ms) FILTER (WHERE wait_type = 'SOS_SCHEDULER_YIELD') AS scheduler_yield_ms
        FROM sqlserver_wait_stats_delta
        WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3
        GROUP BY bucket ORDER BY bucket`

	rows, err := tl.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetCPUPressureTrend: %w", err)
	}
	defer rows.Close()

	var out []models.CPUPressurePoint
	for rows.Next() {
		var p models.CPUPressurePoint
		var sig, yield *int64
		if err := rows.Scan(&p.Timestamp, &sig, &yield); err != nil {
			continue
		}
		if sig != nil {
			p.SignalWaitMs = *sig
		}
		if yield != nil {
			p.SchedulerYieldMs = *yield
		}
		out = append(out, p)
	}
	return out, nil
}

// GetDatabaseWaitImpact returns wait time per database in the selected time window.
func (tl *TimescaleLogger) GetDatabaseWaitImpact(
	ctx context.Context,
	serverID uuid.UUID,
	from, to time.Time,
) ([]models.DatabaseWaitImpact, error) {

	const q = `
        SELECT
            database_name,
            SUM(wait_duration_ms) AS total_wait_ms
        FROM sqlserver_active_wait_sessions
        WHERE server_id = $1
          AND capture_timestamp BETWEEN $2 AND $3
        GROUP BY database_name
        ORDER BY total_wait_ms DESC
        LIMIT 10`

	rows, err := tl.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetDatabaseWaitImpact: %w", err)
	}
	defer rows.Close()

	var out []models.DatabaseWaitImpact
	for rows.Next() {
		var r models.DatabaseWaitImpact
		if err := rows.Scan(&r.DatabaseName, &r.TotalWaitMs); err != nil {
			continue
		}
		if r.DatabaseName == "" {
			r.DatabaseName = "Other/System"
		}
		out = append(out, r)
	}
	return out, nil
}

// GetBlockingTree returns the blocking hierarchy at the most-recent snapshot within the time range.
func (tl *TimescaleLogger) GetBlockingTree(
	ctx context.Context,
	serverID uuid.UUID,
	from, to time.Time,
) ([]models.WaitStatsBlockingNode, error) {

	const q = `
        WITH latest AS (
            SELECT MAX(capture_timestamp) AS ts
            FROM sqlserver_active_wait_sessions
            WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3
        )
        SELECT
            session_id, blocking_session_id, wait_type, wait_duration_ms,
            database_name, query_text
        FROM sqlserver_active_wait_sessions
        WHERE server_id = $1
          AND capture_timestamp = (SELECT ts FROM latest)
          AND (blocking_session_id > 0
               OR session_id IN (
                   SELECT DISTINCT blocking_session_id
                   FROM sqlserver_active_wait_sessions
                   WHERE server_id = $1 AND capture_timestamp = (SELECT ts FROM latest)
               ))`

	rows, err := tl.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetBlockingTree: %w", err)
	}
	defer rows.Close()

	var allNodes []models.WaitStatsBlockingNode
	for rows.Next() {
		var n models.WaitStatsBlockingNode
		if err := rows.Scan(&n.SessionID, &n.BlockingSessionID, &n.WaitType, &n.WaitDurationMs, &n.DatabaseName, &n.QueryText); err != nil {
			continue
		}
		allNodes = append(allNodes, n)
	}

	return buildBlockingTree(allNodes), nil
}

func buildBlockingTree(nodes []models.WaitStatsBlockingNode) []models.WaitStatsBlockingNode {
	nodeMap := make(map[int]*models.WaitStatsBlockingNode)
	for i := range nodes {
		nodeMap[nodes[i].SessionID] = &nodes[i]
	}

	var roots []models.WaitStatsBlockingNode
	for i := range nodes {
		node := &nodes[i]
		if node.BlockingSessionID == 0 || nodeMap[node.BlockingSessionID] == nil {
			roots = append(roots, *node)
		}
	}

	// This is a simplified tree builder. For a real production app, 
	// we'd do a recursive pass to attach children.
	for i := range roots {
		roots[i].Children = findChildren(&roots[i], nodes)
	}

	return roots
}

func findChildren(parent *models.WaitStatsBlockingNode, allNodes []models.WaitStatsBlockingNode) []models.WaitStatsBlockingNode {
	var children []models.WaitStatsBlockingNode
	for i := range allNodes {
		if allNodes[i].BlockingSessionID == parent.SessionID {
			child := allNodes[i]
			child.Children = findChildren(&child, allNodes)
			children = append(children, child)
		}
	}
	return children
}

// GetWaitTypeHelp returns DBA guidance for a specific wait type.
func (tl *TimescaleLogger) GetWaitTypeHelp(
	ctx context.Context,
	waitType string,
) (models.WaitTypeHelp, error) {

	const q = `
        SELECT wait_type, description, likely_cause, recommended_action
        FROM sqlserver_wait_type_help
        WHERE wait_type = $1`

	var h models.WaitTypeHelp
	err := tl.pool.QueryRow(ctx, q, waitType).Scan(&h.WaitType, &h.Description, &h.LikelyCause, &h.RecommendedAction)
	if err != nil {
		// Return empty/generic if not found
		return models.WaitTypeHelp{
			WaitType:          waitType,
			Description:       "No specific guidance available for this wait type.",
			LikelyCause:       "Review Microsoft documentation for this wait type.",
			RecommendedAction: "Monitor trend over time. If persistent, investigate with sys.dm_exec_sessions.",
		}, nil
	}
	return h, nil
}
func (tl *TimescaleLogger) GetTopWaitTypesV2(
	ctx context.Context,
	serverID uuid.UUID,
	from, to time.Time,
	limit int,
) ([]models.TopWaitType, error) {

	const q = `
        WITH totals AS (
            SELECT SUM(delta_wait_ms) as grand_total
            FROM sqlserver_wait_stats_delta
            WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3
        )
        SELECT
            d.wait_type,
            d.wait_category,
            SUM(d.delta_wait_ms)          AS total_wait_ms,
            SUM(d.delta_waiting_tasks)    AS total_tasks,
            CAST(SUM(d.delta_wait_ms) AS FLOAT) / NULLIF(SUM(d.delta_waiting_tasks), 0) AS avg_wait_ms,
            (CAST(SUM(d.delta_wait_ms) AS FLOAT) / NULLIF(t.grand_total, 0)) * 100      AS pct_of_total,
            COALESCE(h.description, '')   AS description,
            COALESCE(h.recommended_action, '') AS recommended_action
        FROM sqlserver_wait_stats_delta d
        CROSS JOIN totals t
        LEFT JOIN sqlserver_wait_type_help h ON d.wait_type = h.wait_type
        WHERE d.server_id = $1
          AND d.capture_timestamp BETWEEN $2 AND $3
        GROUP BY d.wait_type, d.wait_category, t.grand_total, h.description, h.recommended_action
        HAVING SUM(d.delta_wait_ms) > 0
        ORDER BY total_wait_ms DESC
        LIMIT $4`

	rows, err := tl.pool.Query(ctx, q, serverID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("GetTopWaitTypesV2: %w", err)
	}
	defer rows.Close()

	var out []models.TopWaitType
	for rows.Next() {
		var r models.TopWaitType
		if err := rows.Scan(&r.WaitType, &r.Category, &r.WaitTimeMs, &r.WaitingTasksCount,
			&r.AvgWaitMs, &r.PercentOfTotal, &r.Description, &r.RecommendedAction); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// GetWaitTrendsV2 returns a category-level time-series.
// It detects range and switches from raw delta table to 1h CAGG automatically.
func (tl *TimescaleLogger) GetWaitTrendsV2(
	ctx context.Context,
	serverID uuid.UUID,
	from, to time.Time,
) ([]models.WaitTrendPoint, error) {

	rangeHours := to.Sub(from).Hours()
	var query string
	bucket := "5 minutes"

	if rangeHours <= 48 {
		if rangeHours > 12 {
			bucket = "15 minutes"
		}
		// Use raw delta table (1-5 min resolution)
		query = fmt.Sprintf(`
            SELECT 
                time_bucket('%s', capture_timestamp) AS bucket,
                SUM(CASE WHEN wait_category = 'CPU'         THEN delta_wait_ms ELSE 0 END) AS cpu,
                SUM(CASE WHEN wait_category = 'IO_DATA'     THEN delta_wait_ms ELSE 0 END) AS io,
                SUM(CASE WHEN wait_category = 'IO_LOG'      THEN delta_wait_ms ELSE 0 END) AS io_log,
                SUM(CASE WHEN wait_category = 'MEMORY'      THEN delta_wait_ms ELSE 0 END) AS memory,
                SUM(CASE WHEN wait_category = 'LOCKING'     THEN delta_wait_ms ELSE 0 END) AS locking,
                SUM(CASE WHEN wait_category = 'PARALLELISM' THEN delta_wait_ms ELSE 0 END) AS parallel,
                SUM(CASE WHEN wait_category NOT IN ('CPU','IO_DATA','IO_LOG','MEMORY','LOCKING','PARALLELISM') 
                         THEN delta_wait_ms ELSE 0 END) AS other
            FROM sqlserver_wait_stats_delta
            WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3
            GROUP BY bucket ORDER BY bucket`, bucket)
	} else {
		// Use hourly continuous aggregate
		query = `
            SELECT 
                bucket,
                SUM(CASE WHEN wait_category = 'CPU'         THEN total_wait_ms ELSE 0 END) AS cpu,
                SUM(CASE WHEN wait_category = 'IO_DATA'     THEN total_wait_ms ELSE 0 END) AS io,
                SUM(CASE WHEN wait_category = 'IO_LOG'      THEN total_wait_ms ELSE 0 END) AS io_log,
                SUM(CASE WHEN wait_category = 'MEMORY'      THEN total_wait_ms ELSE 0 END) AS memory,
                SUM(CASE WHEN wait_category = 'LOCKING'     THEN total_wait_ms ELSE 0 END) AS locking,
                SUM(CASE WHEN wait_category = 'PARALLELISM' THEN total_wait_ms ELSE 0 END) AS parallel,
                SUM(CASE WHEN wait_category NOT IN ('CPU','IO_DATA','IO_LOG','MEMORY','LOCKING','PARALLELISM') 
                         THEN total_wait_ms ELSE 0 END) AS other
            FROM sqlserver_cagg_wait_delta_1h
            WHERE server_id = $1 AND bucket BETWEEN $2 AND $3
            GROUP BY bucket ORDER BY bucket`
	}
	rows, err := tl.pool.Query(ctx, query, serverID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetWaitTrendsV2: %w", err)
	}
	defer rows.Close()

	var out []models.WaitTrendPoint
	for rows.Next() {
		var p models.WaitTrendPoint
		var ioLog float64 // IO_LOG is merged into IO for the frontend model
		if err := rows.Scan(&p.Timestamp, &p.CPU, &p.IO, &ioLog, &p.Memory, &p.Locking, &p.Parallel, &p.Other); err != nil {
			continue
		}
		p.IO += ioLog // Merge IO_DATA and IO_LOG for the UI chart
		out = append(out, p)
	}
	return out, nil
}

// GetActiveWaitSessionsV2 returns sessions from the most-recent snapshot within the time range.
func (tl *TimescaleLogger) GetActiveWaitSessionsV2(
	ctx context.Context,
	serverID uuid.UUID,
	from, to time.Time,
) ([]models.ActiveWaitSession, error) {

	const q = `
        WITH latest AS (
            SELECT MAX(capture_timestamp) AS ts
            FROM sqlserver_active_wait_sessions
            WHERE server_id = $1 AND capture_timestamp BETWEEN $2 AND $3
        )
        SELECT
            session_id, wait_type, wait_duration_ms, blocking_session_id,
            database_name, host_name, program_name, login_name, query_text,
            capture_timestamp
        FROM sqlserver_active_wait_sessions
        WHERE server_id = $1
          AND capture_timestamp = (SELECT ts FROM latest)
        ORDER BY wait_duration_ms DESC
        LIMIT 50`

	rows, err := tl.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetActiveWaitSessionsV2: %w", err)
	}
	defer rows.Close()

	var out []models.ActiveWaitSession
	for rows.Next() {
		var s models.ActiveWaitSession
		if err := rows.Scan(&s.SessionID, &s.WaitType, &s.WaitDurationMs, &s.BlockingSessionID,
			&s.DatabaseName, &s.HostName, &s.ProgramName, &s.LoginName, &s.QueryText, &s.CaptureTimestamp); err != nil {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}
