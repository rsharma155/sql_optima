// Package service implements business logic for metric collection and caching.
// It provides a unified interface for both SQL Server and PostgreSQL monitoring data.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Metrics service orchestration including TimescaleDB persistence and cache management.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
	"github.com/rsharma155/sql_optima/pkg/dashboard"
)

type MetricsService struct {
	PgRepo           *repository.PgRepository
	MsRepo           *repository.MssqlRepository
	WidgetRepo       *repository.WidgetRepository
	UserRepo         *repository.UserRepository
	Config           *config.Config
	cacheMutex       sync.RWMutex
	dashboardCache   map[string]models.DashboardMetrics
	pgDashboardCache map[string]models.PgCoreDashboardCache
	xeDb             *sql.DB
	xeSqlitePath     string
	tsLogger         *hot.TimescaleLogger
	tsHotStorage     *hot.HotStorage

	// Storage & Index Health collection scheduling.
	// We collect raw engine counters frequently, but persist certain Timescale snapshots
	// on coarser cadences to avoid unnecessary storage growth.
	sihMu            sync.Mutex
	sihLastIndex15m  map[string]time.Time
	sihLastTable15m  map[string]time.Time
	sihLastGrowth6h  map[string]time.Time
	sihLastDefsDaily map[string]time.Time
}

func NewMetricsService(pg *repository.PgRepository, ms *repository.MssqlRepository, cfg *config.Config, tsStorage *hot.HotStorage) *MetricsService {
	var tsLogger *hot.TimescaleLogger
	if tsStorage != nil {
		tsLogger = hot.NewTimescaleLogger(tsStorage.Pool())
		log.Println("[MetricsService] TimescaleDB logger initialized")
	} else {
		log.Println("[MetricsService] TimescaleDB not configured, metrics will not be persisted")
	}

	var widgetRepo *repository.WidgetRepository
	var userRepo *repository.UserRepository
	if tsStorage != nil {
		widgetRepo = repository.NewWidgetRepository(tsStorage.Pool())
		userRepo = repository.NewUserRepository(tsStorage.Pool())
		log.Println("[MetricsService] Widget registry and user management initialized")
	}

	return &MetricsService{
		PgRepo:           pg,
		MsRepo:           ms,
		WidgetRepo:       widgetRepo,
		UserRepo:         userRepo,
		Config:           cfg,
		dashboardCache:   make(map[string]models.DashboardMetrics),
		pgDashboardCache: make(map[string]models.PgCoreDashboardCache),
		tsLogger:         tsLogger,
		tsHotStorage:     tsStorage,

		sihLastIndex15m:  make(map[string]time.Time),
		sihLastTable15m:  make(map[string]time.Time),
		sihLastGrowth6h:  make(map[string]time.Time),
		sihLastDefsDaily: make(map[string]time.Time),
	}
}

// GetTimescaleDBPool returns the TimescaleDB connection pool for direct queries
func (s *MetricsService) GetTimescaleDBPool() *pgxpool.Pool {
	if s.tsHotStorage != nil {
		return s.tsHotStorage.Pool()
	}
	return nil
}

func (s *MetricsService) IsTimescaleConnected() bool {
	return s.tsLogger != nil && s.tsHotStorage != nil
}

// =============================================================================
// Timescale-backed Storage & Index Health reads
// =============================================================================

func (s *MetricsService) TimescaleStorageIndexHealthIndexUsage(ctx context.Context, engine, instance, from, to string, limit int) ([]models.IndexUsageStat, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not configured")
	}
	return s.tsLogger.QueryStorageIndexHealthIndexUsage(ctx, engine, instance, from, to, limit)
}

func (s *MetricsService) TimescaleStorageIndexHealthTableUsage(ctx context.Context, engine, instance, from, to string, limit int) ([]models.TableUsageStat, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not configured")
	}
	return s.tsLogger.QueryStorageIndexHealthTableUsage(ctx, engine, instance, from, to, limit)
}

func (s *MetricsService) TimescaleStorageIndexHealthGrowth(ctx context.Context, engine, instance, from, to string, limit int) ([]models.TableSizeHistory, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not configured")
	}
	return s.tsLogger.QueryStorageIndexHealthTableGrowth(ctx, engine, instance, from, to, limit)
}

func (s *MetricsService) TimescaleStorageIndexHealthDashboard(ctx context.Context, engine, instance, from, to string, dbNames, schemaNames []string, tableLike string) (*hot.StorageIndexHealthDashboard, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not configured")
	}
	return s.tsLogger.QueryStorageIndexHealthDashboard(ctx, engine, instance, from, to, hot.SIHFilters{
		DBNames: dbNames, SchemaNames: schemaNames, TableLike: tableLike,
	})
}

func (s *MetricsService) TimescaleStorageIndexHealthFilterOptions(ctx context.Context, engine, instance, from, to string, dbName, schemaName string) (*hot.SIHFilterOptions, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not configured")
	}
	return s.tsLogger.QueryStorageIndexHealthFilterOptions(ctx, engine, instance, from, to, dbName, schemaName)
}

func (s *MetricsService) GetAllInstanceStatuses() map[string]string {
	statuses := make(map[string]string)
	if s.MsRepo != nil {
		for name, status := range s.MsRepo.GetAllInstanceStatuses() {
			statuses[name] = status
		}
	}
	if s.PgRepo != nil {
		for name, status := range s.PgRepo.GetAllInstanceStatuses() {
			statuses[name] = status
		}
	}
	return statuses
}

// TimescalePing checks connectivity to TimescaleDB when configured (for readiness probes).
func (s *MetricsService) TimescalePing(ctx context.Context) error {
	if s == nil || s.tsHotStorage == nil {
		return fmt.Errorf("timescale not configured")
	}
	return s.tsHotStorage.Pool().Ping(ctx)
}

// FetchPgBestPracticesWithTimescale runs the pg_settings DBA audit. When a recent postgres_settings_snapshot
// exists in Timescale for this instance, current setting (and unit) values are taken from that snapshot for
// any parameter present in both the snapshot and the audit list; boot/default values always come from live pg_settings.
func (s *MetricsService) FetchPgBestPracticesWithTimescale(ctx context.Context, instanceName string) models.BestPracticesResult {
	configs, err := s.PgRepo.QueryPgBestPracticesConfigRows(instanceName)
	if err != nil {
		return models.BestPracticesResult{InstanceName: instanceName}
	}

	result := s.PgRepo.FetchPgBestPracticesFromConfigs(instanceName, configs)
	result.DataSource = "live"

	if s.tsLogger == nil {
		return result
	}

	latestTs, _, snapRows, _, err := s.tsLogger.GetPostgresSettingsSnapshotLatestTwo(ctx, instanceName)
	if err != nil || latestTs.IsZero() || len(snapRows) == 0 {
		return result
	}

	byName := make(map[string]hot.PostgresSettingSnapshotRow, len(snapRows))
	for _, r := range snapRows {
		byName[r.Name] = r
	}

	overlaid := false
	for i := range configs {
		if snap, ok := byName[configs[i].Name]; ok {
			configs[i].Setting = snap.Setting
			if snap.Unit != "" {
				configs[i].Unit = snap.Unit
			}
			overlaid = true
		}
	}
	if !overlaid {
		return result
	}

	out := s.PgRepo.FetchPgBestPracticesFromConfigs(instanceName, configs)
	out.DataSource = "timescale"
	out.SnapshotCapturedAt = latestTs.UTC().Format(time.RFC3339)
	return out
}

// =============================================================================
// Timescale-backed “enterprise metrics” reads (used to avoid direct DMV calls)
// =============================================================================

func (s *MetricsService) GetTimescaleLatchWaits(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetLatchWaits(ctx, instanceName, limit)
}

func (s *MetricsService) GetTimescaleWaitingTasks(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetWaitingTasks(ctx, instanceName, limit)
}

func (s *MetricsService) GetTimescaleMemoryGrants(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetMemoryGrants(ctx, instanceName, limit)
}

func (s *MetricsService) GetTimescaleProcedureStats(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetProcedureStats(ctx, instanceName, limit)
}

func (s *MetricsService) GetTimescaleFileIOLatency(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetFileIOLatency(ctx, instanceName, limit)
}

// WarmFileIOLatencyToTimescale appends one DMV snapshot to Timescale so the dashboard disk-latency
// trend can render on page load instead of waiting only for the Enterprise metrics ticker.
func (s *MetricsService) WarmFileIOLatencyToTimescale(ctx context.Context, instanceName string) {
	if s == nil || s.tsLogger == nil {
		return
	}
	rows, err := s.MsRepo.FetchFileIOLatency(instanceName)
	if err != nil {
		log.Printf("[MetricsService] WarmFileIOLatency fetch failed for %s: %v", instanceName, err)
		return
	}
	if len(rows) == 0 {
		log.Printf("[MetricsService] WarmFileIOLatency: DMV returned no file rows for %s", instanceName)
		return
	}
	if err := s.tsLogger.LogFileIOLatency(ctx, instanceName, rows); err != nil {
		log.Printf("[MetricsService] WarmFileIOLatency Timescale write failed for %s: %v", instanceName, err)
	}
}

func (s *MetricsService) GetTimescaleSpinlockStats(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetSpinlockStats(ctx, instanceName, limit)
}

func (s *MetricsService) GetTimescaleMemoryClerks(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetMemoryClerks(ctx, instanceName, limit)
}

func (s *MetricsService) GetTimescaleTempdbFiles(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetTempdbFiles(ctx, instanceName, limit)
}

func (s *MetricsService) GetTimescalePlanCacheHealth(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetPlanCacheHealth(ctx, instanceName, limit)
}

func (s *MetricsService) GetTimescaleMemoryGrantWaiters(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetMemoryGrantWaiters(ctx, instanceName, limit)
}

func (s *MetricsService) GetTimescaleTempdbTopConsumers(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetTempdbTopConsumers(ctx, instanceName, limit)
}

func (s *MetricsService) GetTimescaleWaitCategoryAgg(ctx context.Context, instanceName string, minutes int) ([]hot.WaitCategoryAgg, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetWaitCategoryAgg(ctx, instanceName, minutes)
}

// GetTimescalePerformanceDebtFindings returns recent Performance Debt findings (Timescale snapshot).
func (s *MetricsService) GetTimescalePerformanceDebtFindings(ctx context.Context, instanceName string, lookback time.Duration) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetLatestPerformanceDebtFindings(ctx, instanceName, lookback)
}

func (s *MetricsService) GetTimescaleSchedulerWG(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetSchedulerWG(ctx, instanceName, limit)
}

func (s *MetricsService) FetchGlobalEstate() []models.GlobalInstanceMetric {
	var metrics []models.GlobalInstanceMetric
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Fetch concurrently
	for _, inst := range s.Config.Instances {
		wg.Add(1)
		go func(instance config.Instance) {
			defer wg.Done()

			// Establish safe timeout threshold
			start := time.Now()
			_ = start

			var m models.GlobalInstanceMetric
			m.Name = instance.Name
			m.Type = instance.Type

			if instance.Type == "postgres" {
				m = s.PgRepo.GetGlobalMetric(instance.Name, m)
			} else {
				m = s.MsRepo.GetGlobalMetric(instance.Name, m)
			}

			mu.Lock()
			metrics = append(metrics, m)
			mu.Unlock()

		}(inst)
	}

	wg.Wait()
	return metrics
}

// InitXeDatabase initializes the SQLite connection for extended events
func (s *MetricsService) InitXeDatabase(sqlitePath string) error {
	abs, err := filepath.Abs(sqlitePath)
	if err != nil {
		return err
	}

	s.xeSqlitePath = abs
	sqliteDSN := fmt.Sprintf("file:%s?mode=rwc&_pragma=foreign_keys(1)", abs)
	db, err := sql.Open("sqlite", sqliteDSN)
	if err != nil {
		return err
	}

	s.xeDb = db
	return nil
}

// GetBestPractices fetches and evaluates best practices configuration for an instance
func (s *MetricsService) GetBestPractices(instanceName string) models.BestPracticesResult {
	return s.MsRepo.FetchBestPractices(instanceName)
}

// GetGuardrails fetches guardrails audit results for an instance
func (s *MetricsService) GetGuardrails(instanceName string) models.GuardrailsResult {
	return s.MsRepo.FetchGuardrails(instanceName)
}

// GetRecentXEvents retrieves recent extended events from SQLite for a given instance
func (s *MetricsService) GetRecentXEvents(instance string) ([]models.SqlServerXeEvent, error) {
	if s.xeDb == nil {
		return []models.SqlServerXeEvent{}, nil
	}

	// Query last 100 events from the last 1 hour, ordered by timestamp DESC
	query := `
		SELECT 
			server_instance_name, event_type, event_timestamp, event_data_xml, 
			parsed_payload_json, file_name, file_offset
		FROM sql_server_xevents
		WHERE server_instance_name = ? 
		  AND event_timestamp > datetime('now', '-1 hour')
		ORDER BY event_timestamp DESC
		LIMIT 100
	`

	rows, err := s.xeDb.Query(query, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.SqlServerXeEvent
	for rows.Next() {
		var e models.SqlServerXeEvent
		var eventTS sql.NullString
		var parsedJSON sql.NullString

		err := rows.Scan(
			&e.ServerInstanceName,
			&e.EventType,
			&eventTS,
			&e.EventDataXML,
			&parsedJSON,
			&e.FileName,
			&e.FileOffset,
		)
		if err != nil {
			return nil, err
		}

		if eventTS.Valid {
			e.EventTimestamp = eventTS.String
		}
		if parsedJSON.Valid {
			e.ParsedPayloadJSON = parsedJSON.String
		}

		events = append(events, e)
	}

	return events, rows.Err()
}

// GetXEventMetrics retrieves aggregated extended event metrics for dashboard
func (s *MetricsService) GetXEventMetrics(instance string) models.XEventMetrics {
	if s.xeDb == nil {
		return models.XEventMetrics{}
	}

	metrics := models.XEventMetrics{
		ServerInstanceName: instance,
		Timestamp:          time.Now().Format("2006-01-02 15:04:05"),
	}

	// Get count of recent events (last 1 hour) by event type
	query := `
		SELECT event_type, COUNT(*) as count
		FROM sql_server_xevents
		WHERE server_instance_name = ?
		  AND event_timestamp > datetime('now', '-1 hour')
		GROUP BY event_type
		ORDER BY count DESC
	`

	rows, err := s.xeDb.Query(query, instance)
	if err != nil {
		return metrics
	}
	defer rows.Close()

	metrics.EventCounts = make(map[string]int)
	totalCount := 0

	for rows.Next() {
		var eventType string
		var count int
		if err := rows.Scan(&eventType, &count); err != nil {
			continue
		}
		metrics.EventCounts[eventType] = count
		totalCount += count
	}

	metrics.TotalEventsLastHour = totalCount

	// Get recent events (last 10)
	recentEvents, _ := s.GetRecentXEvents(instance)
	if len(recentEvents) > 10 {
		metrics.RecentEvents = recentEvents[:10]
	} else {
		metrics.RecentEvents = recentEvents
	}

	return metrics
}

// GetPostgresOverview returns a compact summary from cached Postgres telemetry (no extra DB round-trips).
func (s *MetricsService) GetPostgresOverview(instanceName string) models.InstanceOverview {
	pg := s.GetCachedPgCoreDashboard(instanceName)
	out := models.InstanceOverview{
		InstanceName: instanceName,
		Engine:       "postgres",
		Timestamp:    pg.Timestamp,
	}
	out.DatabaseCount = len(pg.KnownDatabases)

	thr := s.GetCachedPgThroughputDashboard(instanceName, "all")
	if n := len(thr.Tps); n > 0 {
		out.LastTps = thr.Tps[n-1]
	}
	if pct, err := s.PgRepo.FetchCacheHitRatioPct(instanceName); err == nil {
		out.LastCacheHitPct = pct
	} else if n := len(thr.CacheHitPct); n > 0 {
		// Fallback to cached delta-based estimate
		out.LastCacheHitPct = thr.CacheHitPct[n-1]
	}

	// Fetch additional metrics
	active, idle, total, err := s.PgRepo.GetConnectionStats(instanceName)
	if err == nil {
		out.ActiveConns = active
		out.IdleConns = idle
		out.TotalConns = total
	}

	lag, status, err := s.PgRepo.GetReplicationLag(instanceName)
	if err == nil {
		out.ReplicationLag = lag
		out.ReplicationStatus = status
	}

	return out
}

// GetMssqlOverview returns a compact summary from cached SQL Server dashboard metrics.
func (s *MetricsService) GetMssqlOverview(instanceName string) models.InstanceOverview {
	d := s.GetCachedDashboard(instanceName)
	out := models.InstanceOverview{
		InstanceName: instanceName,
		Engine:       "sqlserver",
		Timestamp:    d.Timestamp,
		AvgCPULoad:   d.AvgCPULoad,
		MemoryUsage:  d.MemoryUsage,
		ActiveUsers:  d.ActiveUsers,
		TotalLocks:   d.TotalLocks,
		Deadlocks:    d.Deadlocks,
	}
	if d.TopQueries != nil {
		out.TopQueryCount = len(d.TopQueries)
	}
	return out
}

func (s *MetricsService) GetTimescaleSQLServerMetrics(instanceName string, limit int) ([]hot.SQLServerMetricRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSQLServerMetrics(context.Background(), instanceName, limit)
}

func (s *MetricsService) GetTimescalePostgresThroughput(instanceName string, limit int) ([]hot.PostgresThroughputRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresThroughput(context.Background(), instanceName, limit)
}

func (s *MetricsService) GetTimescalePostgresConnections(instanceName string, limit int) ([]hot.PostgresConnectionRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresConnections(context.Background(), instanceName, limit)
}

func (s *MetricsService) GetTimescalePostgresSystemStats(instanceName string, limit int) ([]hot.PostgresSystemStatsRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresSystemStats(context.Background(), instanceName, limit)
}

func (s *MetricsService) GetTimescalePostgresReplicationSlots(instanceName string, limit int) ([]hot.PostgresReplicationSlotRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresReplicationSlots(context.Background(), instanceName, limit)
}

func (s *MetricsService) GetTimescalePostgresDiskStats(instanceName string, limit int) ([]hot.PostgresDiskStatRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresDiskStats(context.Background(), instanceName, limit)
}

func (s *MetricsService) GetLatestPostgresBackupRun(ctx context.Context, instanceName string) (*hot.PostgresBackupRunRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetLatestPostgresBackupRun(ctx, instanceName)
}

func (s *MetricsService) GetPostgresBackupRunHistory(ctx context.Context, instanceName string, limit int) ([]hot.PostgresBackupRunRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresBackupRunHistory(ctx, instanceName, limit)
}

func (s *MetricsService) LogPostgresBackupRun(ctx context.Context, row hot.PostgresBackupRunRow) error {
	if s.tsLogger == nil {
		return fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.LogPostgresBackupRun(ctx, row)
}

func (s *MetricsService) LogPostgresLogEvents(ctx context.Context, instanceName string, rows []hot.PostgresLogEventRow) error {
	if s.tsLogger == nil {
		return fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.LogPostgresLogEvents(ctx, instanceName, rows)
}

func (s *MetricsService) GetPostgresLogSummary(ctx context.Context, instanceName string, windowMinutes int) (*hot.PostgresLogSummary, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresLogSummary(ctx, instanceName, windowMinutes)
}

func (s *MetricsService) GetPostgresLogEvents(ctx context.Context, instanceName string, limit int, severity string) ([]hot.PostgresLogEventRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresLogEvents(ctx, instanceName, limit, severity)
}

func (s *MetricsService) GetTimescalePostgresVacuumProgress(instanceName string, limit int) ([]hot.PostgresVacuumProgressRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresVacuumProgress(context.Background(), instanceName, limit)
}

func (s *MetricsService) GetPostgresTableMaintenanceHistory(ctx context.Context, instanceName string, schema string, table string, limit int) ([]hot.PostgresTableMaintRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresTableMaintenanceHistory(ctx, instanceName, schema, table, limit)
}

func (s *MetricsService) GetLatestPostgresTableMaintenance(ctx context.Context, instanceName string, limit int) ([]hot.PostgresTableMaintRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetLatestPostgresTableMaintenance(ctx, instanceName, limit)
}

func (s *MetricsService) GetPostgresSessionStateCountsHistory(ctx context.Context, instanceName string, limit int) ([]hot.PostgresSessionStateCountRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresSessionStateCountsHistory(ctx, instanceName, limit)
}

// GetPostgresLockWaitHistory returns timestamps and counts of sessions in Lock wait (from postgres_wait_event_stats).
func (s *MetricsService) GetPostgresLockWaitHistory(ctx context.Context, instanceName string, windowMinutes, maxPoints int) ([]string, []int, error) {
	if s.tsLogger == nil {
		return nil, nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresLockWaitHistory(ctx, instanceName, windowMinutes, maxPoints)
}

func (s *MetricsService) GetLatestPostgresPoolerStats(ctx context.Context, instanceName string) (*hot.PostgresPoolerStatRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetLatestPostgresPoolerStats(ctx, instanceName)
}

func (s *MetricsService) GetPostgresPoolerStatsHistory(ctx context.Context, instanceName string, limit int) ([]hot.PostgresPoolerStatRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresPoolerStatsHistory(ctx, instanceName, limit)
}

func (s *MetricsService) GetPostgresDeadlocksHistory(ctx context.Context, instanceName string, minutes int, limit int) ([]hot.PostgresDeadlockStatRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresDeadlocksHistory(ctx, instanceName, minutes, limit)
}

func (s *MetricsService) GetTimescaleSQLServerTopQueries(instanceName string, limit int, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSQLServerTopQueriesWithRange(context.Background(), instanceName, limit, from, to)
}

// GetTimescaleSQLServerTopQueriesLatest returns recent top-query rows (includes query_text) for CPU drilldown and similar UIs.
func (s *MetricsService) GetTimescaleSQLServerTopQueriesLatest(instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSQLServerTopQueries(context.Background(), instanceName, limit)
}

// GetTimescaleSQLServerConnectionStats returns latest per-database connection snapshots from Timescale.
func (s *MetricsService) GetTimescaleSQLServerConnectionStats(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetLatestSQLServerConnectionSnapshots(ctx, instanceName, limit)
}

// GetTimescaleAGHealthSummary wraps Timescale AG health rollup (last hour).
func (s *MetricsService) GetTimescaleAGHealthSummary(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetAGHealthSummary(ctx, instanceName, limit)
}

// GetTimescaleDatabaseThroughputSummary wraps Timescale DB throughput rollup (last hour).
func (s *MetricsService) GetTimescaleDatabaseThroughputSummary(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetDatabaseThroughputSummary(ctx, instanceName, limit)
}

func (s *MetricsService) GetQueryStatsDashboard(instanceName, metric, timeRange, dimension string, limit int, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	params := hot.QueryStatsDashboardParams{
		InstanceName: instanceName,
		Metric:       metric,
		TimeRange:    timeRange,
		Dimension:    dimension,
		Limit:        limit,
		From:         from,
		To:           to,
	}
	return s.tsLogger.GetQueryStatsDashboard(context.Background(), params)
}

// GetTimescaleSQLServerCPUHistory returns sqlserver_cpu_history points for the given RFC3339 window.
func (s *MetricsService) GetTimescaleSQLServerCPUHistory(instanceName, from, to string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSQLServerCPUHistory(context.Background(), instanceName, from, to, limit)
}

// GetTimescaleSQLServerMemoryDrilldown returns memory_usage (sqlserver_metrics), PLE (sqlserver_memory_history),
// and OS memory fields from sqlserver_cpu_scheduler_stats for the same RFC3339 window as CPU drilldown.
func (s *MetricsService) GetTimescaleSQLServerMemoryDrilldown(instanceName, from, to string, limit int) (map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	ctx := context.Background()
	metrics, err := s.tsLogger.GetSQLServerMetricsRange(ctx, instanceName, from, to, limit)
	if err != nil {
		return nil, err
	}
	ple, err := s.tsLogger.GetSQLServerMemoryHistoryRange(ctx, instanceName, from, to, limit)
	if err != nil {
		return nil, err
	}
	sched, err := s.tsLogger.GetSQLServerSchedulerMemoryRange(ctx, instanceName, from, to, limit)
	if err != nil {
		return nil, err
	}
	mem, err := s.tsLogger.GetSQLServerMemoryMetricsRange(ctx, instanceName, from, to, limit)
	if err != nil {
		return nil, err
	}
	plan, err := s.tsLogger.GetPlanCacheHealthRange(ctx, instanceName, from, to, limit)
	if err != nil {
		plan = []map[string]interface{}{}
	}
	clerks, err := s.tsLogger.GetMemoryClerks(ctx, instanceName, 200)
	if err != nil {
		clerks = []map[string]interface{}{}
	}
	bpdb, err := s.tsLogger.GetSQLServerBufferPoolByDBRange(ctx, instanceName, from, to, 8000)
	if err != nil {
		bpdb = []map[string]interface{}{}
	}

	// Track per-section sources so UI can display "Timescale" vs "Live fallback".
	source := map[string]any{
		"sqlserver_metrics": "timescale",
		"memory_history":    "timescale",
		"scheduler_memory":  "timescale",
		"memory_metrics":    "timescale",
		"plan_cache_health": "timescale",
		"memory_clerks":     "timescale",
		"buffer_pool_by_db": "timescale",
	}

	// Fallback: when Timescale tables are empty (fresh install / collector warming up),
	// return at least one live snapshot so charts don't render blank.
	// This keeps the "Timescale drilldown" page usable even before the first scrape lands.
	if len(mem) == 0 && s.MsRepo != nil {
		if row, err := s.MsRepo.FetchMemoryAnalyzerSnapshot(ctx, instanceName); err == nil && row != nil {
			now := time.Now().UTC()
			row["capture_timestamp"] = now
			row["event_time"] = now.Format(time.RFC3339)
			mem = append(mem, row)
			source["memory_metrics"] = "live_fallback"

			// Also backfill PLE series for the window UI if empty.
			if len(ple) == 0 {
				if v, ok := row["ple_seconds"]; ok {
					ple = append(ple, map[string]interface{}{
						"capture_timestamp":            now,
						"event_time":                   now.Format(time.RFC3339),
						"page_life_expectancy_seconds": v,
						"page_life_expectancy":         v,
					})
					source["memory_history"] = "live_fallback"
				}
			}
		}
	}
	if len(bpdb) == 0 && s.MsRepo != nil {
		if rows, err := s.MsRepo.FetchBufferPoolByDB(ctx, instanceName, 20); err == nil && len(rows) > 0 {
			now := time.Now().UTC()
			for _, r := range rows {
				bpdb = append(bpdb, map[string]interface{}{
					"capture_timestamp": now,
					"event_time":        now.Format(time.RFC3339),
					"database_name":     r["database_name"],
					"buffer_mb":         r["buffer_mb"],
				})
			}
			source["buffer_pool_by_db"] = "live_fallback"
		}
	}
	return map[string]interface{}{
		"instance":          instanceName,
		"from":              from,
		"to":                to,
		"sqlserver_metrics": metrics,
		"memory_history":    ple,
		"scheduler_memory":  sched,
		"memory_metrics":    mem,
		"plan_cache_health": plan,
		"memory_clerks":     clerks,
		"buffer_pool_by_db": bpdb,
		"data_source":       source,
	}, nil
}

func (s *MetricsService) GetQueryStatsTimeSeries(instanceName, metric, timeRange string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetQueryStatsTimeSeries(context.Background(), instanceName, metric, timeRange)
}

func (s *MetricsService) GetTimescaleSQLServerLongRunningQueries(instanceName string, limit int, from, to string, database string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	log.Printf("[MetricsService] GetTimescaleSQLServerLongRunningQueries called for instance=%s limit=%d database=%q", instanceName, limit, database)
	return s.tsLogger.GetSQLServerLongRunningQueries(context.Background(), instanceName, limit, from, to, database)
}

func (s *MetricsService) GetDashboardFromTimescale(instanceName string) (map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}

	ctx := context.Background()
	metrics, err := s.tsLogger.GetSQLServerMetrics(ctx, instanceName, 60)
	if err != nil {
		return nil, err
	}

	topQueries, err := s.tsLogger.GetSQLServerTopQueries(ctx, instanceName, 20)
	if err != nil {
		topQueries = []map[string]interface{}{}
	}

	connStats, err := s.tsLogger.GetLatestSQLServerConnectionSnapshots(ctx, instanceName, 100)
	if err != nil {
		connStats = []map[string]interface{}{}
	}

	result := map[string]interface{}{
		"metrics":           metrics,
		"top_queries":       topQueries,
		"connection_stats":  connStats,
		"connections":       connStats, // legacy key for older clients
		"instance_name":     instanceName,
	}

	return result, nil
}

// GetDashboardHomepageV2 returns the Phase-1 DBA homepage payload using cached metrics only.
// Phase-2 will extend this to prefer TimescaleDB snapshots and add missing signals.
func (s *MetricsService) GetDashboardHomepageV2(instanceName string) dashboard.HomepageV2 {
	d := s.GetCachedDashboard(instanceName)

	// Phase-2: prefer Timescale snapshot for risk/health strip when available.
	// If Timescale is unavailable, we fall back to best-effort cache-derived signals.
	var tsRisk *hot.RiskHealthRow
	var waitAgg []hot.WaitCategoryAgg
	var batchTrend []map[string]interface{}
	var ioTrend []map[string]interface{}
	var bchrTrend []map[string]interface{}
	if s.tsLogger != nil {
		if r, err := s.tsLogger.GetLatestSQLServerRiskHealth(context.Background(), instanceName); err == nil {
			tsRisk = r
		}
		if a, err := s.tsLogger.GetWaitCategoryAgg(context.Background(), instanceName, 15); err == nil {
			waitAgg = a
		}
		if t, err := s.tsLogger.GetBatchRequestsTrend(context.Background(), instanceName, 60); err == nil {
			batchTrend = t
		}
		if t, err := s.tsLogger.GetFileIOLatencyTrend(context.Background(), instanceName, 60); err == nil {
			ioTrend = t
		}
		if t, err := s.tsLogger.GetBufferCacheHitTrend(context.Background(), instanceName, 60); err == nil {
			bchrTrend = t
		}
	}

	// Best-effort derive a few health signals from cached fields.
	var ple *float64
	if n := len(d.PLEHistory); n > 0 {
		v := d.PLEHistory[n-1]
		ple = &v
	}
	var blockingSessions *int
	if d.ActiveBlocks != nil {
		v := len(d.ActiveBlocks)
		blockingSessions = &v
	}

	if tsRisk != nil {
		v := tsRisk.PLE
		ple = &v
		b := tsRisk.BlockingSessions
		blockingSessions = &b
	}

	hs := dashboard.ComputeHealthScore(dashboard.HealthInputs{
		BlockingSessions: blockingSessions,
		PLE:              ple,
		FailedLoginsLast5Min: func() *int {
			if tsRisk == nil {
				return nil
			}
			v := tsRisk.FailedLogins5m
			return &v
		}(),
	})

	out := dashboard.HomepageV2{
		InstanceName: instanceName,
		Timestamp:    d.Timestamp,
		GeneratedAt:  time.Now().UTC(),
		HealthRisk: map[string]any{
			"health_score": hs,
			"blocking_sessions": func() any {
				if blockingSessions == nil {
					return nil
				}
				return *blockingSessions
			}(),
			"ple": func() any {
				if ple == nil {
					return nil
				}
				return *ple
			}(),
			"memory_grants_pending": func() any {
				if tsRisk == nil {
					return nil
				}
				return tsRisk.MemoryGrantsPending
			}(),
			"failed_logins_5m": func() any {
				if tsRisk == nil {
					return nil
				}
				return tsRisk.FailedLogins5m
			}(),
			"tempdb_used_percent": func() any {
				if tsRisk == nil {
					return nil
				}
				return tsRisk.TempdbUsedPercent
			}(),
			"max_log_used_percent": func() any {
				if tsRisk == nil {
					return nil
				}
				return tsRisk.MaxLogUsedPercent
			}(),
			"max_log_db_name": func() any {
				if tsRisk == nil {
					return nil
				}
				return tsRisk.MaxLogDbName
			}(),
		},
		WorkloadCapacity: map[string]any{
			"avg_cpu_load":  d.AvgCPULoad,
			"memory_usage":  d.MemoryUsage,
			"active_users":  d.ActiveUsers,
			"total_locks":   d.TotalLocks,
			"deadlocks":     d.Deadlocks,
			"batch_requests_per_sec": func() any {
				if tsRisk == nil {
					return nil
				}
				return tsRisk.BatchReqPerSec
			}(),
			"compilations_per_sec": func() any {
				if tsRisk == nil {
					return nil
				}
				return tsRisk.CompilationsPerSec
			}(),
			"compilation_ratio": func() any {
				if tsRisk == nil {
					return nil
				}
				return dashboard.CompilationRatio(tsRisk.BatchReqPerSec, tsRisk.CompilationsPerSec)
			}(),
			"compilation_severity": func() any {
				if tsRisk == nil {
					return nil
				}
				return dashboard.CompilationSeverity(dashboard.CompilationRatio(tsRisk.BatchReqPerSec, tsRisk.CompilationsPerSec))
			}(),
		},
		RootCause: map[string]any{
			"wait_history": d.WaitHistory,
			"file_history": d.FileHistory,
			"cpu_history":  d.CPUHistory,
			"wait_categories_15m": func() any {
				if waitAgg == nil {
					return nil
				}
				return waitAgg
			}(),
			"batch_requests_trend_1h": func() any {
				if batchTrend == nil {
					return nil
				}
				return batchTrend
			}(),
			"disk_latency_trend_1h": func() any {
				if ioTrend == nil {
					return nil
				}
				return ioTrend
			}(),
		},
		MemoryStorage: map[string]any{
			"ple_history": d.PLEHistory,
			"mem_history": d.MemHistory,
			"disk_usage":  d.DiskUsage,
			"disk_by_db":  d.DiskByDB,
			"buffer_cache_hit_trend_1h": func() any {
				if bchrTrend == nil {
					return nil
				}
				return bchrTrend
			}(),
		},
		LiveDiagnostics: map[string]any{
			"active_blocks":     d.ActiveBlocks,
			"top_queries":       d.TopQueries,
			"connection_stats":  d.ConnectionStats,
			"xevent_metrics":    d.XEventMetrics,
			"locks_by_db":       d.LocksByDB,
		},
		Compat: map[string]any{
			// Keep legacy dashboard payload available so the frontend can migrate incrementally.
			"dashboard": d,
		},
	}

	return out
}

// GetDashboardHomepageV2WithSource returns the homepage payload and which source was used for Timescale-backed parts.
func (s *MetricsService) GetDashboardHomepageV2WithSource(instanceName string) (dashboard.HomepageV2, string) {
	out := s.GetDashboardHomepageV2(instanceName)
	if s.tsLogger == nil {
		return out, "live_cache"
	}

	// If risk health exists, we treat the homepage as Timescale-backed.
	if _, err := s.tsLogger.GetLatestSQLServerRiskHealth(context.Background(), instanceName); err == nil {
		return out, "timescale"
	}
	return out, "live_cache_fallback"
}

// GetPostgresDBObservationMetrics retrieves DBA-focused health metrics for PostgreSQL
func (s *MetricsService) GetPostgresDBObservationMetrics(instanceName string) repository.DBObservationMetrics {
	metrics, err := s.PgRepo.FetchDBObservationMetrics(instanceName)
	if err != nil {
		log.Printf("[MetricsService] GetPostgresDBObservationMetrics error for %s: %v", instanceName, err)
		return repository.DBObservationMetrics{}
	}
	if metrics == nil {
		return repository.DBObservationMetrics{}
	}
	return *metrics
}

func (s *MetricsService) GetTimescaleCPUSchedulerStats(instanceName string, limit int) ([]hot.CPUSchedulerStatsRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetCPUSchedulerStats(context.Background(), instanceName, limit)
}

func (s *MetricsService) GetTimescaleServerProperties(instanceName string) (*hot.ServerPropertiesRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetServerProperties(context.Background(), instanceName)
}

func (s *MetricsService) GetDashboardWidgets(instanceName string) ([]map[string]interface{}, error) {
	if s.WidgetRepo == nil {
		return nil, fmt.Errorf("widget registry not configured")
	}
	widgets, err := s.WidgetRepo.GetWidgetsByInstance(instanceName)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]interface{}, 0, len(widgets))
	for _, w := range widgets {
		result = append(result, map[string]interface{}{
			"widget_id":         w.WidgetID,
			"dashboard_section": w.DashboardSection,
			"title":             w.Title,
			"chart_type":        w.ChartType,
			"current_sql":       w.CurrentSQL,
			"default_sql":       w.DefaultSQL,
		})
	}
	return result, nil
}

func (s *MetricsService) ExecuteQuery(instanceName string, sql string, timeoutSeconds int) ([]map[string]interface{}, error) {
	if s.WidgetRepo == nil {
		return nil, fmt.Errorf("widget registry not configured")
	}
	return nil, fmt.Errorf("query execution not implemented")
}

func (s *MetricsService) GetQueryBottlenecks(instanceName string, limit int) ([]map[string]interface{}, error) {
	return s.GetQueryStoreBottlenecks(context.Background(), instanceName, "1h", limit, "")
}

func (s *MetricsService) GetQueryBottlenecksWithRange(instanceName, timeRange string, limit int, database string) ([]map[string]interface{}, error) {
	return s.GetQueryStoreBottlenecks(context.Background(), instanceName, timeRange, limit, database)
}

func (s *MetricsService) GetMssqlQueryStoreSQLText(ctx context.Context, instanceName, databaseName, queryHash string) (string, error) {
	if s.MsRepo == nil {
		return "", fmt.Errorf("MsRepo not configured")
	}
	return s.MsRepo.FetchQueryStoreSQLText(instanceName, databaseName, queryHash)
}

func pgQueryDeltaToStat(d hot.PostgresQueryStatsDelta) repository.PgQueryStat {
	return repository.PgQueryStat{
		QueryID:         d.QueryID,
		Query:           d.QueryText,
		Calls:           d.Calls,
		TotalTime:       d.TotalTimeMs,
		MeanTime:        d.MeanTimeMs,
		Rows:            d.Rows,
		TempBlksRead:    d.TempBlksRead,
		TempBlksWritten: d.TempBlksWritten,
		BlkReadTime:     d.BlkReadTimeMs,
		BlkWriteTime:    d.BlkWriteTimeMs,
	}
}

// GetPostgresQueriesForAPI returns stats for a wall-clock window using Timescale snapshots when available; otherwise live cumulative pg_stat_statements.
func (s *MetricsService) GetPostgresQueriesForAPI(ctx context.Context, instanceName string, from, to time.Time) ([]repository.PgQueryStat, map[string]interface{}, error) {
	meta := map[string]interface{}{
		"window_from": from.UTC().Format(time.RFC3339),
		"window_to":   to.UTC().Format(time.RFC3339),
	}
	if s.tsLogger != nil {
		deltas, t0, t1, winNote, err := s.tsLogger.GetPostgresQueryStatsWindowDelta(ctx, instanceName, from, to, 50)
		if err == nil {
			meta["stats_source"] = "timescale_delta"
			meta["baseline_capture"] = t0.UTC().Format(time.RFC3339)
			meta["end_capture"] = t1.UTC().Format(time.RFC3339)
			meta["stats_note"] = "Values are deltas between two snapshots of pg_stat_statements. Total time is the sum of execution times in this window; avg time is total ÷ calls in the window (not a single run’s wall-clock duration)."
			if winNote != "" {
				meta["window_note"] = winNote
			}
			if len(deltas) == 0 {
				meta["stats_note"] = "No query activity recorded between the baseline and end snapshots for this window."
			}
			out := make([]repository.PgQueryStat, 0, len(deltas))
			for _, d := range deltas {
				out = append(out, pgQueryDeltaToStat(d))
			}
			return out, meta, nil
		}
		log.Printf("[MetricsService] postgres query window delta unavailable for %s: %v", instanceName, err)
	}
	live, err := s.PgRepo.GetQueryStats(instanceName)
	if err != nil {
		return nil, meta, err
	}
	meta["stats_source"] = "live_cumulative"
	meta["baseline_capture"] = nil
	meta["end_capture"] = time.Now().UTC().Format(time.RFC3339)
	meta["stats_note"] = "pg_stat_statements holds cumulative counters since the last reset or server start — not limited to the selected time range. “Total time” is the sum of all executions for that statement (many runs add up). “Avg time” is mean milliseconds per execution. For true time-range stats, enable TimescaleDB and the enterprise collector (postgres_query_stats snapshots)."
	return live, meta, nil
}

func (s *MetricsService) instanceType(name string) string {
	for _, inst := range s.Config.Instances {
		if inst.Name == name {
			return inst.Type
		}
	}
	return ""
}
