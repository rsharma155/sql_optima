// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Metrics service orchestration including TimescaleDB persistence and cache management.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"log/slog"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/collectors"
	"github.com/rsharma155/sql_optima/internal/collectors/infrastructure/sqlserver"
	"github.com/rsharma155/sql_optima/internal/collectors/postgres"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/domain/entities"
	pg_backup_repo "github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/domain/repositories"
	pg_obs_repo "github.com/rsharma155/sql_optima/internal/domain/postgres_observability/domain/repositories"
	pg_obs_coll "github.com/rsharma155/sql_optima/internal/domain/postgres_observability/collectors"
	pg_security_coll "github.com/rsharma155/sql_optima/internal/domain/postgres_security/collectors"
	pg_security_repo "github.com/rsharma155/sql_optima/internal/domain/postgres_security/domain/repositories"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/domain"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_monitoring/instance_health"
	"github.com/rsharma155/sql_optima/internal/domain/servers"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/security"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type InstanceControlPlane struct {
	tasks chan func()
}

type MetricsService struct {
	PgRepo              *repository.PgRepository
	MsRepo              *repository.SqlServerRepository
	WidgetRepo          *repository.WidgetRepository
	UserRepo            *repository.UserRepository
	ServerRepo          servers.ServerStore
	AuditRepo           *repository.AuditLogRepository
	ConfigRepo          *repository.CollectorConfigRepository
	ServerKMS           servers.KeyManagementService
	ServerSecretBox     servers.SecretBox
	Config              *config.Config
	RegistryReload      func()
	cacheMutex          sync.RWMutex
	dashboardCache      map[uuid.UUID]models.DashboardMetrics
	pgDashboardCache    map[uuid.UUID]models.PgCoreDashboardCache
	tsLogger            *hot.TimescaleLogger
	tsHotStorage        *hot.HotStorage
	FetchInterval       time.Duration
	PgSnapshotCollector      *collectors.PgSnapshotCollector
	PgComprehensiveCollector *collectors.PgComprehensiveCollector
	waitDMVCollector         *sqlserver.WaitStatsDMVCollector

	// PostgreSQL Monitoring Domain Services
	PgHealthService *instance_health.InstanceHealthService
	PgHealthRepo    *instance_health.InstanceHealthRepository
	PgQueryRouter   *postgres.QueryMetricsRouter
	PgTimescaleColl *postgres.PgTimescaleCollector
	PgObsCollector      *pg_obs_coll.PostgresObservabilityCollector
	PgSecurityCollector *pg_security_coll.PostgresSecurityCollector
	PgIndexDefCollector *collectors.PgIndexDefinitionCollector

	// Postgres Domain Repositories
	PgObservabilityRepo *pg_obs_repo.PostgresObservabilityRepository
	PgBackupRepo        *pg_backup_repo.PostgresBackupRepository
	PgDRPolicyRepo      *pg_backup_repo.DRPolicyRepository
	PgSecurityRepo      *pg_security_repo.PostgresSecurityRepository

	tsMu             sync.Mutex
	sihMu            sync.Mutex
	sihLastIndex15m  map[uuid.UUID]time.Time
	sihLastTable15m  map[uuid.UUID]time.Time
	sihLastGrowth6h  map[uuid.UUID]time.Time
	sihLastDefsDaily map[uuid.UUID]time.Time

	controlPlanes map[uuid.UUID]*InstanceControlPlane
	cpMu          sync.Mutex

	hwSlowMu          sync.Mutex
	hwLastSlowCollect  map[uuid.UUID]time.Time
	hwPropsLogged      map[uuid.UUID]bool

	PulseSvc      *PulseService

	platformSettings *repository.PlatformSettingsRepository
	osIngestMu       sync.RWMutex
	osIngestEnabled  bool
	osIngestLoaded   bool
}

func NewMetricsService(pg *repository.PgRepository, ms *repository.SqlServerRepository, cfg *config.Config, tsStorage *hot.HotStorage) *MetricsService {
	var tsLogger *hot.TimescaleLogger
	if tsStorage != nil {
		tsLogger = hot.NewTimescaleLogger(tsStorage.Pool())
	}

	svc := &MetricsService{
		PgRepo:           pg,
		MsRepo:           ms,
		Config:           cfg,
		tsLogger:         tsLogger,
		tsHotStorage:     tsStorage,
		FetchInterval:    60 * time.Second,
		dashboardCache:   make(map[uuid.UUID]models.DashboardMetrics),
		pgDashboardCache: make(map[uuid.UUID]models.PgCoreDashboardCache),
		sihLastIndex15m:  make(map[uuid.UUID]time.Time),
		sihLastTable15m:  make(map[uuid.UUID]time.Time),
		sihLastGrowth6h:  make(map[uuid.UUID]time.Time),
		sihLastDefsDaily:  make(map[uuid.UUID]time.Time),
		hwLastSlowCollect: make(map[uuid.UUID]time.Time),
		hwPropsLogged:     make(map[uuid.UUID]bool),
		controlPlanes:     make(map[uuid.UUID]*InstanceControlPlane),
		waitDMVCollector: &sqlserver.WaitStatsDMVCollector{},
	}

	if tsLogger != nil {
		svc.PulseSvc = NewPulseService(cfg, ms, pg, tsLogger, svc)
		svc.PgHealthRepo = instance_health.NewInstanceHealthRepository(tsStorage.Pool())
		svc.PgHealthService = instance_health.NewInstanceHealthService(svc.PgHealthRepo)
		svc.PgSnapshotCollector = collectors.NewPgSnapshotCollector(pg, svc.PgHealthService, svc.PgHealthRepo, tsLogger)
		svc.PgComprehensiveCollector = collectors.NewPgComprehensiveCollector(pg, tsLogger)
		svc.PgTimescaleColl = postgres.NewPgTimescaleCollector(pg, tsLogger)
		svc.PgIndexDefCollector = collectors.NewPgIndexDefinitionCollector(pg, tsLogger)
		svc.PgObservabilityRepo = pg_obs_repo.NewPostgresObservabilityRepository(tsStorage.Pool())
		svc.PgObsCollector = pg_obs_coll.NewPostgresObservabilityCollector(svc.PgObservabilityRepo)
		svc.PgSecurityRepo = pg_security_repo.NewPostgresSecurityRepository(tsStorage.Pool())
		svc.PgSecurityCollector = pg_security_coll.NewPostgresSecurityCollector(svc.PgSecurityRepo)
		svc.PgBackupRepo = pg_backup_repo.NewPostgresBackupRepository(tsStorage.Pool())
		svc.PgDRPolicyRepo = pg_backup_repo.NewDRPolicyRepository(tsStorage.Pool())
		svc.platformSettings = repository.NewPlatformSettingsRepository(tsStorage.Pool())
		_ = svc.ReloadOSMetricsIngest(context.Background())
	}

	return svc
}

// GetDRPolicy returns per-server DR targets or defaults when unset.
func (s *MetricsService) GetDRPolicy(ctx context.Context, serverID uuid.UUID) entities.DRPolicy {
	if s.PgDRPolicyRepo == nil {
		return entities.DefaultDRPolicy(serverID)
	}
	p, err := s.PgDRPolicyRepo.Get(ctx, serverID)
	if err != nil {
		return entities.DefaultDRPolicy(serverID)
	}
	return p
}

// GetAllInstanceStatuses merges connection statuses from both Postgres and SQL Server repositories.
func (s *MetricsService) GetAllInstanceStatuses() map[string]string {
	statuses := make(map[string]string)
	if s.PgRepo != nil {
		for k, v := range s.PgRepo.GetAllInstanceStatuses() {
			statuses[k] = v
		}
	}
	if s.MsRepo != nil {
		for k, v := range s.MsRepo.GetAllInstanceStatuses() {
			statuses[k] = v
		}
	}
	return statuses
}

func (s *MetricsService) GetTimescaleDBPool() *pgxpool.Pool {
	if s.tsHotStorage == nil {
		return nil
	}
	return s.tsHotStorage.Pool()
}

func (s *MetricsService) IsTimescaleConnected() bool {
	return s.tsHotStorage != nil
}

func (s *MetricsService) GetTimescaleDBLogger() *hot.TimescaleLogger {
	return s.tsLogger
}

func (s *MetricsService) TimescalePing(ctx context.Context) error {
	if s.tsHotStorage == nil {
		return fmt.Errorf("timescale storage not configured")
	}
	return s.tsHotStorage.Pool().Ping(ctx)
}

func (s *MetricsService) EnqueueCollection(serverID uuid.UUID, task func()) {
	s.cpMu.Lock()
	cp, ok := s.controlPlanes[serverID]
	if !ok {
		cp = &InstanceControlPlane{
			tasks: make(chan func(), 100),
		}
		s.controlPlanes[serverID] = cp
		go func(id uuid.UUID, plane *InstanceControlPlane) {
			slog.Info("[ControlPlane] Starting sequential worker for", "val", id)
			for t := range plane.tasks {
				t()
			}
		}(serverID, cp)
	}
	s.cpMu.Unlock()

	select {
	case cp.tasks <- task:
	default:
		slog.Warn("[ControlPlane] WARNING: Task queue full for", "val", serverID)
	}
}

func (s *MetricsService) GetPgDashboardV2(ctx context.Context, serverID uuid.UUID) (map[string]interface{}, error) {
	if s.PgHealthRepo == nil {
		return nil, fmt.Errorf("postgres health repository not initialized")
	}

	snapshot, err := s.PgHealthRepo.GetLatestSnapshot(ctx, serverID)
	if err != nil && !strings.Contains(err.Error(), "no rows") {
		return nil, err
	}

	var blocking int
	if s.tsHotStorage != nil {
		_ = s.tsHotStorage.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM dashboard.postgres_active_incidents WHERE server_id = $1 AND blocking_pids <> '{}'", serverID).Scan(&blocking)
	}

	var slowQueries []models.PgSession
	res := map[string]interface{}{
		"snapshot": snapshot,
		"incidents": map[string]interface{}{
			"blocking_sessions": blocking,
			"slow_queries":      slowQueries,
		},
		"timestamp": time.Now().UTC(),
	}

	if s.PgObservabilityRepo != nil {
		from := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
		to := time.Now().Format(time.RFC3339)
		res["load_trend"], _ = s.PgObservabilityRepo.GetLoadTrend(ctx, serverID, from, to)
		res["wait_trend"], _ = s.PgObservabilityRepo.GetWaitCategoryTrend(ctx, serverID, from, to)
	}

	return res, nil
}

func (s *MetricsService) GetTimescalePostgresDatabases(ctx context.Context, serverID uuid.UUID) ([]string, error) {
	return s.PgRepo.FetchDatabases(ctx, serverID.String())
}

func (s *MetricsService) GetPostgresOverview(serverID uuid.UUID) models.InstanceOverview {
	if s.PgHealthRepo == nil {
		return models.InstanceOverview{Engine: "postgres"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	snapshot, err := s.PgHealthRepo.GetLatestSnapshot(ctx, serverID)
	if err != nil {
		return models.InstanceOverview{Engine: "postgres"}
	}

	instanceName := ""
	for _, inst := range s.Config.Instances {
		if inst.ServerID == serverID {
			instanceName = inst.Name
			break
		}
	}

	return models.InstanceOverview{
		InstanceName:    instanceName,
		Engine:          "postgres",
		Timestamp:       snapshot.CollectedAt.Format(time.RFC3339),
		DatabaseCount:   0, // Placeholder or fetch if needed
		LastTps:         snapshot.TPS,
		LastCacheHitPct: snapshot.CacheHitRatio * 100,
		ActiveConns:     snapshot.ActiveSessions,
		IdleConns:       snapshot.IdleSessions,
		TotalConns:      snapshot.ActiveSessions + snapshot.IdleSessions + snapshot.IdleInTxSessions,
		ReplicationLag:  snapshot.ReplicaLagSec,
	}
}

func (s *MetricsService) GetPostgresStorageMetrics(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"tables": []hot.PostgresTableMaintResponse{}}, nil
	}
	tables, err := s.tsLogger.GetLatestPostgresTableMaint(ctx, serverID, 200)
	if err != nil {
		return nil, err
	}
	if tables == nil {
		tables = []hot.PostgresTableMaintResponse{}
	}
	return map[string]interface{}{
		"tables": tables,
	}, nil
}

func (s *MetricsService) GetPostgresVacuumProgress(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"progress": []hot.PostgresVacuumProgressRow{}}, nil
	}
	prog, err := s.tsLogger.GetPostgresVacuumProgress(ctx, serverID, 50)
	if err != nil {
		return nil, err
	}
	if prog == nil {
		prog = []hot.PostgresVacuumProgressRow{}
	}
	return map[string]interface{}{
		"progress": prog,
	}, nil
}

func (s *MetricsService) GetPostgresBloatEstimates(ctx context.Context, serverID uuid.UUID, limit int) (interface{}, error) {
	if s.PgRepo == nil {
		return map[string]interface{}{"tables": []interface{}{}}, nil
	}
	name := s.instanceNameByServerID(serverID)
	if name == "" {
		return map[string]interface{}{"tables": []interface{}{}}, nil
	}
	tables, err := s.PgRepo.GetBloatEstimates(ctx, name, limit)
	if err != nil || tables == nil {
		return map[string]interface{}{"tables": []interface{}{}}, nil
	}
	return map[string]interface{}{"tables": tables}, nil
}

func (s *MetricsService) GetPostgresXIDWraparoundRisk(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	if s.PgRepo == nil {
		return map[string]interface{}{"databases": []interface{}{}}, nil
	}
	name := s.instanceNameByServerID(serverID)
	if name == "" {
		return map[string]interface{}{"databases": []interface{}{}}, nil
	}
	dbs, err := s.PgRepo.GetXIDWraparoundRisk(ctx, name)
	if err != nil || dbs == nil {
		return map[string]interface{}{"databases": []interface{}{}}, nil
	}
	return map[string]interface{}{"databases": dbs}, nil
}

func (s *MetricsService) GetPostgresLongRunningTransactions(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	if s.PgRepo == nil {
		return map[string]interface{}{"transactions": []interface{}{}}, nil
	}
	name := s.instanceNameByServerID(serverID)
	if name == "" {
		return map[string]interface{}{"transactions": []interface{}{}}, nil
	}
	txns, err := s.PgRepo.GetLongRunningTransactions(ctx, name)
	if err != nil || txns == nil {
		return map[string]interface{}{"transactions": []interface{}{}}, nil
	}
	return map[string]interface{}{"transactions": txns}, nil
}

func (s *MetricsService) GetPostgresIndexBloat(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	if s.PgRepo == nil {
		return map[string]interface{}{"indexes": []interface{}{}}, nil
	}
	name := s.instanceNameByServerID(serverID)
	if name == "" {
		return map[string]interface{}{"indexes": []interface{}{}}, nil
	}
	indexes, err := s.PgRepo.GetIndexBloat(ctx, name, 200)
	if err != nil || indexes == nil {
		return map[string]interface{}{"indexes": []interface{}{}}, nil
	}
	return map[string]interface{}{"indexes": indexes}, nil
}

func (s *MetricsService) GetPostgresIdleInTransactionSessions(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	if s.PgRepo == nil {
		return map[string]interface{}{"sessions": []interface{}{}}, nil
	}
	name := s.instanceNameByServerID(serverID)
	if name == "" {
		return map[string]interface{}{"sessions": []interface{}{}}, nil
	}
	sessions, err := s.PgRepo.GetIdleInTransactionSessions(ctx, name)
	if err != nil || sessions == nil {
		return map[string]interface{}{"sessions": []interface{}{}}, nil
	}
	return map[string]interface{}{"sessions": sessions}, nil
}

func (s *MetricsService) GetPostgresCPUHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetPostgresCPUHistory(ctx, serverID, from, to, limit)
}

func (s *MetricsService) GetPostgresMemoryHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time) (interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"history": []interface{}{}, "current": nil}, nil
	}
	return s.tsLogger.GetPgMemoryTimeSeries(ctx, serverID, from, to)
}

func (s *MetricsService) GetPostgresWaitEventsHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]hot.PostgresWaitEventRow, error) {
	if s.tsLogger == nil {
		return []hot.PostgresWaitEventRow{}, nil
	}
	return s.tsLogger.GetPostgresWaitEventsHistory(ctx, serverID, from, to, limit)
}

func (s *MetricsService) GetPostgresDbIOHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]hot.PostgresDbIORow, error) {
	if s.tsLogger == nil {
		return []hot.PostgresDbIORow{}, nil
	}
	return s.tsLogger.GetPostgresDbIOHistory(ctx, serverID, from, to, limit)
}

func (s *MetricsService) GetPostgresSettingsDrift(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"changes": []interface{}{}}, nil
	}
	latestTs, prevTs, latest, prev, err := s.tsLogger.GetPostgresSettingsSnapshotLatestTwo(ctx, serverID)
	if err != nil {
		return map[string]interface{}{"changes": []interface{}{}}, nil
	}
	return map[string]interface{}{
		"latest_timestamp": latestTs,
		"prev_timestamp":   prevTs,
		"latest":           latest,
		"prev":             prev,
	}, nil
}



func (s *MetricsService) GetTimescalePostgresReplicationSlots(serverID uuid.UUID, limit int) ([]hot.PostgresReplicationSlotRow, error) {
	if s.tsLogger == nil {
		return []hot.PostgresReplicationSlotRow{}, nil
	}
	return s.tsLogger.GetPostgresReplicationSlotHistory(context.Background(), serverID, limit)
}

func (s *MetricsService) GetTimescalePostgresDiskStats(serverID uuid.UUID, limit int) ([]hot.PostgresDiskStatRow, error) {
	if s.tsLogger == nil {
		return []hot.PostgresDiskStatRow{}, nil
	}
	diskHistory, err := s.tsLogger.GetPostgresDiskHistory(context.Background(), serverID, limit)
	if err != nil { return nil, err }
	var out []hot.PostgresDiskStatRow
	for _, r := range diskHistory {
		out = append(out, hot.PostgresDiskStatRow(r))
	}
	return out, nil
}

func (s *MetricsService) GetPostgresSessionStateCountsHistory(ctx context.Context, serverID uuid.UUID, limit int) ([]hot.PostgresSessionStateCountRow, error) {
	if s.tsLogger == nil {
		return []hot.PostgresSessionStateCountRow{}, nil
	}
	res, err := s.tsLogger.GetPostgresSessionStateHistory(ctx, serverID, limit)
	if err != nil { return nil, err }
	var out []hot.PostgresSessionStateCountRow
	for _, r := range res {
		out = append(out, hot.PostgresSessionStateCountRow(r))
	}
	return out, nil
}

func (s *MetricsService) GetLatestPostgresSessions(ctx context.Context, serverID uuid.UUID) ([]hot.PgSessionSnapshotRow, error) {
	if s.tsLogger == nil {
		return []hot.PgSessionSnapshotRow{}, nil
	}
	return s.tsLogger.GetLatestPgSessionSnapshot(ctx, serverID)
}

func (s *MetricsService) GetLatestPostgresLocks(ctx context.Context, serverID uuid.UUID) ([]hot.PgLockSnapshotRow, error) {
	if s.tsLogger == nil {
		return []hot.PgLockSnapshotRow{}, nil
	}
	return s.tsLogger.GetLatestPgLockSnapshot(ctx, serverID)
}

func (s *MetricsService) GetPostgresQueriesForAPI(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]repository.PgQueryStat, map[string]interface{}, error) {
	stats, err := s.PgHealthRepo.GetQueryStatsRange(ctx, serverID, from, to)
	if err != nil { return nil, nil, err }
	var out []repository.PgQueryStat
	for _, d := range stats {
		out = append(out, pgQueryDeltaToStat(d))
	}
	return out, nil, nil
}

func pgQueryDeltaToStat(d hot.PostgresQueryStatsDelta) repository.PgQueryStat {
	return repository.PgQueryStat{
		QueryID: d.QueryID,
		Calls: d.CallsDelta,
		TotalTime: d.TotalTimeDeltaMs,
	}
}

func (s *MetricsService) ReplaceInstanceRepositories(pg *repository.PgRepository, ms *repository.SqlServerRepository) {
	if s == nil {
		return
	}
	s.PgRepo = pg
	s.MsRepo = ms
}

func (s *MetricsService) GetLatestSQLServerKPIs(ctx context.Context, serverID uuid.UUID) (map[string]interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSQLServerBlockingKPIs(ctx, serverID)
}

func (s *MetricsService) GetDashboardWidgets(ctx context.Context, serverID uuid.UUID) ([]models.DashboardWidget, error) {
	return []models.DashboardWidget{}, nil
}

func (s *MetricsService) ExecuteWidgetQuery(ctx context.Context, serverID uuid.UUID, widgetID string) (interface{}, error) {
	return nil, nil
}

func (s *MetricsService) GetPgLocksBlockingKpis(ctx context.Context, serverID uuid.UUID) (*hot.PgBlockingKpis, error) {
	if s.tsLogger == nil {
		return &hot.PgBlockingKpis{}, nil
	}
	return s.tsLogger.GetPgBlockingKpis(ctx, serverID)
}

func (s *MetricsService) LogPostgresBackupRun(ctx context.Context, serverID uuid.UUID, stats interface{}) error {
	return nil
}

func (s *MetricsService) GetLatestPostgresBackupRun(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"status": "no_data"}, nil
	}
	row, err := s.tsLogger.GetLatestPostgresBackupRun(ctx, serverID)
	if err != nil {
		return map[string]interface{}{"status": "no_data"}, nil
	}
	return row, nil
}


func (s *MetricsService) FetchPgBestPracticesWithTimescale(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	if s.tsHotStorage == nil {
		return map[string]interface{}{"checks": []interface{}{}}, nil
	}

	// Align with ruleengine schema (rule_name, current_value, recommended) used by /api/rules/best-practices.
	query := `
		SELECT
			r.rule_id,
			r.rule_name,
			r.category,
			r.severity,
			UPPER(COALESCE(e.status, 'OK')) AS status,
			COALESCE(
				NULLIF(BTRIM(e.current_value), ''),
				NULLIF(BTRIM(raw.current_value), ''),
				''
			) AS current_value,
			COALESCE(NULLIF(BTRIM(e.recommended), ''), r.recommended_value, '') AS recommended_value,
			COALESCE(r.description, '') AS description
		FROM ruleengine.rules r
		LEFT JOIN LATERAL (
			SELECT status, current_value, recommended
			FROM ruleengine.rule_results_evaluated
			WHERE server_id = $1 AND rule_id = r.rule_id AND target_db_type = 'postgres'
			ORDER BY capture_timestamp DESC
			LIMIT 1
		) e ON TRUE
		LEFT JOIN LATERAL (
			SELECT NULLIF(BTRIM(raw_payload->>'CurrentValue'), '') AS current_value
			FROM ruleengine.rule_results_raw
			WHERE server_id = $1 AND rule_id = r.rule_id
			ORDER BY capture_timestamp DESC
			LIMIT 1
		) raw ON TRUE
		WHERE r.is_enabled = true AND r.target_db_type = 'postgres'
		ORDER BY
			CASE UPPER(COALESCE(e.status, 'OK'))
				WHEN 'CRITICAL' THEN 1
				WHEN 'WARNING' THEN 2
				ELSE 3
			END,
			r.category,
			r.rule_name`

	rows, err := s.tsHotStorage.Pool().Query(ctx, query, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []map[string]interface{}
	for rows.Next() {
		var id, name, cat, sev, status, current, recommended, desc string
		if err := rows.Scan(&id, &name, &cat, &sev, &status, &current, &recommended, &desc); err != nil {
			continue
		}
		checks = append(checks, map[string]interface{}{
			"id":                id,
			"rule_id":           id,
			"name":              name,
			"rule_name":         name,
			"category":          cat,
			"severity":          sev,
			"status":            status,
			"current_value":     current,
			"recommended_value": recommended,
			"recommendation":    recommended,
			"description":       desc,
		})
	}

	return map[string]interface{}{
		"checks":     checks,
		"updated_at": time.Now().UTC(),
	}, nil
}

func (s *MetricsService) GetPostgresControlCenterHistory(ctx context.Context, serverID uuid.UUID, from, to string, limit int) (*hot.PostgresControlCenterHistory, error) {
	if s.tsLogger == nil {
		return &hot.PostgresControlCenterHistory{
			Labels:              []string{},
			TPS:                 []float64{},
			WALMBPerMin:         []float64{},
			ReplLagSec:          []float64{},
			CheckpointReqRatio:  []float64{},
			Autovacuum:          []int{},
			DeadTuplePct:        []float64{},
			BlockingSessions:    []int{},
			HealthScore:         []int{},
			CacheHitRatioPct:    []float64{},
			ConnectionsUsagePct: []float64{},
		}, nil
	}
	return s.tsLogger.GetPostgresControlCenterHistory(ctx, serverID, from, to, limit)
}

func (s *MetricsService) GetLatestPostgresControlCenterStats(ctx context.Context, serverID uuid.UUID) (*hot.PostgresControlCenterRow, error) {
	if s.tsLogger == nil {
		return &hot.PostgresControlCenterRow{}, nil
	}
	return s.tsLogger.GetLatestPostgresControlCenterStats(ctx, serverID)
}

func (s *MetricsService) GetPgIncidentFeed(ctx context.Context, serverID uuid.UUID, severity int, limit int) ([]hot.PgIncidentFeedRow, error) {
	if s.tsLogger == nil {
		return []hot.PgIncidentFeedRow{}, nil
	}
	return s.tsLogger.GetPgIncidentFeed(ctx, serverID, severity, limit)
}

func (s *MetricsService) LogPostgresLogEvents(ctx context.Context, serverID uuid.UUID, events interface{}) error {
	return nil
}

func (s *MetricsService) GetLatestPostgresTableMaint(ctx context.Context, serverID uuid.UUID, limit int) ([]hot.PostgresTableMaintResponse, error) {
	if s.tsLogger == nil {
		return []hot.PostgresTableMaintResponse{}, nil
	}
	res, err := s.tsLogger.GetLatestPostgresTableMaint(ctx, serverID, limit)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return []hot.PostgresTableMaintResponse{}, nil
	}
	return res, nil
}

func (s *MetricsService) GetLatestPostgresXIDRisk(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	return map[string]interface{}{"max_age": 0, "databases": []interface{}{}}, nil
}

func (s *MetricsService) GetLatestPostgresWALArchiverRisk(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	instanceName := s.instanceNameByServerID(serverID)
	if instanceName == "" || s.PgRepo == nil {
		return map[string]interface{}{"status": "no_data"}, nil
	}
	return s.PgRepo.GetWALArchiverRisk(ctx, instanceName)
}

func (s *MetricsService) GetPostgresReplicationLagDetail(ctx context.Context, serverID uuid.UUID, from, to string, limit int) (map[string]hot.PostgresReplicationLagSeries, error) {
	if s.tsLogger == nil {
		return map[string]hot.PostgresReplicationLagSeries{}, nil
	}
	return s.tsLogger.GetPostgresReplicationLagDetail(ctx, serverID, from, to, limit)
}

func (s *MetricsService) GetLatestPostgresPoolerStats(ctx context.Context, serverID uuid.UUID) (*hot.PostgresPoolerStatRow, error) {
	if s.tsLogger == nil {
		return &hot.PostgresPoolerStatRow{}, nil
	}
	return s.tsLogger.GetLatestPostgresPoolerStats(ctx, serverID)
}

func (s *MetricsService) GetPostgresPoolerStatsHistory(ctx context.Context, serverID uuid.UUID, limit int) ([]hot.PostgresPoolerStatRow, error) {
	if s.tsLogger == nil {
		return []hot.PostgresPoolerStatRow{}, nil
	}
	return s.tsLogger.GetPostgresPoolerHistory(ctx, serverID, limit)
}

func (s *MetricsService) GetPostgresDBObservationMetrics(ctx context.Context, serverID uuid.UUID, dbName string) (interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"observations": []interface{}{}}, nil
	}
	return map[string]interface{}{"observations": []interface{}{}}, nil
}

func (s *MetricsService) GetCachedPgThroughputDashboard(serverID uuid.UUID) (interface{}, error) {
	return nil, nil
}

func (s *MetricsService) getEngine(serverID uuid.UUID) string {
	for _, inst := range s.Config.Instances {
		if inst.ServerID == serverID {
			return strings.ToLower(inst.Type)
		}
	}
	return "sqlserver"
}

func (s *MetricsService) TimescaleStorageIndexHealthIndexUsage(ctx context.Context, serverID uuid.UUID, from, to, db, schema, table string) (interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"points": []models.IndexUsageStat{}}, nil
	}
	engine := s.getEngine(serverID)
	stats, err := s.tsLogger.QueryStorageIndexHealthIndexUsage(ctx, engine, serverID.String(), from, to, db, schema, table, 100)
	if stats == nil {
		stats = []models.IndexUsageStat{}
	}
	return map[string]interface{}{"points": stats}, err
}

func (s *MetricsService) TimescaleStorageIndexHealthTableUsage(ctx context.Context, serverID uuid.UUID, from, to string) (interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"points": []interface{}{}, "tables": []interface{}{}}, nil
	}
	engine := s.getEngine(serverID)
	return s.tsLogger.QueryStorageIndexHealthTableUsage(ctx, engine, serverID.String(), from, to, 100)
}

func (s *MetricsService) TimescaleStorageIndexHealthGrowth(ctx context.Context, serverID uuid.UUID, from, to string) (interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"growth": []interface{}{}}, nil
	}
	engine := s.getEngine(serverID)
	return s.tsLogger.QueryStorageIndexHealthTableGrowth(ctx, engine, serverID.String(), from, to, 100)
}

func (s *MetricsService) TimescaleStorageIndexHealthDashboard(ctx context.Context, serverID uuid.UUID, from, to string, f hot.SIHFilters) (interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"kpis": map[string]interface{}{}, "alerts": []interface{}{}}, nil
	}
	engine := s.getEngine(serverID)
	if from == "" {
		from = time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	}
	if to == "" {
		to = time.Now().Format(time.RFC3339)
	}
	return s.tsLogger.QueryStorageIndexHealthDashboard(ctx, engine, serverID.String(), from, to, f)
}

func (s *MetricsService) TimescaleStorageIndexHealthFilterOptions(ctx context.Context, serverID uuid.UUID, from, to string, db, schema string) (interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"databases": []string{}, "schemas": []string{}}, nil
	}
	engine := s.getEngine(serverID)
	return s.tsLogger.QueryStorageIndexHealthFilterOptions(ctx, engine, serverID.String(), from, to, db, schema)
}

func (s *MetricsService) TimescaleStorageIndexDefinition(ctx context.Context, serverID uuid.UUID, from, to string) (interface{}, error) {
	return map[string]interface{}{"definition": ""}, nil
}

func (s *MetricsService) GetPgssStatus(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	if s.Config == nil || s.PgRepo == nil {
		return map[string]interface{}{"enabled": false, "reason": "service not configured"}, nil
	}
	instanceName := ""
	for _, inst := range s.Config.Instances {
		if inst.ServerID == serverID {
			instanceName = inst.Name
			break
		}
	}
	if instanceName == "" {
		return map[string]interface{}{"enabled": false, "reason": "instance not found"}, nil
	}

	enabled := s.PgRepo.GetPgssSupported(instanceName)
	if !enabled {
		// Cache may be stale (e.g. set before the extension was installed, or due to a
		// startup race). Do a live probe so the status page always reflects reality.
		s.PgRepo.RefreshPgssStatus(ctx, instanceName)
		enabled = s.PgRepo.GetPgssSupported(instanceName)
	}
	preloadActive := s.PgRepo.GetPgssSharedPreload(instanceName)
	extInstalled := s.PgRepo.GetPgssExtensionInstalled(instanceName)

	sid := uuid.Nil
	for _, inst := range s.Config.Instances {
		if inst.Name == instanceName {
			sid = inst.ServerID
			break
		}
	}
	hasData := enabled && s.tsLogger != nil && s.tsLogger.GetPgssHasData(ctx, sid)
	rawSnapshots := 0
	if enabled && s.tsLogger != nil {
		rawSnapshots = s.tsLogger.GetPgssRawSnapshotCount(ctx, sid)
	}

	msg := ""
	if !enabled {
		if preloadActive && !extInstalled {
			msg = "pg_stat_statements is loaded in shared_preload_libraries, but the extension is not created in this database. Run 'CREATE EXTENSION pg_stat_statements;' to enable collection."
		} else {
			msg = "pg_stat_statements is not enabled — add it to shared_preload_libraries and restart PostgreSQL."
		}
	} else if !hasData {
		if rawSnapshots == 0 {
			msg = "pg_stat_statements is enabled. Query data collection is in progress — charts will appear within 2–3 minutes."
		} else {
			msg = fmt.Sprintf("pg_stat_statements is enabled and %d raw snapshots collected. Delta computation is in progress — charts will appear shortly.", rawSnapshots)
		}
	}

	return map[string]interface{}{
		"ready":                 enabled && hasData,
		"enabled":               enabled,
		"shared_preload_active": preloadActive,
		"extension_installed":   extInstalled,
		"has_data":              hasData,
		"raw_snapshots":         rawSnapshots,
		"message":               msg,
		"version":               s.PgRepo.GetPgVersion(ctx, instanceName),
	}, nil
}

func (s *MetricsService) GetQueryBottlenecksWithRange(ctx context.Context, serverID uuid.UUID, start, end string) (interface{}, error) {
	return nil, nil
}

func (s *MetricsService) GetSqlServerQueryStoreSQLText(ctx context.Context, serverID uuid.UUID, dbName, sqlHash string) (string, error) {
	return "", nil
}

func (s *MetricsService) RebindTimescale(tsStorage *hot.HotStorage) error {
	s.tsMu.Lock()
	defer s.tsMu.Unlock()
	if s.tsHotStorage != nil {
		s.tsHotStorage.Close()
	}
	s.tsHotStorage = tsStorage
	if tsStorage != nil {
		s.tsLogger = hot.NewTimescaleLogger(tsStorage.Pool())
		s.bindPoolRepos(tsStorage.Pool())
	} else {
		s.tsLogger = nil
	}
	return nil
}

// BindTimescaleRepos wires all TimescaleDB-backed repositories.
// Safe to call at startup or after a pool rebind.
func (s *MetricsService) BindTimescaleRepos(pool *pgxpool.Pool) {
	s.tsMu.Lock()
	defer s.tsMu.Unlock()
	s.bindPoolRepos(pool)
}

// bindPoolRepos is the unlocked core; callers must hold tsMu.
func (s *MetricsService) bindPoolRepos(pool *pgxpool.Pool) {
	s.ServerRepo = repository.NewServerRegistryRepository(pool)
	s.UserRepo = repository.NewUserRepository(pool)
	s.AuditRepo = repository.NewAuditLogRepository(pool)
	s.ConfigRepo = repository.NewCollectorConfigRepository(pool)
	s.ServerSecretBox = security.NewEnvelopeSecretBox()

	// Initialize Postgres Domain Repositories
	s.PgObservabilityRepo = pg_obs_repo.NewPostgresObservabilityRepository(pool)
	s.PgObsCollector = pg_obs_coll.NewPostgresObservabilityCollector(s.PgObservabilityRepo)
	s.PgTimescaleColl = postgres.NewPgTimescaleCollector(s.PgRepo, hot.NewTimescaleLogger(pool))
	s.PgIndexDefCollector = collectors.NewPgIndexDefinitionCollector(s.PgRepo, hot.NewTimescaleLogger(pool))
	s.PgBackupRepo = pg_backup_repo.NewPostgresBackupRepository(pool)
	s.PgDRPolicyRepo = pg_backup_repo.NewDRPolicyRepository(pool)
	s.PgSecurityRepo = pg_security_repo.NewPostgresSecurityRepository(pool)
	s.PgSecurityCollector = pg_security_coll.NewPostgresSecurityCollector(s.PgSecurityRepo)
}

// InitKMSIfNeeded initializes the credential encryption KMS from jwtSecret if it
// hasn't been set yet (e.g. when TimescaleDB is connected via the setup wizard
// rather than being available at startup).
func (s *MetricsService) InitKMSIfNeeded(jwtSecret []byte) {
	if s.ServerKMS != nil {
		return
	}
	kms, _ := config.InitServerRegistryKMS(jwtSecret)
	s.ServerKMS = kms
}

func (s *MetricsService) EnsureServerKMS() error {
	return nil
}

func (s *MetricsService) GetSQLServerRiskHealthHistory(ctx context.Context, serverID uuid.UUID, hours int) (interface{}, error) {
	return nil, nil
}

func (s *MetricsService) GetSqlServerQueryStatsTimeSeries(ctx context.Context, serverID uuid.UUID, metric, from, to, dbName string, excludeSystem bool, monitoringLogins []string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetQueryStatsTimeSeries(ctx, serverID.String(), metric, from, to, dbName, excludeSystem, monitoringLogins)
}

func (s *MetricsService) GetSqlServerWaitStatsTimeSeries(ctx context.Context, serverID uuid.UUID, from, to time.Time) (interface{}, error) {
	if s.tsLogger == nil {
		return []interface{}{}, nil
	}
	return s.tsLogger.GetWaitTrendV2(ctx, serverID, from, to)
}

func (s *MetricsService) GetSqlServerPerfCountersTimeSeries(ctx context.Context, serverID uuid.UUID, from, to time.Time) (interface{}, error) {
	if s.tsLogger == nil {
		return []interface{}{}, nil
	}
	return s.tsLogger.GetSqlServerPerfCounters(ctx, serverID, from.Format(time.RFC3339), to.Format(time.RFC3339), nil)
}

func (s *MetricsService) GetSqlServerFileIOTimeSeries(ctx context.Context, serverID uuid.UUID, from, to time.Time) (interface{}, error) {
	if s.tsLogger == nil {
		return []interface{}{}, nil
	}
	return s.tsLogger.GetIOLatencyTrendV2(ctx, serverID, from, to)
}

func (s *MetricsService) GetSqlServerFileIO(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSqlServerFileIO(ctx, serverID, from, to)
}

func (s *MetricsService) GetSqlServerPlanCacheTimeSeries(ctx context.Context, serverID uuid.UUID, from, to time.Time) (interface{}, error) {
	if s.tsLogger == nil {
		return []interface{}{}, nil
	}
	return s.tsLogger.GetSqlServerPlanCache(ctx, serverID, from.Format(time.RFC3339), to.Format(time.RFC3339))
}

func (s *MetricsService) GetSqlServerMemoryClerksTimeSeries(ctx context.Context, serverID uuid.UUID, from, to time.Time) (interface{}, error) {
	if s.tsLogger == nil {
		return []interface{}{}, nil
	}
	return s.tsLogger.GetSqlServerMemoryClerksV2(ctx, serverID, from.Format(time.RFC3339), to.Format(time.RFC3339))
}

func (s *MetricsService) GetSqlServerMemoryGrantsTimeSeries(ctx context.Context, serverID uuid.UUID, from, to time.Time) (interface{}, error) {
	if s.tsLogger == nil {
		return []interface{}{}, nil
	}
	return s.tsLogger.GetSqlServerMemoryGrantsV2(ctx, serverID, from.Format(time.RFC3339), to.Format(time.RFC3339))
}

func (s *MetricsService) GetSqlServerThroughputTimeSeries(ctx context.Context, serverID uuid.UUID, from, to time.Time) (interface{}, error) {
	if s.tsLogger == nil {
		return []interface{}{}, nil
	}
	return s.tsLogger.GetThroughputTrendV2(ctx, serverID, from, to)
}

func (s *MetricsService) GetLatestSqlServerHealthKPIs(ctx context.Context, serverID uuid.UUID) (interface{}, int64, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{}, 0, nil
	}
	kpis, uptime, err := s.tsLogger.GetLatestHealthV2KPIs(ctx, serverID)
	if err != nil {
		return map[string]interface{}{}, 0, err
	}
	return kpis, uptime, nil
}

func (s *MetricsService) GetSqlServerPerformanceDebt(ctx context.Context, serverID uuid.UUID, lookbackHours int, database string) (models.PerformanceDebtResponse, error) {
	if s.tsLogger == nil {
		return models.PerformanceDebtResponse{Findings: []map[string]interface{}{}}, nil
	}
	lookback := time.Duration(lookbackHours) * time.Hour
	results, err := s.tsLogger.GetPerformanceDebtFindings(ctx, serverID, lookback, database)
	if err != nil {
		return models.PerformanceDebtResponse{Findings: []map[string]interface{}{}}, err
	}
	if results == nil {
		results = []map[string]interface{}{}
	}

	var summary models.PerformanceDebtSummary
	summary.TotalFindings = len(results)
	for _, f := range results {
		sev, _ := f["severity"].(string)
		switch strings.ToUpper(sev) {
		case "CRITICAL":
			summary.CriticalFindings++
		case "WARNING":
			summary.WarningFindings++
		case "INFO":
			summary.InfoFindings++
		}
	}

	summary.Tooltips = map[string]string{
		"total_findings":    "Total number of performance debt findings identified for this instance.",
		"critical_findings": "High-impact issues that should be addressed immediately (e.g., extremely fragmented indexes, disabled autogrowth).",
		"warning_findings":  "Moderate issues that may degrade performance over time (e.g., missing indexes with high impact, stale statistics).",
		"info_findings":     "Informational findings and best practices (e.g., unused indexes with low write volume).",
	}

	return models.PerformanceDebtResponse{
		Findings: results,
		Summary:  summary,
	}, nil
}

func (s *MetricsService) GetSqlServerMemoryDrilldown(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	results, err := s.tsLogger.GetSQLServerMemoryMetricsRange(ctx, serverID, from, to, 200)
	if err != nil {
		return []map[string]interface{}{}, err
	}
	if results == nil {
		results = []map[string]interface{}{}
	}
	return results, nil
}

func (s *MetricsService) GetBestPractices(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	instanceName := s.instanceNameByServerID(serverID)
	if instanceName == "" || s.tsLogger == nil || s.MsRepo == nil {
		return models.BestPracticesResult{InstanceName: instanceName}, nil
	}

	result := models.BestPracticesResult{
		InstanceName: instanceName,
		Timestamp:    fmt.Sprintf("%d", time.Now().Unix()),
	}

	// Server-level config: queries sys.configurations only (not sys.databases).
	result.ServerConfig = s.MsRepo.FetchServerConfigChecks(ctx, instanceName)

	// Database-level config: read from the sqlserver_database_catalog snapshot
	// instead of querying sys.databases live.
	catalogRows, err := s.tsLogger.GetLatestDatabaseCatalog(ctx, serverID)
	if err != nil {
		slog.Info("[GetBestPractices] catalog fetch", "target", instanceName, "err", err)
	}
	if len(catalogRows) > 0 {
		result.DatabaseConfig = evaluateDatabaseConfigsFromCatalog(catalogRows)
		result.DataSource = "timescale"
		result.SnapshotCapturedAt = catalogRows[0].CaptureTimestamp.Format(time.RFC3339)
	} else {
		result.DataSource = "live"
	}

	return result, nil
}

func (s *MetricsService) GetGuardrails(ctx context.Context, serverID uuid.UUID) (interface{}, error) {
	instanceName := s.instanceNameByServerID(serverID)
	if instanceName == "" || s.MsRepo == nil {
		return map[string]interface{}{"status": "unknown"}, nil
	}

	// All DMV-based checks (storage, disk, log backups, long txns, autogrowth, tempdb, resource gov)
	// still use a live connection — none of these query sys.databases.
	result := s.MsRepo.FetchGuardrailsExceptLogHealth(ctx, instanceName)

	// Log health check: use the catalog snapshot instead of querying sys.databases live.
	if s.tsLogger != nil {
		catalogRows, err := s.tsLogger.GetLatestDatabaseCatalog(ctx, serverID)
		if err != nil {
			slog.Info("[GetGuardrails] catalog fetch", "target", instanceName, "err", err)
		}
		result.LogHealth = evaluateLogHealthFromCatalog(catalogRows)
	}

	result.HealthScore, result.HealthStatus = s.MsRepo.ComputeGuardrailsHealth(result)
	result.Summary = s.MsRepo.SummariseGuardrailsRisks(result)

	return result, nil
}

// instanceNameByServerID returns the instance name for the given server UUID,
// or an empty string if not found.
func (s *MetricsService) instanceNameByServerID(serverID uuid.UUID) string {
	if s.Config == nil {
		return ""
	}
	for _, inst := range s.Config.Instances {
		if inst.ServerID == serverID {
			return inst.Name
		}
	}
	return ""
}

// evaluateDatabaseConfigsFromCatalog applies database best-practice rules using
// pre-fetched sqlserver_database_catalog rows (no live sys.databases query).
func evaluateDatabaseConfigsFromCatalog(rows []hot.SQLServerDatabaseCatalogRow) []models.DatabaseConfigCheck {
	var checks []models.DatabaseConfigCheck
	for _, r := range rows {
		check := models.DatabaseConfigCheck{
			DatabaseName:       r.DatabaseName,
			PageVerify:         r.PageVerifyOptionDesc,
			AutoShrink:         r.IsAutoShrinkOn,
			AutoClose:          r.IsAutoCloseOn,
			TargetRecoveryTime: r.TargetRecoveryTimeInSeconds,
			Status:             "GREEN",
		}
		if r.IsAutoShrinkOn {
			check.Status = "RED"
			check.Message = "Auto Shrink is enabled. Turn this off immediately."
		} else if r.IsAutoCloseOn {
			check.Status = "RED"
			check.Message = "Auto Close is enabled. Turn this off immediately."
		} else if !strings.EqualFold(r.PageVerifyOptionDesc, "CHECKSUM") {
			check.Status = "RED"
			check.Message = "Page verify is not set to CHECKSUM."
		} else if r.TargetRecoveryTimeInSeconds == 0 {
			check.Status = "YELLOW"
			check.Message = "Set Target Recovery Time to 60 seconds."
		}
		checks = append(checks, check)
	}
	return checks
}

// evaluateLogHealthFromCatalog applies log-health rules using pre-fetched catalog rows.
// Note: VLF counts and log space metrics are not stored in the catalog and will be zero.
func evaluateLogHealthFromCatalog(rows []hot.SQLServerDatabaseCatalogRow) []models.LogHealthInfo {
	var logs []models.LogHealthInfo
	for _, r := range rows {
		l := models.LogHealthInfo{
			DatabaseName:  r.DatabaseName,
			RecoveryModel: r.RecoveryModelDesc,
			LogReuseWait:  r.LogReuseWaitDesc,
			Severity:      "GREEN",
		}
		if l.LogReuseWait != "NOTHING" && l.LogReuseWait != "" {
			l.Severity = "WARNING"
			l.Message = fmt.Sprintf("Log reuse wait: %s", l.LogReuseWait)
			l.DrillDown = "Transaction log cannot truncate. Active transactions or backup chain breaks preventing log reuse."
			l.MitigationSQL = "-- Check what's blocking log reuse\nSELECT /* SQL_OPTIMA */ name, log_reuse_wait_desc FROM sys.databases WHERE name = '" + l.DatabaseName + "';"
		}
		logs = append(logs, l)
	}
	return logs
}

func (s *MetricsService) GetSqlServerPrimaryAnalysisDatabase(ctx context.Context, serverID uuid.UUID, from, to time.Time) (string, error) {
	if s.tsLogger == nil {
		return "", nil
	}
	return s.tsLogger.GetSqlServerPrimaryAnalysisDatabase(ctx, serverID, from, to)
}

func (s *MetricsService) GetSqlServerPrimaryWorkloadDatabase(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) (string, error) {
	if s.tsLogger == nil {
		return "", nil
	}
	return s.tsLogger.GetSqlServerPrimaryWorkloadDatabase(ctx, serverID, from, to, filter)
}

func (s *MetricsService) GetSqlServerDatabasesInRange(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) ([]domain.SqlServerWorkloadDatabaseActivity, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetSqlServerDatabasesInRange(ctx, serverID, from, to, filter)
}

func (s *MetricsService) GetSqlServerQueryAnalysisSummary(ctx context.Context, serverID uuid.UUID, from, to time.Time, database string, excludeSys bool, monitoringLogins []string) (*hot.SqlServerQueryAnalysisSummaryRow, error) {
	if s.tsLogger == nil {
		return &hot.SqlServerQueryAnalysisSummaryRow{}, nil
	}
	return s.tsLogger.GetSqlServerQueryAnalysisSummary(ctx, serverID, from, to, database, excludeSys, monitoringLogins)
}

func (s *MetricsService) GetSqlServerQueryRegressions(ctx context.Context, serverID uuid.UUID, limit int, monitoringLogins []string) ([]hot.SqlServerQueryRegressionRow, error) {
	if s.tsLogger == nil {
		return []hot.SqlServerQueryRegressionRow{}, nil
	}
	return s.tsLogger.GetSqlServerQueryRegressions(ctx, serverID, limit, monitoringLogins)
}

func (s *MetricsService) GetSqlServerPlanInstability(ctx context.Context, serverID uuid.UUID, limit int) ([]hot.SqlServerPlanInstabilityRow, error) {
	if s.tsLogger == nil {
		return []hot.SqlServerPlanInstabilityRow{}, nil
	}
	return s.tsLogger.GetSqlServerPlanInstability(ctx, serverID, limit)
}

func (s *MetricsService) GetSqlServerTopQueriesAnalysis(ctx context.Context, serverID uuid.UUID, sortBy string, limit int, from, to time.Time, database string, excludeSys bool, monitoringLogins []string) ([]hot.SqlServerTopQueryIntervalRow, error) {
	if s.tsLogger == nil {
		return []hot.SqlServerTopQueryIntervalRow{}, nil
	}
	return s.tsLogger.GetSqlServerTopQueriesFromInterval(ctx, serverID, sortBy, limit, from, to, database, excludeSys, monitoringLogins)
}

func (s *MetricsService) GetSqlServerPrimaryQueryAnalysisDatabase(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) (string, error) {
	if s.tsLogger == nil {
		return "", nil
	}
	return s.tsLogger.GetSqlServerPrimaryQueryAnalysisDatabase(ctx, serverID, from, to, filter)
}

func (s *MetricsService) GetSqlServerTopQueryTrends(ctx context.Context, serverID uuid.UUID, from, to time.Time, database string, excludeSys bool, monitoringLogins []string, limit int, fingerprints []string) (*hot.SqlServerTopQueryTrendsResponse, error) {
	if s.tsLogger == nil {
		return &hot.SqlServerTopQueryTrendsResponse{Series: []hot.SqlServerTopQueryTrendSeries{}}, nil
	}
	return s.tsLogger.GetSqlServerTopQueryTrends(ctx, serverID, from, to, database, excludeSys, monitoringLogins, limit, fingerprints)
}

func (s *MetricsService) ListSqlServerWatchedQueries(ctx context.Context, serverID uuid.UUID) ([]hot.SqlServerWatchedQueryRow, error) {
	if s.tsLogger == nil {
		return []hot.SqlServerWatchedQueryRow{}, nil
	}
	return s.tsLogger.ListSqlServerWatchedQueries(ctx, serverID)
}

func (s *MetricsService) GetSqlServerWatchedQuery(ctx context.Context, id int) (*hot.SqlServerWatchedQueryRow, error) {
	if s.tsLogger == nil {
		return &hot.SqlServerWatchedQueryRow{}, nil
	}
	return s.tsLogger.GetSqlServerWatchedQuery(ctx, id)
}

func (s *MetricsService) AddSqlServerWatchedQuery(ctx context.Context, serverID uuid.UUID, row hot.SqlServerWatchedQueryRow) (int, error) {
	if s.tsLogger == nil {
		return 0, nil
	}
	return s.tsLogger.InsertSqlServerWatchedQuery(ctx, row)
}

func (s *MetricsService) DeleteSqlServerWatchedQuery(ctx context.Context, id int) error {
	if s.tsLogger == nil {
		return nil
	}
	return s.tsLogger.DeleteSqlServerWatchedQuery(ctx, id)
}

func (s *MetricsService) ListWatchedQuerySnapshots(ctx context.Context, serverID uuid.UUID, watchedQueryID int) ([]hot.SqlServerWatchedSnapshotRow, error) {
	if s.tsLogger == nil {
		return []hot.SqlServerWatchedSnapshotRow{}, nil
	}
	// Default to last 7 days if no range provided (future: take range from params)
	to := time.Now()
	from := to.Add(-7 * 24 * time.Hour)
	return s.tsLogger.GetSqlServerWatchedQuerySnapshots(ctx, watchedQueryID, from, to)
}

func (s *MetricsService) ListWatchedQueryEvents(ctx context.Context, watchedQueryID int) ([]hot.SqlServerWatchedEventRow, error) {
	if s.tsLogger == nil {
		return []hot.SqlServerWatchedEventRow{}, nil
	}
	return s.tsLogger.GetSqlServerWatchedQueryEvents(ctx, watchedQueryID)
}

func (s *MetricsService) GetWatchedQuerySnapshot(ctx context.Context, watchedQueryID int, timestamp string) (hot.SqlServerWatchedSnapshotRow, error) {
	if s.tsLogger == nil {
		return hot.SqlServerWatchedSnapshotRow{}, nil
	}
	t, _ := time.Parse(time.RFC3339, timestamp)
	snaps, err := s.tsLogger.GetSqlServerWatchedQuerySnapshots(ctx, watchedQueryID, t.Add(-1*time.Second), t.Add(1*time.Second))
	if err != nil || len(snaps) == 0 {
		return hot.SqlServerWatchedSnapshotRow{}, fmt.Errorf("snapshot not found")
	}
	return snaps[0], nil
}

func (s *MetricsService) GetSqlServerWorkloadSummary(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) (*domain.SqlServerWorkloadSummary, error) {
	if s.tsLogger == nil {
		return &domain.SqlServerWorkloadSummary{}, nil
	}
	return s.tsLogger.GetSqlServerWorkloadSummary(ctx, serverID, from, to, filter)
}

func (s *MetricsService) GetSqlServerWorkloadTrends(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) ([]domain.SqlServerWorkloadTrendPoint, error) {
	if s.tsLogger == nil {
		return []domain.SqlServerWorkloadTrendPoint{}, nil
	}
	trends, err := s.tsLogger.GetSqlServerWorkloadTrends(ctx, serverID, from, to, filter)
	if err != nil {
		return nil, err
	}
	if len(trends) == 0 && filter.Database == "" {
		trends, err = s.tsLogger.GetSqlServerWorkloadTrendsFromPerfCounters(ctx, serverID, from, to)
	}
	return trends, err
}

func (s *MetricsService) GetSqlServerWorkloadTopOffenders(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int, filter domain.WorkloadQueryFilter) ([]domain.SqlServerWorkloadTopQuery, error) {
	if s.tsLogger == nil {
		return []domain.SqlServerWorkloadTopQuery{}, nil
	}
	return s.tsLogger.GetSqlServerWorkloadTopOffenders(ctx, serverID, from, to, limit, filter)
}

func (s *MetricsService) GetSqlServerWorkloadAppLoadTimeline(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) (interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSqlServerWorkloadAppLoadTimeline(ctx, serverID, from, to, filter)
}

func (s *MetricsService) GetSqlServerWorkloadLoginLoadTimeline(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) (interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSqlServerWorkloadLoginLoadTimeline(ctx, serverID, from, to, filter)
}

func (s *MetricsService) GetSqlServerWorkloadTopApps(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int, filter domain.WorkloadQueryFilter) (interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSqlServerWorkloadTopApps(ctx, serverID, from, to, limit, filter)
}

func (s *MetricsService) GetSqlServerWorkloadTopLogins(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int, filter domain.WorkloadQueryFilter) (interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSqlServerWorkloadTopLogins(ctx, serverID, from, to, limit, filter)
}

func (s *MetricsService) GetSQLServerJobDetails(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSQLServerJobDetails(ctx, serverID, from, to)
}

func (s *MetricsService) GetSQLServerJobSchedules(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSQLServerJobSchedules(ctx, serverID, from, to)
}

func (s *MetricsService) GetSQLServerJobFailures(ctx context.Context, serverID uuid.UUID, from, to string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSQLServerJobFailures(ctx, serverID, from, to, limit)
}

func (s *MetricsService) GetSQLServerJobMetrics(ctx context.Context, serverID uuid.UUID, from, to string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSQLServerJobMetrics(ctx, serverID, from, to, limit)
}

func (s *MetricsService) GetSqlServerTempdbConsumers(ctx context.Context, serverID uuid.UUID, from, to string) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSqlServerTempdbConsumers(ctx, serverID, from, to)
}

func (s *MetricsService) GetSQLServerBufferPoolByDB(ctx context.Context, serverID uuid.UUID, from, to string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSQLServerBufferPoolByDBRange(ctx, serverID, from, to, limit)
}

func (s *MetricsService) GetTimescaleSQLServerMetricsRange(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]hot.SQLServerMetricRow, error) {
	if s.tsLogger == nil {
		return []hot.SQLServerMetricRow{}, nil
	}
	return s.tsLogger.GetSQLServerMetricsTimeRange(ctx, serverID, from, to)
}

func (s *MetricsService) GetTimescaleSQLServerMetrics(ctx context.Context, serverID uuid.UUID, limit int) ([]hot.SQLServerMetricRow, error) {
	if s.tsLogger == nil {
		return []hot.SQLServerMetricRow{}, nil
	}
	return s.tsLogger.GetSQLServerMetrics(ctx, serverID, limit)
}

func (s *MetricsService) GetSQLServerAGHealth(ctx context.Context, serverID uuid.UUID) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSQLServerAGHealth(ctx, serverID)
}

func (s *MetricsService) GetSQLServerReplicationStatus(ctx context.Context, serverID uuid.UUID) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSQLServerReplicationStatus(ctx, serverID)
}

func (s *MetricsService) GetSQLServerLogShippingHealth(ctx context.Context, serverID uuid.UUID) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetLogShippingHealth(ctx, serverID)
}

func (s *MetricsService) GetSQLServerHealthV2(ctx context.Context, serverID uuid.UUID, from, to time.Time) (models.HealthV2DashboardResponse, error) {
	var res models.HealthV2DashboardResponse
	var instanceName string
	var databaseName string

	if s.Config != nil {
		for _, inst := range s.Config.Instances {
			if inst.ServerID == serverID {
				instanceName = inst.Name
				databaseName = inst.Database
				break
			}
		}
	}

	if instanceName == "" {
		return res, fmt.Errorf("server ID %s not found in configuration", serverID)
	}

	// 1. Fetch Live Data (DMVs)
	res, err := s.MsRepo.FetchHealthV2(ctx, instanceName)
	if err != nil {
		slog.Error("[MetricsService] Live HealthV2 fetch failed", "err", err)
		// Fallback to empty response if repo fails, but continue to get trends if possible
		res.InstanceName = instanceName
	}
	res.DatabaseName = databaseName

	// 2. Fetch Historical Trends (TimescaleDB)
	if s.tsLogger != nil && s.tsLogger.Pool() != nil {
		trends, _ := s.tsLogger.GetWaitTrendV2(ctx, serverID, from, to)
		res.WaitTrends = trends
		
		io, _ := s.tsLogger.GetIOLatencyTrendV2(ctx, serverID, from, to)
		res.IOLatency = io
		
		tp, _ := s.tsLogger.GetThroughputTrendV2(ctx, serverID, from, to)
		res.Throughput = tp
	}

	res.Tooltips = map[string]string{
		"sql_cpu_pct":            "The percentage of CPU resources currently utilized by the SQL Server process itself.",
		"runnable_tasks":         "The number of tasks waiting for a CPU. High values indicate CPU pressure.",
		"mem_grants_pending":     "Queries waiting for memory to execute. Non-zero values indicate severe memory bottlenecks.",
		"data_cache_life_sec":    "Average time (in seconds) a data page stays in memory. Higher is better; low values indicate memory churning.",
		"memory_utilization_pct": "How much of the 'Target Server Memory' SQL Server has actually committed.",
		"storage_minimum_free_pct": "The percentage of free space on your most full disk volume.",
		"volumes_tracked":        "Total number of unique disk volumes hosting database files for this instance.",
	}

	return res, nil
}

func (s *MetricsService) GetConfig() *config.Config {
	return s.Config
}

func (s *MetricsService) GetSQLServerCPUHistory(ctx context.Context, serverID uuid.UUID, from, to string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetSQLServerCPUHistory(ctx, serverID, from, to, limit)
}

func (s *MetricsService) GetCachedDashboard(serverID uuid.UUID) models.DashboardMetrics {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.dashboardCache[serverID]
}

func (s *MetricsService) SetCachedDashboard(serverID uuid.UUID, data models.DashboardMetrics) {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	s.dashboardCache[serverID] = data
}

func (s *MetricsService) GetDashboardFromTimescale(serverID uuid.UUID) (map[string]interface{}, error) {
	// Wrapper for GetLatestSQLServerKPIs or similar
	return s.GetLatestSQLServerKPIs(context.Background(), serverID)
}

func (s *MetricsService) GetDashboardHomepageV2WithSource(serverID uuid.UUID, from, to string) (interface{}, string) {
	// Simple implementation
	cache := s.GetCachedDashboard(serverID)
	return cache, "cache"
}

func (s *MetricsService) GetPostgresVacuumHistory(ctx context.Context, serverID uuid.UUID, limit int) ([]hot.PostgresVacuumProgressRow, error) {
	if s.tsLogger == nil {
		return []hot.PostgresVacuumProgressRow{}, nil
	}
	return s.tsLogger.GetPostgresVacuumProgress(ctx, serverID, limit)
}

func (s *MetricsService) GetPostgresTableMaintHistory(ctx context.Context, serverID uuid.UUID, database, schema, table string, limit int) ([]hot.PostgresTableMaintResponse, error) {
	if s.tsLogger == nil {
		return []hot.PostgresTableMaintResponse{}, nil
	}
	return s.tsLogger.GetPostgresTableMaintHistory(ctx, serverID, database, schema, table, limit)
}

func (s *MetricsService) GetPostgresDeadlockHistory(ctx context.Context, serverID uuid.UUID, minutes int, limit int) ([]hot.PostgresDeadlockStatRow, error) {
	if s.tsLogger == nil {
		return []hot.PostgresDeadlockStatRow{}, nil
	}
	return s.tsLogger.GetPostgresDeadlocksHistory(ctx, serverID, minutes, limit)
}

func (s *MetricsService) GetPostgresBackupHistory(ctx context.Context, serverID uuid.UUID, limit int) ([]hot.PostgresBackupRunRow, error) {
	if s.tsLogger == nil {
		return []hot.PostgresBackupRunRow{}, nil
	}
	return s.tsLogger.GetPostgresBackupRunHistory(ctx, serverID, limit)
}

func (s *MetricsService) GetPostgresLogSummary(ctx context.Context, serverID uuid.UUID, windowMinutes int) (*hot.PostgresLogSummary, error) {
	if s.tsLogger == nil {
		return &hot.PostgresLogSummary{}, nil
	}
	return s.tsLogger.GetPostgresLogSummary(ctx, serverID, windowMinutes)
}

func (s *MetricsService) GetPostgresLogEvents(ctx context.Context, serverID uuid.UUID, limit int, severity string) ([]hot.PostgresLogEventRow, error) {
	if s.tsLogger == nil {
		return []hot.PostgresLogEventRow{}, nil
	}
	return s.tsLogger.GetPostgresLogEvents(ctx, serverID, limit, severity)
}

func (s *MetricsService) GetPgLocksBlockingTimeline(ctx context.Context, serverID uuid.UUID, from, to time.Time) (map[string]interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"timeline": []interface{}{}, "incidents": []interface{}{}}, nil
	}
	timeline, err := s.tsLogger.GetPgBlockingTimelineRange(ctx, serverID, from, to)
	if err != nil || timeline == nil {
		timeline = []hot.PgBlockingTimelinePoint{}
	}
	incidents, err := s.tsLogger.GetPgBlockingIncidentsRange(ctx, serverID, from, to)
	if err != nil || incidents == nil {
		incidents = []hot.PgBlockingIncident{}
	}
	return map[string]interface{}{"timeline": timeline, "incidents": incidents}, nil
}

func (s *MetricsService) GetPgLocksBlockingTopLockedTables(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	tables, err := s.tsLogger.GetPgTopLockedTablesRange(ctx, serverID, from, to, limit)
	if err != nil || tables == nil {
		return []map[string]interface{}{}, nil
	}
	out := make([]map[string]interface{}, 0, len(tables))
	for _, t := range tables {
		out = append(out, map[string]interface{}{
			"relation_name":    t.RelationName,
			"waiting_count":    t.WaitingCount,
			"max_wait_seconds": t.MaxWaitSec,
		})
	}
	return out, nil
}

func (s *MetricsService) GetPgLocksBlockingDetails(ctx context.Context, serverID uuid.UUID, from, to time.Time) (map[string]interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"blocking_tree": []interface{}{}, "collected_at": time.Now().UTC()}, nil
	}
	resp, err := s.tsLogger.GetPgBlockingDetailsInRange(ctx, serverID, from, to)
	if err != nil || resp == nil {
		return map[string]interface{}{"blocking_tree": []interface{}{}, "collected_at": time.Now().UTC()}, nil
	}
	return map[string]interface{}{
		"blocking_tree": resp.BlockingTree,
		"collected_at":  resp.CollectedAt,
	}, nil
}

func (s *MetricsService) GetPgChronicBlockers(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	blockers, err := s.tsLogger.GetPgChronicBlockersRange(ctx, serverID, from, to, 15)
	if err != nil || blockers == nil {
		return []map[string]interface{}{}, nil
	}
	out := make([]map[string]interface{}, 0, len(blockers))
	for _, b := range blockers {
		out = append(out, map[string]interface{}{
			"query_sample":       b.QuerySample,
			"root_blocker_count": b.RootBlockerCount,
			"total_victims":      b.TotalVictims,
			"avg_duration_sec":   b.AvgDurationSec,
			"max_duration_sec":   b.MaxDurationSec,
		})
	}
	return out, nil
}

func (s *MetricsService) GetPgBlockingIncidentsList(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]hot.PgBlockingIncident, error) {
	if s.tsLogger == nil {
		return []hot.PgBlockingIncident{}, nil
	}
	return s.tsLogger.GetPgBlockingIncidentsRange(ctx, serverID, from, to)
}

func (s *MetricsService) GetSqlServerVitals(ctx context.Context, serverID uuid.UUID) (map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("storage not available")
	}
	mem, err := s.tsLogger.GetLatestSQLServerMemoryMetrics(ctx, serverID)
	if err != nil {
		return nil, err
	}
	vols, err := s.tsLogger.GetLatestSQLServerVolumeStats(ctx, serverID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"memory":  mem,
		"volumes": vols,
	}, nil
}

func (s *MetricsService) GetServerName(serverID uuid.UUID) string {
	for _, inst := range s.Config.Instances {
		if inst.ServerID == serverID {
			return inst.Name
		}
	}
	return ""
}

func (s *MetricsService) GetPostgresBackupRunHistory(ctx context.Context, serverID uuid.UUID, limit int) (interface{}, error) {
	return s.GetPostgresBackupHistory(ctx, serverID, limit)
}
func (s *MetricsService) GetPostgresSettingsSnapshotLatestTwo(ctx context.Context, serverID uuid.UUID) (time.Time, time.Time, []hot.PostgresSettingSnapshotRow, []hot.PostgresSettingSnapshotRow, error) {
	if s.tsLogger == nil {
		return time.Time{}, time.Time{}, nil, nil, nil
	}
	return s.tsLogger.GetPostgresSettingsSnapshotLatestTwo(ctx, serverID)
}
func (s *MetricsService) GetPostgresDeadlocksHistory(ctx context.Context, serverID uuid.UUID, minutes int, limit int) ([]hot.PostgresDeadlockStatRow, error) {
	return s.GetPostgresDeadlockHistory(ctx, serverID, minutes, limit)
}
func (s *MetricsService) GetPostgresBGWriterHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetPostgresBGWriterHistory(ctx, serverID, from, to, limit)
}
func (s *MetricsService) GetPostgresArchiverHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	return s.tsLogger.GetPostgresArchiverHistory(ctx, serverID, from, to, limit)
}

func (s *MetricsService) GetPostgresCheckpointSummary(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) (interface{}, error) {
	if s.tsLogger == nil {
		return []interface{}{}, nil
	}
	data, err := s.tsLogger.GetPostgresBGWriterHistory(ctx, serverID, from, to, limit)
	if err != nil || data == nil {
		return []interface{}{}, nil
	}
	// Enrich each row with checkpoint_req_ratio = checkpoints_req / (checkpoints_req + checkpoints_timed)
	for _, row := range data {
		timed, _ := row["checkpoints_timed"].(int64)
		req, _ := row["checkpoints_req"].(int64)
		total := timed + req
		ratio := 0.0
		if total > 0 {
			ratio = float64(req) / float64(total)
		}
		row["checkpoint_req_ratio"] = ratio
	}
	return data, nil
}
func (s *MetricsService) GetTimescalePostgresVacuumProgress(ctx context.Context, serverID uuid.UUID, limit int) (interface{}, error) {
	return s.GetPostgresVacuumHistory(ctx, serverID, limit)
}
func (s *MetricsService) GetPostgresLockWaitTrend(ctx context.Context, serverID uuid.UUID, windowMinutes int) (map[string]interface{}, error) {
	if s.tsLogger == nil {
		return map[string]interface{}{"labels": []string{}, "lock_waiting_sessions": []int{}}, nil
	}
	labels, counts, err := s.tsLogger.GetPostgresLockWaitHistory(ctx, serverID, windowMinutes, 400)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"labels":                 labels,
		"lock_waiting_sessions": counts,
	}, nil
}
func (s *MetricsService) GetPostgresReplicationLagTrend(ctx context.Context, serverID uuid.UUID, from, to string, limit int) (map[string]hot.PostgresReplicationLagSeries, error) {
	if s.tsLogger == nil {
		return map[string]hot.PostgresReplicationLagSeries{}, nil
	}
	return s.tsLogger.GetPostgresReplicationLagDetail(ctx, serverID, from, to, limit)
}

func (s *MetricsService) GetPostgresReplicationLagHistory(ctx context.Context, serverID uuid.UUID, from, to string, limit int) ([]map[string]interface{}, error) {
	if s.tsLogger == nil {
		return []map[string]interface{}{}, nil
	}
	return s.tsLogger.GetPostgresReplicationLagHistory(ctx, serverID, from, to, limit)
}

func (s *MetricsService) GetPostgresReplicationStatus(ctx context.Context, serverID uuid.UUID) (*models.PgReplicationStats, error) {
	instanceName := ""
	for _, inst := range s.Config.Instances {
		if inst.ServerID == serverID {
			instanceName = inst.Name
			break
		}
	}
	if instanceName == "" {
		return nil, fmt.Errorf("instance not found: %s", serverID)
	}

	stats, err := s.PgRepo.GetReplicationStats(ctx, instanceName)
	if err != nil {
		return nil, err
	}

	// Enrich with latest historical stats if available
	if s.tsLogger != nil {
		latest, err := s.tsLogger.GetLatestPostgresControlCenterStats(ctx, serverID)
		if err == nil && latest != nil {
			if stats.WalGenRateMBps == 0 {
				stats.WalGenRateMBps = latest.WALMBPerMin / 60.0 // convert MB/min to MB/s
			}
		}
		// If BgWriterEffPct is still 0, try to get it from bgwriter history
		if stats.BgWriterEffPct == 0 {
			bgHistory, err := s.tsLogger.GetPostgresBGWriterHistory(ctx, serverID, time.Now().Add(-1*time.Hour), time.Now(), 1)
			if err == nil && len(bgHistory) > 0 {
				h := bgHistory[0]
				bb, _ := h["buffers_backend"].(int64)
				mc, _ := h["maxwritten_clean"].(int64)
				if (bb + mc) > 0 {
					stats.BgWriterEffPct = float64(bb) / float64(bb+mc) * 100
				}
			}
		}
	}

	return stats, nil
}
func (s *MetricsService) GetSqlServerOverview(serverID uuid.UUID) interface{} {
	return nil
}
