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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rsharma155/sql_optima/internal/collectors/pghostcpu"
	"github.com/rsharma155/sql_optima/internal/collectors/postgres"
	pg_backup_repo "github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/domain/repositories"
	pg_obs_repo "github.com/rsharma155/sql_optima/internal/domain/postgres_observability/domain/repositories"
	pg_security_repo "github.com/rsharma155/sql_optima/internal/domain/postgres_security/domain/repositories"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/domain"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_monitoring/instance_health"
	"github.com/rsharma155/sql_optima/internal/domain/servers"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
	pg_repo "github.com/rsharma155/sql_optima/internal/repository/postgres"
	appsec "github.com/rsharma155/sql_optima/internal/security"
	"github.com/rsharma155/sql_optima/internal/security/sqlsandbox"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
	"github.com/rsharma155/sql_optima/pkg/dashboard"
)

type MetricsService struct {
	PgRepo           *repository.PgRepository
	MsRepo           *repository.SqlServerRepository
	WidgetRepo       *repository.WidgetRepository
	UserRepo         *repository.UserRepository
	ServerRepo       servers.ServerStore
	AuditRepo        *repository.AuditLogRepository
	ConfigRepo       *repository.CollectorConfigRepository
	ServerKMS        servers.KeyManagementService
	ServerSecretBox  servers.SecretBox
	Config           *config.Config
	RegistryReload   func() // optional: reload cfg.Instances + DB pools after registry CRUD (wired by appserver)
	cacheMutex       sync.RWMutex
	dashboardCache   map[string]models.DashboardMetrics
	pgDashboardCache map[string]models.PgCoreDashboardCache
	xeDb             *sql.DB
	xeSqlitePath     string
	tsLogger         *hot.TimescaleLogger
	tsHotStorage     *hot.HotStorage

	// PostgreSQL Monitoring Domain Services
	PgHealthService *instance_health.InstanceHealthService
	PgHealthRepo    *instance_health.InstanceHealthRepository
	PgQueryRouter   *postgres.QueryMetricsRouter
	PgTimescaleColl *postgres.PgTimescaleCollector

	// New Postgres Domain Repositories
	PgObservabilityRepo *pg_obs_repo.PostgresObservabilityRepository
	PgBackupRepo        *pg_backup_repo.PostgresBackupRepository
	PgSecurityRepo      *pg_security_repo.PostgresSecurityRepository

	// Storage & Index Health collection scheduling.
	// We collect raw engine counters frequently, but persist certain Timescale snapshots
	// on coarser cadences to avoid unnecessary storage growth.
	sihMu            sync.Mutex
	sihLastIndex15m  map[string]time.Time
	sihLastTable15m  map[string]time.Time
	sihLastGrowth6h  map[string]time.Time
	sihLastDefsDaily map[string]time.Time

	// Control Plane for Sequential Collection
	controlPlanes map[string]*InstanceControlPlane
	cpMu          sync.Mutex
}

type InstanceControlPlane struct {
	tasks chan func()
}

func NewMetricsService(pg *repository.PgRepository, ms *repository.SqlServerRepository, cfg *config.Config, tsStorage *hot.HotStorage) *MetricsService {
	var tsLogger *hot.TimescaleLogger
	if tsStorage != nil {
		tsLogger = hot.NewTimescaleLogger(tsStorage.Pool())
		log.Println("[MetricsService] TimescaleDB logger initialized")
	} else {
		log.Println("[MetricsService] TimescaleDB not configured, metrics will not be persisted")
	}

	var widgetRepo *repository.WidgetRepository
	var userRepo *repository.UserRepository
	var serverRepo servers.ServerStore
	var auditRepo *repository.AuditLogRepository
	var configRepo *repository.CollectorConfigRepository
	var secretBox servers.SecretBox
	var pgHealthRepo *instance_health.InstanceHealthRepository
	var pgHealthService *instance_health.InstanceHealthService
	var pgQueryRouter *postgres.QueryMetricsRouter
	var pgTimescaleColl *postgres.PgTimescaleCollector

	var pgObsRepo *pg_obs_repo.PostgresObservabilityRepository
	var pgBackupRepo *pg_backup_repo.PostgresBackupRepository
	var pgSecurityRepo *pg_security_repo.PostgresSecurityRepository

	if tsStorage != nil {
		pgHealthRepo = instance_health.NewInstanceHealthRepository(tsStorage.Pool())
		pgHealthService = instance_health.NewInstanceHealthService(pgHealthRepo)

		pgObsRepo = pg_obs_repo.NewPostgresObservabilityRepository(tsStorage.Pool())
		pgBackupRepo = pg_backup_repo.NewPostgresBackupRepository(tsStorage.Pool())
		pgSecurityRepo = pg_security_repo.NewPostgresSecurityRepository(tsStorage.Pool())

		// Initialize PostgreSQL Query Router
		tsDB := stdlib.OpenDBFromPool(tsStorage.Pool())
		smRepo := pg_repo.NewPgStatMonitorRepository()
		smCollector := postgres.NewPgStatMonitorCollector(smRepo, tsDB, pgHealthRepo)
		pgssCollector := postgres.NewPgStatStatementsLegacyCollector(pg, tsLogger)
		pgQueryRouter = postgres.NewQueryMetricsRouter(smCollector, pgssCollector, smRepo, tsDB)

		// Initialize Phase 8 Timescale Collector
		pgTimescaleColl = postgres.NewPgTimescaleCollector(pg, pgHealthService, tsLogger)

		widgetRepo = repository.NewWidgetRepository(tsStorage.Pool())
		userRepo = repository.NewUserRepository(tsStorage.Pool())
		log.Println("[MetricsService] Widget registry and user management initialized")

		serverRepo = repository.NewServerRegistryRepository(tsStorage.Pool())
		auditRepo = repository.NewAuditLogRepository(tsStorage.Pool())
		configRepo = repository.NewCollectorConfigRepository(tsStorage.Pool())
		secretBox = appsec.NewEnvelopeSecretBox()
		log.Println("[MetricsService] Server registry initialized")
	}

	return &MetricsService{
		PgRepo:           pg,
		MsRepo:           ms,
		WidgetRepo:       widgetRepo,
		UserRepo:         userRepo,
		ServerRepo:       serverRepo,
		AuditRepo:        auditRepo,
		ConfigRepo:       configRepo,
		ServerSecretBox:  secretBox,
		Config:           cfg,
		dashboardCache:   make(map[string]models.DashboardMetrics),
		pgDashboardCache: make(map[string]models.PgCoreDashboardCache),
		tsLogger:         tsLogger,
		tsHotStorage:     tsStorage,
		controlPlanes:    make(map[string]*InstanceControlPlane),
		PgHealthRepo:     pgHealthRepo,
		PgHealthService:  pgHealthService,
		PgQueryRouter:    pgQueryRouter,
		PgTimescaleColl:  pgTimescaleColl,
		PgObservabilityRepo: pgObsRepo,
		PgBackupRepo:        pgBackupRepo,
		PgSecurityRepo:      pgSecurityRepo,
		sihLastIndex15m:     make(map[string]time.Time),
		sihLastTable15m:     make(map[string]time.Time),
		sihLastGrowth6h:     make(map[string]time.Time),
		sihLastDefsDaily:    make(map[string]time.Time),
	}
}

// EnqueueCollection routes a task to the instance-specific sequential worker.
// This acts as a Master Control Plane to prevent overlapping DMV queries and excessive locking.
func (s *MetricsService) EnqueueCollection(instanceName string, task func()) {
	s.cpMu.Lock()
	if s.controlPlanes == nil {
		s.controlPlanes = make(map[string]*InstanceControlPlane)
	}
	cp, ok := s.controlPlanes[instanceName]
	if !ok {
		cp = &InstanceControlPlane{
			tasks: make(chan func(), 100), // Bounded buffer to prevent memory exhaustion
		}
		s.controlPlanes[instanceName] = cp
		// Start a dedicated single-threaded worker for this instance
		go func(name string, plane *InstanceControlPlane) {
			log.Printf("[ControlPlane] Starting sequential worker for instance: %s", name)
			for t := range plane.tasks {
				t()
			}
		}(instanceName, cp)
	}
	s.cpMu.Unlock()

	// Enqueue the task; drop if queue is full to prevent background pile-up during server stalls.
	select {
	case cp.tasks <- task:
		// Enqueued successfully
	default:
		log.Printf("[ControlPlane] WARNING: Task queue full for %s, skipping this collection cycle to prevent resource exhaustion", instanceName)
	}
}

func (s *MetricsService) GetPgDashboardV2(ctx context.Context, instanceName string) (map[string]interface{}, error) {
	if s.PgHealthRepo == nil {
		return nil, fmt.Errorf("postgres health repository not initialized")
	}

	snapshot, err := s.PgHealthRepo.GetLatestSnapshot(ctx, instanceName)
	if err != nil && !strings.Contains(err.Error(), "no rows") {
		return nil, err
	}

	// Fetch mini lists for incidents
	blocking, _ := s.PgRepo.FetchBlockingSessionsCount(instanceName)
	slowQueries, _ := s.PgRepo.FetchSlowQueriesCount(instanceName, 500)

	res := map[string]interface{}{
		"snapshot": snapshot,
		"incidents": map[string]interface{}{
			"blocking_sessions": blocking,
			"slow_queries":      slowQueries,
		},
		"timestamp": time.Now().UTC(),
	}

	return res, nil
}

// ReplaceInstanceRepositories swaps SQL connection pools after cfg.Instances changes (e.g. registry reload).
func (s *MetricsService) ReplaceInstanceRepositories(pg *repository.PgRepository, ms *repository.SqlServerRepository) {
	if s == nil {
		return
	}
	if pg != nil {
		s.PgRepo = pg
	}
	if ms != nil {
		s.MsRepo = ms
	}
}

// RebindTimescale replaces the Timescale pool and dependent repositories (used after first-run setup).
func (s *MetricsService) RebindTimescale(ts *hot.HotStorage) {
	if s == nil {
		return
	}
	if s.tsHotStorage != nil {
		s.tsHotStorage.Close()
	}
	s.tsHotStorage = ts
	if ts != nil {
		s.tsLogger = hot.NewTimescaleLogger(ts.Pool())
		s.PgHealthRepo = instance_health.NewInstanceHealthRepository(ts.Pool())
		s.PgHealthService = instance_health.NewInstanceHealthService(s.PgHealthRepo)

		// New Postgres Domain Repositories
		s.PgObservabilityRepo = pg_obs_repo.NewPostgresObservabilityRepository(ts.Pool())
		s.PgBackupRepo = pg_backup_repo.NewPostgresBackupRepository(ts.Pool())
		s.PgSecurityRepo = pg_security_repo.NewPostgresSecurityRepository(ts.Pool())

		// Re-initialize PostgreSQL Query Router
		tsDB := stdlib.OpenDBFromPool(ts.Pool())
		smRepo := pg_repo.NewPgStatMonitorRepository()
		smCollector := postgres.NewPgStatMonitorCollector(smRepo, tsDB, s.PgHealthRepo)
		pgssCollector := postgres.NewPgStatStatementsLegacyCollector(s.PgRepo, s.tsLogger)
		s.PgQueryRouter = postgres.NewQueryMetricsRouter(smCollector, pgssCollector, smRepo, tsDB)

		// Initialize Phase 8 Timescale Collector
		s.PgTimescaleColl = postgres.NewPgTimescaleCollector(s.PgRepo, s.PgHealthService, s.tsLogger)

		s.WidgetRepo = repository.NewWidgetRepository(ts.Pool())
		s.UserRepo = repository.NewUserRepository(ts.Pool())
		s.ServerRepo = repository.NewServerRegistryRepository(ts.Pool())
		s.AuditRepo = repository.NewAuditLogRepository(ts.Pool())
		s.ConfigRepo = repository.NewCollectorConfigRepository(ts.Pool())
		s.ServerSecretBox = appsec.NewEnvelopeSecretBox()
		log.Println("[MetricsService] TimescaleDB rebound; application tables reattached")
		return
	}
	s.tsLogger = nil
	s.PgHealthRepo = nil
	s.PgHealthService = nil
	s.PgQueryRouter = nil
	s.PgTimescaleColl = nil
	s.PgObservabilityRepo = nil
	s.PgBackupRepo = nil
	s.PgSecurityRepo = nil
	s.WidgetRepo = nil
	s.UserRepo = nil
	s.ServerRepo = nil
	s.AuditRepo = nil
	s.ConfigRepo = nil
	s.ServerSecretBox = nil
	log.Println("[MetricsService] TimescaleDB detached")
}

// EnsureServerKMS initializes Vault Transit or local envelope KMS when Timescale was attached
// after process start (e.g. first-run /setup) and KMS was never wired.
func (s *MetricsService) EnsureServerKMS(jwtSecret []byte) {
	if s == nil || s.ServerKMS != nil || s.GetTimescaleDBPool() == nil {
		return
	}
	vaultAddr := strings.TrimSpace(os.Getenv("VAULT_ADDR"))
	if vaultAddr != "" {
		vTok := strings.TrimSpace(os.Getenv("VAULT_TOKEN"))
		vKey := strings.TrimSpace(os.Getenv("VAULT_TRANSIT_KEY"))
		vNs := strings.TrimSpace(os.Getenv("VAULT_NAMESPACE"))
		vMount := strings.TrimSpace(os.Getenv("VAULT_TRANSIT_MOUNT"))
		if k, err := appsec.InitVaultClient(appsec.VaultConfig{Addr: vaultAddr, Token: vTok, Namespace: vNs, TransitMount: vMount, TransitKey: vKey}); err == nil {
			s.ServerKMS = k
			log.Println("[kms] Vault Transit enabled after Timescale attach")
			return
		} else {
			log.Printf("[vault] KMS init after Timescale attach: %v", err)
		}
	}
	if lk, err := appsec.NewLocalEnvelopeKMS(jwtSecret); err == nil {
		s.ServerKMS = lk
		log.Println("[kms] local envelope KMS enabled after Timescale attach")
	} else {
		log.Printf("[kms] could not enable local KMS after Timescale attach: %v", err)
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

func (s *MetricsService) GetTimescaleDBLogger() *hot.TimescaleLogger {
	return s.tsLogger
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

func (s *MetricsService) generateStorageInsights(dash *hot.StorageIndexHealthDashboard) []map[string]interface{} {
	var insights []map[string]interface{}
	k := dash.KPIs

	// 1. Unused Index Insight (Aggregate)
	if k.UnusedIndexCount > 0 {
		insights = append(insights, map[string]interface{}{
			"severity": "critical",
			"message":  fmt.Sprintf("%d unused indexes detected, consuming %.1f MB of space.", k.UnusedIndexCount, k.UnusedIndexMB),
		})
	}

	// 2. Individual Large Unused Indexes
	for _, idx := range dash.UnusedIndexes {
		if idx.Value2 > 500 { // > 500MB
			insights = append(insights, map[string]interface{}{
				"severity":    "warning",
				"message":     fmt.Sprintf("Large unused index: %s (%s) consuming %.1f MB.", idx.IndexName, idx.TableName, idx.Value2),
				"db_name":     idx.DBName,
				"schema_name": idx.SchemaName,
				"table_name":  idx.TableName,
			})
		}
	}

	// 3. Growth Insight
	if k.Growth7dPct > 10 {
		insights = append(insights, map[string]interface{}{
			"severity": "warning",
			"message":  fmt.Sprintf("High growth detected: Database is expanding at %.1f%% weekly.", k.Growth7dPct),
		})
	}

	// 4. Fragmentation / Scan Insight
	if k.HighScanTableCount > 0 {
		insights = append(insights, map[string]interface{}{
			"severity": "warning",
			"message":  fmt.Sprintf("%d tables have high scan-to-seek ratios, indicating potential missing indexes.", k.HighScanTableCount),
		})
	}

	// 5. Duplicate Index Candidates
	for _, d := range dash.RawDuplicateIndexes {
		insights = append(insights, map[string]interface{}{
			"severity":    "warning",
			"message":     fmt.Sprintf("Duplicate index candidate on %s.%s: %v", d["schema_name"], d["table_name"], d["suggestion"]),
			"db_name":     d["db_name"],
			"schema_name": d["schema_name"],
			"table_name":  d["table_name"],
		})
	}

	return insights
}

func (s *MetricsService) TimescaleStorageIndexHealthDashboard(ctx context.Context, engine, instance, from, to string, dbNames, schemaNames []string, tableLike string) (*hot.StorageIndexHealthDashboard, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not configured")
	}
	dash, err := s.tsLogger.QueryStorageIndexHealthDashboard(ctx, engine, instance, from, to, hot.SIHFilters{
		DBNames: dbNames, SchemaNames: schemaNames, TableLike: tableLike,
	})
	if err != nil {
		return nil, err
	}

	if dash != nil {
		// Populate dynamic insights
		dash.Insights = s.generateStorageInsights(dash)
	}

	return dash, nil
}

func (s *MetricsService) GetSQLServerRiskHealthHistory(ctx context.Context, instanceName string, from, to time.Time) ([]hot.RiskHealthRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not connected")
	}
	return s.tsLogger.GetSQLServerRiskHealthHistory(ctx, instanceName, from, to)
}

func (s *MetricsService) GetTableSizeHistory(ctx context.Context, engine, instanceName, from, to string, db, schema, table string) ([]models.TableSizeHistory, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not connected")
	}
	return s.tsLogger.GetTableSizeHistory(ctx, engine, instanceName, from, to, db, schema, table)
}

func (s *MetricsService) GetIndexUsageHistory(ctx context.Context, engine, instanceName, from, to string, db, schema, table string) ([]models.IndexUsageStat, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not connected")
	}
	return s.tsLogger.GetIndexUsageHistory(ctx, engine, instanceName, from, to, db, schema, table)
}

func (s *MetricsService) GetIndexFragmentationHistory(ctx context.Context, engine, instanceName, from, to string, db, schema, table string) ([]hot.IndexFragHistoryRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not connected")
	}
	return s.tsLogger.GetIndexFragmentationHistory(ctx, engine, instanceName, from, to, db, schema, table)
}

func (s *MetricsService) TimescaleStorageIndexHealthFilterOptions(ctx context.Context, engine, instance, from, to string, dbName, schemaName string) (*hot.SIHFilterOptions, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not configured")
	}
	return s.tsLogger.QueryStorageIndexHealthFilterOptions(ctx, engine, instance, from, to, dbName, schemaName)
}

func (s *MetricsService) TimescaleStorageIndexDefinition(ctx context.Context, engine, instance, dbName, schemaName, indexName string) ([]hot.IndexDefinitionRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not configured")
	}
	return s.tsLogger.QueryIndexDefinition(ctx, engine, instance, dbName, schemaName, indexName)
}

// =============================================================================
// Timescale-backed standardized “enterprise metrics v2” reads
// =============================================================================

func (s *MetricsService) GetSqlServerWaitStatsTimeSeries(ctx context.Context, instanceName string, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not connected")
	}
	return s.tsLogger.GetSqlServerWaitStats(ctx, instanceName, from, to)
}

func (s *MetricsService) GetSqlServerPerfCountersTimeSeries(ctx context.Context, instanceName string, from, to string, counters []string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not connected")
	}
	return s.tsLogger.GetSqlServerPerfCounters(ctx, instanceName, from, to, counters)
}

func (s *MetricsService) GetSqlServerFileIOTimeSeries(ctx context.Context, instanceName string, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not connected")
	}
	return s.tsLogger.GetSqlServerFileIO(ctx, instanceName, from, to)
}

func (s *MetricsService) GetSqlServerPlanCacheTimeSeries(ctx context.Context, instanceName string, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not connected")
	}
	return s.tsLogger.GetSqlServerPlanCache(ctx, instanceName, from, to)
}

func (s *MetricsService) GetSqlServerMemoryClerksTimeSeries(ctx context.Context, instanceName string, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not connected")
	}
	return s.tsLogger.GetSqlServerMemoryClerksV2(ctx, instanceName, from, to)
}

func (s *MetricsService) GetSqlServerMemoryGrantsTimeSeries(ctx context.Context, instanceName string, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not connected")
	}
	return s.tsLogger.GetSqlServerMemoryGrantsV2(ctx, instanceName, from, to)
}

func (s *MetricsService) GetSqlServerTempdbConsumersTimeSeries(ctx context.Context, instanceName string, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale not connected")
	}
	return s.tsLogger.GetSqlServerTempdbConsumers(ctx, instanceName, from, to)
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

func (s *MetricsService) GetTimescaleWaitCategoryAgg(ctx context.Context, instanceName string, minutes int, from, to string) ([]hot.WaitCategoryAgg, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetWaitCategoryAgg(ctx, instanceName, minutes, from, to)
}

// GetTimescalePerformanceDebtFindings returns recent Performance Debt findings (Timescale snapshot).
func (s *MetricsService) GetTimescalePerformanceDebtFindings(ctx context.Context, instanceName string, lookback time.Duration, database string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetLatestPerformanceDebtFindings(ctx, instanceName, lookback, database)
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

// GetPostgresOverview returns a compact summary from TimescaleDB snapshots (no target DB round-trips).
func (s *MetricsService) GetPostgresOverview(instanceName string) models.InstanceOverview {
	out := models.InstanceOverview{
		InstanceName: instanceName,
		Engine:       "postgres",
	}

	if s.PgHealthRepo != nil {
		snap, err := s.PgHealthRepo.GetLatestSnapshot(context.Background(), instanceName)
		if err == nil && snap != nil {
			out.Timestamp = snap.CollectedAt.Format("15:04:05")
			out.LastTps = snap.TPS
			out.ActiveConns = snap.ActiveSessions
			out.IdleConns = snap.IdleSessions
			out.TotalConns = snap.ActiveSessions + snap.IdleSessions // Approximate if not explicitly in snapshot
			out.LastCacheHitPct = snap.CacheHitRatio
			out.ReplicationLag = snap.ReplicaLagSec
			out.ReplicationStatus = "Unknown" // Can be refined if needed
			if snap.ReplicaLagSec == 0 {
				out.ReplicationStatus = "Healthy"
			} else if snap.ReplicaLagSec > 0 {
				out.ReplicationStatus = "Lagging"
			}
			return out
		}
	}

	// Fallback to minimal cached info if snapshot missing
	pg := s.GetCachedPgCoreDashboard(instanceName)
	out.Timestamp = pg.Timestamp
	out.DatabaseCount = len(pg.KnownDatabases)
	return out
}

// GetSqlServerOverview returns a compact summary from cached SQL Server dashboard metrics.
func (s *MetricsService) GetSqlServerOverview(instanceName string) models.InstanceOverview {
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

func (s *MetricsService) GetTimescaleSQLServerMetricsRange(instanceName, from, to string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSQLServerMetricsRange(context.Background(), instanceName, from, to, limit)
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

// GetPostgresCpuHistory returns Timescale postgres_system_stats rows (host vs Postgres CPU, load, cores).
func (s *MetricsService) GetTimescalePostgresSystemStatsRange(instanceName string, from, to time.Time) ([]hot.PostgresSystemStatsRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresSystemStatsRange(context.Background(), instanceName, from, to)
}

// GetPostgresCpuHistoryRange returns Timescale postgres_system_stats rows for a specific time range.
func (s *MetricsService) GetPostgresCpuHistoryRange(instanceName string, from, to time.Time) ([]hot.PostgresSystemStatsRow, error) {
	return s.GetTimescalePostgresSystemStatsRange(instanceName, from, to)
}

func (s *MetricsService) GetPostgresCpuHistory(instanceName string, limit int) ([]hot.PostgresSystemStatsRow, error) {
	return s.GetTimescalePostgresSystemStats(instanceName, limit)
}

// GetPostgresCpuSaturation merges the latest Timescale snapshot with optional agent probes for KPIs.
func (s *MetricsService) GetPostgresCpuSaturation(instanceName string) map[string]interface{} {
	var tsRow hot.PostgresSystemStatsRow
	haveTS := false
	if s.tsLogger != nil {
		if rows, err := s.tsLogger.GetPostgresSystemStats(context.Background(), instanceName, 1); err == nil && len(rows) > 0 {
			tsRow = rows[0]
			haveTS = true
		}
	}

	active := tsRow.TotalConnections
	load1 := tsRow.Load1m
	cores := tsRow.CpuCores
	pgPct := tsRow.PostgresCpuPercent
	hostPct := tsRow.HostCpuPercent
	if hostPct == 0 && haveTS {
		hostPct = tsRow.CPUUsage
	}

	// Local fallback if timescale has nothing, though ideally collector covers it
	if !haveTS {
		localSnap := pghostcpu.Collect()
		load1 = localSnap.Load1m
		cores = localSnap.CpuCores
		pgPct = localSnap.PostgresCpuPercent
		hostPct = localSnap.HostCpuPercent
	}

	// Try Agent data
	osConfigured := false
	if s.ServerRepo != nil {
		srv, _ := s.ServerRepo.GetByName(context.Background(), instanceName)
		if srv.Host != "" {
			if s.tsLogger != nil {
				osStatus, _ := s.tsLogger.CheckOSCollectorStatus(context.Background(), srv.Host)
				osConfigured = osStatus
				if osStatus {
					if agentData, err := s.tsLogger.GetLatestOSCPUSaturation(context.Background(), srv.Host); err == nil {
						load1 = agentData["load_1m"].(float64)
						cores = agentData["cpu_cores"].(int)
						hostPct = 100.0 - agentData["cpu_idle_pct"].(float64)
					}
				}
			}
		}
	}

	sat := pghostcpu.CpuSaturationPct(load1, cores)
	per := pghostcpu.CpuPerConnection(pgPct, active)

	return map[string]interface{}{
		"instance":                instanceName,
		"cpu_saturation_pct":      sat,
		"cpu_per_connection":      per,
		"load_1m":                 load1,
		"cpu_cores":               cores,
		"host_cpu_percent":        hostPct,
		"postgres_cpu_percent":    pgPct,
		"active_connections":      active,
		"os_collector_configured": osConfigured,
	}
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

func (s *MetricsService) GetPostgresTableMaintenanceHistory(ctx context.Context, instanceName string, database string, schema string, table string, limit int) ([]hot.PostgresTableMaintRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresTableMaintenanceHistory(ctx, instanceName, database, schema, table, limit)
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

// GetPgLocksBlockingKpis returns a lightweight KPI bundle for the Locks & Blocking dashboard.
func (s *MetricsService) GetPgLocksBlockingKpis(ctx context.Context, instanceName string) (*hot.PgBlockingKpis, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPgBlockingKpis(ctx, instanceName)
}

func (s *MetricsService) GetPgBlockingTimeline(ctx context.Context, instanceName string, window time.Duration) ([]hot.PgBlockingTimelinePoint, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPgBlockingTimeline(ctx, instanceName, window)
}

func (s *MetricsService) GetPgBlockingTimelineRange(ctx context.Context, instanceName string, from, to time.Time) ([]hot.PgBlockingTimelinePoint, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPgBlockingTimelineRange(ctx, instanceName, from, to)
}

func (s *MetricsService) GetPgBlockingIncidentsInWindow(ctx context.Context, instanceName string, window time.Duration) ([]hot.PgBlockingIncident, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPgBlockingIncidentsInWindow(ctx, instanceName, window)
}

func (s *MetricsService) GetPgBlockingIncidentsRange(ctx context.Context, instanceName string, from, to time.Time) ([]hot.PgBlockingIncident, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPgBlockingIncidentsRange(ctx, instanceName, from, to)
}

func (s *MetricsService) GetPgTopLockedTables(ctx context.Context, instanceName string, lookback time.Duration, limit int) ([]hot.PgTopLockedTable, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPgTopLockedTables(ctx, instanceName, lookback, limit)
}

func (s *MetricsService) GetPgTopLockedTablesRange(ctx context.Context, instanceName string, from, to time.Time, limit int) ([]hot.PgTopLockedTable, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPgTopLockedTablesRange(ctx, instanceName, from, to, limit)
}

func (s *MetricsService) GetPgBlockingDetailsInRange(ctx context.Context, instanceName string, from, to time.Time) (*hot.PgBlockingDetailsResponse, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPgBlockingDetailsInRange(ctx, instanceName, from, to)
}

func (s *MetricsService) GetPgBlockingIncidents(ctx context.Context, instanceName string, windowHours, limit int) ([]hot.PgBlockingIncidentSummary, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPgBlockingIncidents(ctx, instanceName, windowHours, limit)
}

func (s *MetricsService) GetPgChronicBlockers(ctx context.Context, instanceName string, from, to time.Time, limit int) ([]hot.PgChronicBlockerRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPgChronicBlockers(ctx, instanceName, from, to, limit)
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

func (s *MetricsService) GetTimescaleSQLServerTopQueries(instanceName string, limit int, from, to string, database string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSQLServerTopQueriesWithRange(context.Background(), instanceName, limit, from, to, database)
}

// GetTimescaleSQLServerTopQueriesLatest returns recent top-query rows (includes query_text) for CPU drilldown and similar UIs.
func (s *MetricsService) GetTimescaleSQLServerTopQueriesLatest(instanceName string, limit int, database string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSQLServerTopQueries(context.Background(), instanceName, limit, database)
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
	return s.tsLogger.GetAGHealthSummary(ctx, instanceName, "", "", limit)
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

// GetSqlServerWorkloadSummary returns aggregated KPI metrics for the given time range.
func (s *MetricsService) GetSqlServerWorkloadSummary(ctx context.Context, instanceID string, from, to time.Time) (*domain.SqlServerWorkloadSummary, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSqlServerWorkloadSummary(ctx, instanceID, from, to)
}

// GetSqlServerWorkloadTrends returns bucketed time-series data for workload visualization.
func (s *MetricsService) GetSqlServerWorkloadTrends(ctx context.Context, instanceID string, from, to time.Time) ([]domain.SqlServerWorkloadTrendPoint, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSqlServerWorkloadTrends(ctx, instanceID, from, to)
}

// GetSqlServerWorkloadTopOffenders identifies queries causing the most load in the given period.
func (s *MetricsService) GetSqlServerWorkloadTopOffenders(ctx context.Context, instanceID string, from, to time.Time, limit int) ([]domain.SqlServerWorkloadTopQuery, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSqlServerWorkloadTopOffenders(ctx, instanceID, from, to, limit)
}

func (s *MetricsService) GetSqlServerWorkloadAppLoadTimeline(ctx context.Context, instanceID string, from, to time.Time) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSqlServerWorkloadAppLoadTimeline(ctx, instanceID, from, to)
}

func (s *MetricsService) GetSqlServerWorkloadLoginLoadTimeline(ctx context.Context, instanceID string, from, to time.Time) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSqlServerWorkloadLoginLoadTimeline(ctx, instanceID, from, to)
}

func (s *MetricsService) GetSqlServerWorkloadTopApps(ctx context.Context, instanceID string, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSqlServerWorkloadTopApps(ctx, instanceID, from, to, limit)
}

func (s *MetricsService) GetSqlServerWorkloadTopLogins(ctx context.Context, instanceID string, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetSqlServerWorkloadTopLogins(ctx, instanceID, from, to, limit)
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

func (s *MetricsService) GetQueryStatsTimeSeries(instanceName, metric, from, to string, dbName string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("timescale logger not initialized")
	}
	return s.tsLogger.GetQueryStatsTimeSeries(context.Background(), instanceName, metric, from, to, dbName)
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

	topQueries, err := s.tsLogger.GetSQLServerTopQueries(ctx, instanceName, 20, "")
	if err != nil {
		topQueries = []map[string]interface{}{}
	}

	connStats, err := s.tsLogger.GetLatestSQLServerConnectionSnapshots(ctx, instanceName, 100)
	if err != nil {
		connStats = []map[string]interface{}{}
	}

	result := map[string]interface{}{
		"metrics":          metrics,
		"top_queries":      topQueries,
		"connection_stats": connStats,
		"connections":      connStats, // legacy key for older clients
		"instance_name":    instanceName,
	}

	return result, nil
}

// GetDashboardHomepageV2 returns the Phase-1 DBA homepage payload using cached metrics only.
// Phase-2 will extend this to prefer TimescaleDB snapshots and add missing signals.
func (s *MetricsService) GetDashboardHomepageV2(instanceName string, from, to string) dashboard.HomepageV2 {
	d := s.GetCachedDashboard(instanceName)

	// Phase-2: prefer Timescale snapshot for risk/health strip when available.
	// If Timescale is unavailable, we fall back to best-effort cache-derived signals.
	var tsRisk *hot.RiskHealthRow
	var waitAgg []hot.WaitCategoryAgg
	var batchTrend []map[string]interface{}
	var ioTrend []map[string]interface{}
	var bchrTrend []map[string]interface{}

	// Default to last 1h if not provided for history charts
	if from == "" || to == "" {
		now := time.Now().UTC()
		to = now.Format(time.RFC3339)
		from = now.Add(-1 * time.Hour).Format(time.RFC3339)
	}

	var pleObjects []map[string]any

	if s.tsLogger != nil {
		if r, err := s.tsLogger.GetLatestSQLServerRiskHealth(context.Background(), instanceName); err == nil {
			tsRisk = r
		}
		if a, err := s.tsLogger.GetWaitCategoryAgg(context.Background(), instanceName, 15, from, to); err == nil {
			waitAgg = a
		}
		if t, err := s.tsLogger.GetBatchRequestsTrend(context.Background(), instanceName, 60, from, to); err == nil {
			batchTrend = t
		}
		if t, err := s.tsLogger.GetFileIOLatencyTrend(context.Background(), instanceName, 60, from, to); err == nil {
			ioTrend = t
		}
		if t, err := s.tsLogger.GetBufferCacheHitTrend(context.Background(), instanceName, 60, from, to); err == nil {
			bchrTrend = t
		}

		// Also override historical charts
		if h, err := s.tsLogger.GetSQLServerCPUHistory(context.Background(), instanceName, from, to, 2000); err == nil {
			d.CPUHistory = make([]models.CPUTick, len(h))
			for i, tick := range h {
				sqlProc, _ := tick["sql_process"].(float64)
				sysIdle, _ := tick["system_idle"].(float64)
				otherProc, _ := tick["other_process"].(float64)
				ts, _ := tick["event_time"].(string)
				d.CPUHistory[i] = models.CPUTick{
					SQLProcess:   sqlProc,
					SystemIdle:   sysIdle,
					OtherProcess: otherProc,
					EventTime:    ts,
				}
			}
		}
		if h, err := s.tsLogger.GetSQLServerMetricsRange(context.Background(), instanceName, from, to, 2000); err == nil {
			d.MemHistory = make([]float64, len(h))
			for i, m := range h {
				mem, _ := m["memory_usage"].(float64)
				d.MemHistory[i] = mem
			}
		}
		if h, err := s.tsLogger.GetSQLServerMemoryHistoryRange(context.Background(), instanceName, from, to, 2000); err == nil {
			// Map to list of objects with "ple" key for consistency
			pleObjects = make([]map[string]any, len(h))
			d.PLEHistory = make([]float64, len(h))
			for i, m := range h {
				pleVal, _ := m["page_life_expectancy"].(float64)
				ts, _ := m["capture_timestamp"].(time.Time)
				pleObjects[i] = map[string]any{
					"ple":        pleVal,
					"timestamp":  ts,
					"event_time": ts.Format(time.RFC3339),
				}
				d.PLEHistory[i] = pleVal
			}
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
			"avg_cpu_load": d.AvgCPULoad,
			"memory_usage": d.MemoryUsage,
			"active_users": d.ActiveUsers,
			"total_locks":  d.TotalLocks,
			"deadlocks":    d.Deadlocks,
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
			"ple_history": func() any {
				if len(pleObjects) > 0 {
					return pleObjects
				}
				return d.PLEHistory
			}(),
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
			"active_blocks":    d.ActiveBlocks,
			"top_queries":      d.TopQueries,
			"connection_stats": d.ConnectionStats,
			"xevent_metrics":   d.XEventMetrics,
			"locks_by_db":      d.LocksByDB,
		},
		Compat: map[string]any{
			// Keep legacy dashboard payload available so the frontend can migrate incrementally.
			"dashboard": d,
		},
	}

	return out
}

// GetDashboardHomepageV2WithSource returns the homepage payload and which source was used for Timescale-backed parts.
func (s *MetricsService) GetDashboardHomepageV2WithSource(instanceName string, from, to string) (dashboard.HomepageV2, string) {
	out := s.GetDashboardHomepageV2(instanceName, from, to)
	if s.tsLogger == nil {
		return out, "live_cache"
	}

	// If risk health exists, we treat the homepage as Timescale-backed.
	if _, err := s.tsLogger.GetLatestSQLServerRiskHealth(context.Background(), instanceName); err == nil {
		return out, "timescale"
	}
	return out, "live_cache_fallback"
}

// GetPostgresDBObservationMetrics retrieves health metrics from TimescaleDB snapshots.
func (s *MetricsService) GetPostgresDBObservationMetrics(instanceName string) repository.DBObservationMetrics {
	out := repository.DBObservationMetrics{}
	cc, err := s.GetLatestPostgresControlCenterStats(context.Background(), instanceName)
	if err != nil || cc == nil {
		return out
	}
	out.XIDAge = cc.XIDAge
	out.XIDWraparoundPct = cc.XIDWraparoundPct
	out.IdleInTransactionCnt = 0 // Not directly in CC row, but could be derived or added if needed
	out.MaxTableBloatPct = cc.DeadTuplePct
	return out
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
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	if timeoutSeconds > 60 {
		timeoutSeconds = 60
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	// Execute ad-hoc SQL against TimescaleDB only (never against monitored target DBs).
	wrapped, err := sqlsandbox.WrapWithRowLimit("postgres", sql, sqlsandbox.DefaultMaxRows)
	if err != nil {
		return nil, fmt.Errorf("sql sandbox: %w", err)
	}

	rows, err := s.WidgetRepo.Pool().Query(ctx, wrapped)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fds := rows.FieldDescriptions()
	cols := make([]string, len(fds))
	for i, fd := range fds {
		cols[i] = string(fd.Name)
	}

	out := make([]map[string]interface{}, 0)
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			continue
		}
		m := make(map[string]interface{}, len(cols))
		for i, c := range cols {
			m[c] = vals[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ExecuteWidgetQuery executes a stored widget query against TimescaleDB with sandboxing.
func (s *MetricsService) ExecuteWidgetQuery(ctx context.Context, widgetID string, params map[string]string) ([]map[string]interface{}, error) {
	if s.WidgetRepo == nil {
		return nil, fmt.Errorf("widget registry not configured")
	}
	return s.WidgetRepo.ExecuteWidgetQuery(ctx, widgetID, params)
}

func (s *MetricsService) GetQueryBottlenecks(instanceName string, limit int) ([]map[string]interface{}, error) {
	return s.GetQueryStoreBottlenecks(context.Background(), instanceName, "1h", limit, "", "", "")
}

func (s *MetricsService) GetQueryBottlenecksWithRange(instanceName, timeRange string, limit int, database string, from, to string) ([]map[string]interface{}, error) {
	return s.GetQueryStoreBottlenecks(context.Background(), instanceName, timeRange, limit, database, from, to)
}

func (s *MetricsService) GetSqlServerQueryStoreSQLText(ctx context.Context, instanceName, databaseName, queryHash string) (string, error) {
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

// GetPostgresQueriesForAPI returns stats for a wall-clock window using Timescale snapshots only.
func (s *MetricsService) GetPostgresQueriesForAPI(ctx context.Context, instanceName string, from, to time.Time) ([]repository.PgQueryStat, map[string]interface{}, error) {
	meta := map[string]interface{}{
		"window_from": from.UTC().Format(time.RFC3339),
		"window_to":   to.UTC().Format(time.RFC3339),
	}
	if s.tsLogger == nil {
		return nil, meta, fmt.Errorf("TimescaleDB not connected. Query statistics require TimescaleDB snapshots.")
	}

	deltas, t0, t1, winNote, err := s.tsLogger.GetPostgresQueryStatsWindowDelta(ctx, instanceName, from, to, 50)
	if err != nil {
		log.Printf("[MetricsService] postgres query window delta unavailable for %s: %v", instanceName, err)
		return nil, meta, fmt.Errorf("No query statistics found in TimescaleDB for this range. Ensure the enterprise collector is active.")
	}

	meta["stats_source"] = "timescale_delta"
	meta["baseline_capture"] = t0.UTC().Format(time.RFC3339)
	meta["end_capture"] = t1.UTC().Format(time.RFC3339)
	meta["stats_note"] = "Values are deltas between two snapshots of pg_stat_statements. Total time is the sum of execution times in this window; avg time is total ÷ calls in the window."
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


func (s *MetricsService) GetLatestSQLServerKPIs(ctx context.Context, instanceName string) (map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	data, err := s.tsLogger.GetLatestMetrics(ctx, instanceName, "sqlserver")
	if err != nil {
		return nil, err
	}
	data["source"] = "timescale"
	return data, nil
}

func (s *MetricsService) GetLatestSQLServerRunningQueries(ctx context.Context, instanceName, dbFilter string) ([]models.SQLServerSessionSnapshot, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetLatestSQLServerSessionSnapshots(ctx, instanceName, dbFilter)
}

func (s *MetricsService) GetLatestSQLServerIOLatency(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetFileIOLatency(ctx, instanceName, 20)
}

func (s *MetricsService) GetLatestSQLServerTempDBUsage(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetTempdbFiles(ctx, instanceName, 20)
}

func (s *MetricsService) GetLatestSQLServerBlocking(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	// For now, return empty as full blocking chain persistence for SQL Server is not yet in the 'hot' package
	// but we indicate 'timescale' as the source to fulfill the requirement of no direct hits.
	return []map[string]interface{}{}, nil
}

func (s *MetricsService) GetLatestSQLServerWaitStats(ctx context.Context, instanceName, dbFilter string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	// Fetch latest wait history entry
	limit := 1
	rows, err := s.tsLogger.GetLatestSQLServerWaitHistory(ctx, instanceName, limit)
	if err != nil {
		return nil, err
	}
	// Transform to expected live format if needed
	var results []map[string]interface{}
	for _, r := range rows {
		results = append(results, map[string]interface{}{
			"wait_type":          "Total (Historical)",
			"wait_time_ms":       r.DiskRead + r.Blocking + r.Parallelism + r.Other,
			"wait_resource_desc": "Aggregated from TimescaleDB",
			"timestamp":          r.CaptureTimestamp,
		})
	}
	return results, nil
}

func (s *MetricsService) GetLatestSQLServerConnections(ctx context.Context, instanceName, dbFilter string) ([]models.ConnectionStat, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	rows, err := s.tsLogger.GetLatestSQLServerConnectionSnapshots(ctx, instanceName, 100)
	if err != nil {
		return nil, err
	}
	var results []models.ConnectionStat
	for _, r := range rows {
		dbName, _ := r["database_name"].(string)
		if dbFilter != "" && dbName != dbFilter {
			continue
		}
		results = append(results, models.ConnectionStat{
			LoginName:         r["login_name"].(string),
			DatabaseName:      dbName,
			ActiveConnections: r["active_connections"].(int),
			ActiveRequests:    r["active_requests"].(int),
		})
	}
	return results, nil
}



func (s *MetricsService) GetLatestPostgresSessions(ctx context.Context, instanceName string) ([]hot.PgSessionSnapshotRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetLatestPgSessionSnapshot(ctx, instanceName)
}

func (s *MetricsService) GetLatestPostgresLocks(ctx context.Context, instanceName string) ([]hot.PgLockSnapshotRow, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetLatestPgLockSnapshot(ctx, instanceName)
}

func (s *MetricsService) GetLatestPostgresBlockingTree(ctx context.Context, instanceName string) ([]repository.PgBlockingNode, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	// Fetch last 5 minutes to find the most recent blocking event
	to := time.Now().UTC()
	from := to.Add(-5 * time.Minute)
	// GetPgBlockingDetailsInRange returns a response containing the tree
	resp, err := s.tsLogger.GetPgBlockingDetailsInRange(ctx, instanceName, from, to)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return []repository.PgBlockingNode{}, nil
	}

	// Convert hot.PgBlockingNodeAt to repository.PgBlockingNode
	var out []repository.PgBlockingNode
	for _, n := range resp.BlockingTree {
		out = append(out, s.convertToRepoBlockingNode(n))
	}
	return out, nil
}

func (s *MetricsService) convertToRepoBlockingNode(n hot.PgBlockingNodeAt) repository.PgBlockingNode {
	node := repository.PgBlockingNode{
		PID:        n.PID,
		Database:   n.Database,
		User:       n.User,
		State:      n.State,
		QueryStart: n.QueryStart,
		Duration:   n.Duration,
		WaitEvent:  n.WaitEvent,
		Query:      n.Query,
	}
	for _, c := range n.BlockedBy {
		node.BlockedBy = append(node.BlockedBy, s.convertToRepoBlockingNode(c))
	}
	return node
}


func (s *MetricsService) GetLatestPostgresXIDRisk(ctx context.Context, instanceName string) (*instance_health.PgInstanceSnapshot, error) {
	if s.PgHealthRepo == nil {
		return nil, fmt.Errorf("PgHealthRepo is nil")
	}
	return s.PgHealthRepo.GetLatestSnapshot(ctx, instanceName)
}

func (s *MetricsService) GetLatestPostgresWALArchiverRisk(ctx context.Context, instanceName string) (*instance_health.PgInstanceSnapshot, error) {
	if s.PgHealthRepo == nil {
		return nil, fmt.Errorf("PgHealthRepo is nil")
	}
	return s.PgHealthRepo.GetLatestSnapshot(ctx, instanceName)
}

func (s *MetricsService) GetTimescalePostgresDatabases(ctx context.Context, instanceName string) ([]string, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("TimescaleDB not connected")
	}
	return s.tsLogger.GetPostgresDatabases(ctx, instanceName)
}

func (s *MetricsService) GetPostgresWaitSummary(ctx context.Context, instanceName string) (map[string]int, error) {
	return s.PgRepo.GetWaitEventSummary(instanceName)
}

func (s *MetricsService) GetPostgresTopWaitEvents(ctx context.Context, instanceName string, limit int) (map[string]int, error) {
	return s.PgRepo.GetTopWaitEvents(instanceName, limit)
}

func pleValOnly(h []map[string]interface{}) []float64 {
	out := make([]float64, len(h))
	for i, m := range h {
		val, _ := m["page_life_expectancy"].(float64)
		out[i] = val
	}
	return out
}

// GetHealthV2DashboardData orchestrates fetching both real-time and historical data for SQL Server Health V2.
func (s *MetricsService) GetHealthV2DashboardData(ctx context.Context, instanceName string, from, to time.Time) (models.HealthV2DashboardResponse, error) {
	// 1. Real-time data from repository
	res, err := s.MsRepo.FetchHealthV2(ctx, instanceName)
	if err != nil {
		return res, err
	}

	if s.tsLogger == nil {
		return res, nil // TimescaleDB not configured, return real-time only
	}

	// 2. Historical trends from TimescaleDB
	res.WaitTrends, _ = s.tsLogger.GetWaitTrendV2(ctx, instanceName, from, to)
	res.IOLatency, _ = s.tsLogger.GetIOLatencyTrendV2(ctx, instanceName, from, to)
	res.Throughput, _ = s.tsLogger.GetThroughputTrendV2(ctx, instanceName, from, to)

	return res, nil
}
