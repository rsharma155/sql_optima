// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB reads for admin SQL Server collector / hypertable diagnostics.
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
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SqlServerCollectorDiagnostics is the admin diagnostic payload (no credentials).
type SqlServerCollectorDiagnostics struct {
	ServerID          uuid.UUID                        `json:"server_id"`
	ServerName        string                           `json:"server_name"`
	DBType            string                           `json:"db_type"`
	RegistryActive    bool                             `json:"registry_active"`
	ConnectionStatus  string                           `json:"connection_status,omitempty"`
	Window            DiagnosticWindow                 `json:"window"`
	Summary           DiagnosticSummary                `json:"summary"`
	CollectorState    SqlServerCollectorInstanceState  `json:"collector_state"`
	Collectors        map[string]CollectorConfigStatus `json:"collectors"`
	QueryV2AlwaysOn   bool                             `json:"query_v2_pipeline_always_on"`
	Hypertables       []DiagnosticHypertableArtifact   `json:"hypertables"`
	LatestCapture     map[string]*time.Time            `json:"latest_capture"`
	RowCountsInWindow map[string]int64                 `json:"row_counts_in_window"`
	Hints             []string                         `json:"hints,omitempty"`
}

type DiagnosticSummary struct {
	TablesChecked          int   `json:"tables_checked"`
	TablesWithRowsInWindow int   `json:"tables_with_rows_in_window"`
	TotalRowsInWindow      int64 `json:"total_rows_in_window"`
	TotalRowsAllTime       int64 `json:"total_rows_all_time"`
	TotalStorageBytes      int64 `json:"total_storage_bytes"`
}

type DiagnosticWindow struct {
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`
	Hours int       `json:"hours"`
}

type SqlServerCollectorInstanceState struct {
	LastPollTimeUTC      *time.Time `json:"last_poll_time_utc,omitempty"`
	SQLServerStartTime   *time.Time `json:"sqlserver_start_time,omitempty"`
	LastSuccessfulRun    *time.Time `json:"last_successful_run,omitempty"`
	LastError            *string    `json:"last_error,omitempty"`
	HasCollectorStateRow bool       `json:"has_collector_state_row"`
}

type CollectorConfigStatus struct {
	Enabled          bool `json:"enabled"`
	FrequencySeconds int  `json:"frequency_seconds"`
}

// DiagnosticHypertableArtifact is per-table Timescale coverage for one SQL Server instance.
type DiagnosticHypertableArtifact struct {
	Table              string     `json:"table"`
	Schema             string     `json:"schema"`
	QualifiedName      string     `json:"qualified_name"`
	Category           string     `json:"category"`
	Dashboards         string     `json:"dashboards,omitempty"`
	IsHypertable       bool       `json:"is_hypertable"`
	LatestCapture      *time.Time `json:"latest_capture,omitempty"`
	RowsInWindow       int64      `json:"rows_in_window"`
	RowsTotal          int64      `json:"rows_total"`
	RelationSizeBytes  int64      `json:"relation_size_bytes"`
	RelationSizePretty string     `json:"relation_size_pretty"`
	CompressionEnabled *bool      `json:"compression_enabled,omitempty"`
	NumChunks          *int64     `json:"num_chunks,omitempty"`
}

type diagTableSpec struct {
	Schema     string
	Table      string
	Category   string
	Dashboards string
	Engine     string // monitor.* tables: filter engine column
}

var sqlServerDiagTableCatalog = []diagTableSpec{
	{Schema: "public", Table: "sqlserver_query_stats_history", Category: "query", Dashboards: "Workload, Query Analysis"},
	{Schema: "public", Table: "sqlserver_query_metrics_v2", Category: "query", Dashboards: "Workload, Query Analysis"},
	{Schema: "public", Table: "sqlserver_perf_counters", Category: "performance", Dashboards: "Enterprise, Health, Workload fallback"},
	{Schema: "public", Table: "sqlserver_wait_stats_delta", Category: "performance", Dashboards: "Wait Stats, Health V2"},
	{Schema: "public", Table: "sqlserver_metrics", Category: "performance", Dashboards: "Overview, Instance health"},
	{Schema: "public", Table: "sqlserver_memory_history", Category: "memory", Dashboards: "Memory drilldown"},
	{Schema: "public", Table: "sqlserver_buffer_pool_db", Category: "memory", Dashboards: "Memory, Enterprise"},
	{Schema: "public", Table: "sqlserver_disk_history", Category: "storage", Dashboards: "Health V2, Intelligence"},
	{Schema: "monitor", Table: "index_usage_stats", Category: "storage", Dashboards: "Storage & Index Health", Engine: "sqlserver"},
	{Schema: "monitor", Table: "table_usage_stats", Category: "storage", Dashboards: "Storage & Index Health", Engine: "sqlserver"},
	{Schema: "public", Table: "sqlserver_performance_debt_findings", Category: "governance", Dashboards: "Performance debt"},
	{Schema: "public", Table: "sqlserver_active_sessions", Category: "live", Dashboards: "Sessions, Locks context"},
}

type SqlServerCollectorDiagnosticsRepository struct {
	pool *pgxpool.Pool
}

func NewSqlServerCollectorDiagnosticsRepository(pool *pgxpool.Pool) *SqlServerCollectorDiagnosticsRepository {
	return &SqlServerCollectorDiagnosticsRepository{pool: pool}
}

// GetServerMeta returns registry metadata without credentials.
func (r *SqlServerCollectorDiagnosticsRepository) GetServerMeta(ctx context.Context, serverID uuid.UUID) (name, dbType string, active bool, err error) {
	if r == nil || r.pool == nil {
		return "", "", false, fmt.Errorf("timescale not configured")
	}
	err = r.pool.QueryRow(ctx, `
		SELECT name, db_type, is_active FROM optima_servers WHERE id = $1`, serverID).
		Scan(&name, &dbType, &active)
	return name, dbType, active, err
}

// GetDiagnostics builds the diagnostic report for a SQL Server instance.
func (r *SqlServerCollectorDiagnosticsRepository) GetDiagnostics(
	ctx context.Context,
	serverID uuid.UUID,
	serverName string,
	registryActive bool,
	connectionStatus string,
	from, to time.Time,
) (*SqlServerCollectorDiagnostics, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("timescale not configured")
	}

	hours := int(to.Sub(from).Hours())
	if hours < 1 {
		hours = 1
	}

	out := &SqlServerCollectorDiagnostics{
		ServerID:          serverID,
		ServerName:        serverName,
		DBType:            "sqlserver",
		RegistryActive:    registryActive,
		ConnectionStatus:  connectionStatus,
		Window:            DiagnosticWindow{From: from.UTC(), To: to.UTC(), Hours: hours},
		Collectors:        map[string]CollectorConfigStatus{},
		QueryV2AlwaysOn:   true,
		LatestCapture:     map[string]*time.Time{},
		RowCountsInWindow: map[string]int64{},
	}

	if err := r.loadCollectorState(ctx, serverID, &out.CollectorState); err != nil {
		return nil, err
	}
	r.loadCollectorConfigs(ctx, out.Collectors)

	artifacts, err := r.loadHypertableArtifacts(ctx, serverID, from, to)
	if err != nil {
		return nil, err
	}
	out.Hypertables = artifacts

	for _, a := range artifacts {
		key := a.QualifiedName
		if a.LatestCapture != nil {
			t := *a.LatestCapture
			out.LatestCapture[key] = &t
		}
		out.RowCountsInWindow[key] = a.RowsInWindow
		out.Summary.TablesChecked++
		if a.RowsInWindow > 0 {
			out.Summary.TablesWithRowsInWindow++
		}
		out.Summary.TotalRowsInWindow += a.RowsInWindow
		out.Summary.TotalRowsAllTime += a.RowsTotal
		out.Summary.TotalStorageBytes += a.RelationSizeBytes
	}

	out.Hints = buildDiagnosticHints(out)
	return out, nil
}

func (r *SqlServerCollectorDiagnosticsRepository) loadCollectorState(ctx context.Context, serverID uuid.UUID, st *SqlServerCollectorInstanceState) error {
	var lastPoll, startTime, lastRun sql.NullTime
	var lastErr sql.NullString
	err := r.pool.QueryRow(ctx, `
		SELECT last_poll_time_utc, sqlserver_start_time, last_successful_run, last_error
		FROM sqlserver_collector_instance_state WHERE server_id = $1`, serverID).
		Scan(&lastPoll, &startTime, &lastRun, &lastErr)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	st.HasCollectorStateRow = true
	if lastPoll.Valid {
		t := lastPoll.Time.UTC()
		st.LastPollTimeUTC = &t
	}
	if startTime.Valid {
		t := startTime.Time.UTC()
		st.SQLServerStartTime = &t
	}
	if lastRun.Valid {
		t := lastRun.Time.UTC()
		st.LastSuccessfulRun = &t
	}
	if lastErr.Valid && strings.TrimSpace(lastErr.String) != "" {
		s := lastErr.String
		st.LastError = &s
	}
	return nil
}

func (r *SqlServerCollectorDiagnosticsRepository) loadCollectorConfigs(ctx context.Context, out map[string]CollectorConfigStatus) {
	names := []string{"sqlserver_query_snapshot", "sqlserver_perf_counters", "sqlserver_session_enrichment"}
	for _, name := range names {
		var freq int
		var active bool
		err := r.pool.QueryRow(ctx, `
			SELECT frequency_seconds, is_active FROM optima_collector_configs WHERE collector_name = $1`, name).
			Scan(&freq, &active)
		if err != nil {
			continue
		}
		out[name] = CollectorConfigStatus{Enabled: active, FrequencySeconds: freq}
	}
}

func (r *SqlServerCollectorDiagnosticsRepository) loadHypertableArtifacts(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]DiagnosticHypertableArtifact, error) {
	meta := r.loadHypertableMeta(ctx)
	results := make([]DiagnosticHypertableArtifact, len(sqlServerDiagTableCatalog))

	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	var firstErr error
	var errMu sync.Mutex

	for i, spec := range sqlServerDiagTableCatalog {
		wg.Add(1)
		go func(idx int, s diagTableSpec) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			qualified := s.Schema + "." + s.Table
			art := DiagnosticHypertableArtifact{
				Table:         s.Table,
				Schema:        s.Schema,
				QualifiedName: qualified,
				Category:      s.Category,
				Dashboards:    s.Dashboards,
			}
			if m, ok := meta[qualified]; ok {
				art.IsHypertable = m.isHypertable
				art.RelationSizeBytes = m.sizeBytes
				art.RelationSizePretty = formatBytes(m.sizeBytes)
				if m.compression != nil {
					art.CompressionEnabled = m.compression
				}
				if m.chunks != nil {
					art.NumChunks = m.chunks
				}
			} else {
				art.RelationSizeBytes, art.RelationSizePretty = r.relationSize(ctx, s.Schema, s.Table)
			}

			latest, total, window, err := r.tableRowStats(ctx, s, serverID, from, to)
			errMu.Lock()
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", qualified, err)
			}
			errMu.Unlock()
			if err == nil {
				art.LatestCapture = latest
				art.RowsTotal = total
				art.RowsInWindow = window
			}
			results[idx] = art
		}(i, spec)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

type hypertableMeta struct {
	isHypertable bool
	sizeBytes    int64
	compression  *bool
	chunks       *int64
}

func (r *SqlServerCollectorDiagnosticsRepository) loadHypertableMeta(ctx context.Context) map[string]hypertableMeta {
	out := make(map[string]hypertableMeta)
	rows, err := r.pool.Query(ctx, `
		SELECT h.table_schema, h.table_name,
		       COALESCE(pg_total_relation_size(format('%I.%I', h.table_schema, h.table_name)::regclass), 0),
		       h.compression_enabled,
		       (SELECT COUNT(*)::bigint FROM timescaledb_information.chunks c
		        WHERE c.hypertable_schema = h.table_schema AND c.hypertable_name = h.table_name)
		FROM timescaledb_information.hypertables h`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var schema, table string
		var size int64
		var compression bool
		var chunks int64
		if err := rows.Scan(&schema, &table, &size, &compression, &chunks); err != nil {
			continue
		}
		key := schema + "." + table
		c := chunks
		comp := compression
		out[key] = hypertableMeta{
			isHypertable: true,
			sizeBytes:    size,
			compression:  &comp,
			chunks:       &c,
		}
	}
	return out
}

func (r *SqlServerCollectorDiagnosticsRepository) relationSize(ctx context.Context, schema, table string) (int64, string) {
	var size int64
	_ = r.pool.QueryRow(ctx, `
		SELECT COALESCE(pg_total_relation_size(format('%I.%I', $1, $2)::regclass), 0)`, schema, table).Scan(&size)
	return size, formatBytes(size)
}

func (r *SqlServerCollectorDiagnosticsRepository) tableRowStats(
	ctx context.Context,
	spec diagTableSpec,
	serverID uuid.UUID,
	from, to time.Time,
) (latest *time.Time, total, window int64, err error) {
	qualified := fmt.Sprintf("%s.%s", spec.Schema, spec.Table)
	engineClause := ""
	args := []interface{}{serverID}
	if spec.Engine != "" {
		engineClause = " AND engine = $2"
		args = append(args, spec.Engine)
	}

	latestSQL := fmt.Sprintf(`
		SELECT MAX(capture_timestamp) FROM %s WHERE server_id = $1%s`, qualified, engineClause)
	var ts sql.NullTime
	if err = r.pool.QueryRow(ctx, latestSQL, args...).Scan(&ts); err != nil {
		return nil, 0, 0, err
	}
	if ts.Valid {
		t := ts.Time.UTC()
		latest = &t
	}

	totalSQL := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE server_id = $1%s`, qualified, engineClause)
	if err = r.pool.QueryRow(ctx, totalSQL, args...).Scan(&total); err != nil {
		return latest, 0, 0, err
	}

	windowArgs := append([]interface{}{}, args...)
	windowArgs = append(windowArgs, from, to)
	fromIdx := len(args) + 1
	toIdx := len(args) + 2
	windowSQL := fmt.Sprintf(`
		SELECT COUNT(*) FROM %s
		WHERE server_id = $1%s AND capture_timestamp >= $%d AND capture_timestamp <= $%d`,
		qualified, engineClause, fromIdx, toIdx)
	if err = r.pool.QueryRow(ctx, windowSQL, windowArgs...).Scan(&window); err != nil {
		return latest, total, 0, err
	}
	return latest, total, window, nil
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func buildDiagnosticHints(d *SqlServerCollectorDiagnostics) []string {
	var hints []string
	qSnap, qOk := d.Collectors["sqlserver_query_snapshot"]
	_, perfOk := d.Collectors["sqlserver_perf_counters"]

	if !d.RegistryActive {
		hints = append(hints, "Server is inactive in optima_servers; collectors skip this target.")
	}
	if d.ConnectionStatus == "offline" {
		hints = append(hints, "Live connection status is offline; DMV collectors may not run until connectivity is restored.")
	}
	if qOk && !qSnap.Enabled {
		hints = append(hints, "sqlserver_query_snapshot is disabled in optima_collector_configs; workload and query-analysis history will stay empty.")
	}
	if !d.CollectorState.HasCollectorStateRow {
		hints = append(hints, "No sqlserver_collector_instance_state row yet; query snapshot has not completed a successful cycle for this instance.")
	} else if d.CollectorState.LastError != nil {
		hints = append(hints, "Last query snapshot error: "+*d.CollectorState.LastError)
	}

	var histRows, perfRows int64
	for _, t := range d.Hypertables {
		switch t.QualifiedName {
		case "public.sqlserver_query_stats_history":
			histRows = t.RowsInWindow
		case "public.sqlserver_perf_counters":
			perfRows = t.RowsInWindow
		}
	}
	if histRows == 0 && perfRows > 0 {
		hints = append(hints, "Query-stats history is empty but perf counters exist; workload trends may use perf-counter fallback until 2+ query snapshot intervals complete.")
	}
	if histRows == 0 && (!perfOk || perfRows == 0) {
		if qOk && qSnap.Enabled && d.ConnectionStatus != "offline" {
			hints = append(hints, "No hypertable rows in the selected window; allow 1–2 sqlserver_query_snapshot intervals after deploy or check DMV permissions on the monitored instance.")
		}
	}
	if d.CollectorState.LastSuccessfulRun != nil {
		age := time.Since(*d.CollectorState.LastSuccessfulRun)
		if qOk && age > time.Duration(qSnap.FrequencySeconds*3)*time.Second {
			hints = append(hints, fmt.Sprintf("Last successful query snapshot was %s ago (expected about every %ds).", age.Round(time.Second), qSnap.FrequencySeconds))
		}
	}
	return hints
}
