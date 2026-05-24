package application

import (
	"log/slog"
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/collectors/domain"
	"github.com/rsharma155/sql_optima/internal/collectors/domain/enrichment"
	"github.com/rsharma155/sql_optima/internal/collectors/domain/ruleengine"
	"github.com/rsharma155/sql_optima/internal/collectors/domain/sampler"
	"github.com/rsharma155/sql_optima/internal/collectors/domain/scheduler"
	"github.com/rsharma155/sql_optima/internal/telemetry"
)

type CollectorApp struct {
	scheduler      *scheduler.JobSchedulerService
	mssqlRepo      MSSQLRepo
	pgRepo         PGRepo
	writer         MetricsWriter
	filter         *ruleengine.FilterService
	pgPrevCounters map[uuid.UUID]map[int64]domain.PGQuerySnapshot
	mu             sync.Mutex
}

type MSSQLRepo interface {
	FetchSnapshot(ctx context.Context, lastWatermark time.Time) ([]domain.MSSQLQuerySnapshot, error)
	FetchSessionEnrichment(ctx context.Context) ([]domain.MSSQLSessionEnrichment, error)
	GetSqlServerStartTime(ctx context.Context) (time.Time, error)
}

type PGRepo interface {
	FetchSnapshot(ctx context.Context) ([]domain.PGQuerySnapshot, error)
	FetchActivityEnrichment(ctx context.Context) ([]domain.PGActivityEnrichment, error)
}

type SQLServerQueryStatsWriter interface {
	GetInstanceState(ctx context.Context, serverID uuid.UUID) (lastPoll time.Time, startTime time.Time, err error)
	SaveMetrics(ctx context.Context, serverID uuid.UUID, snapshots []domain.MSSQLQuerySnapshot, pollTime time.Time, sqlStartTime time.Time) error
}

type MetricsWriter interface {
	WriteMSSQLMetrics(ctx context.Context, metrics []domain.MSSQLCombinedMetric) error
	WriteMSSQLSessionEnrichment(ctx context.Context, serverID uuid.UUID, enrichments []domain.MSSQLSessionEnrichment) error
	// ReadMSSQLPlanEnrichment returns recently-seen plan enrichments for an
	// serverID. It lets the 1-minute query-snapshot path correlate
	// plan_handle -> login_name/application_name/is_user_workload using rows
	// already populated by the 30-second session-enrichment job, instead of
	// re-querying sys.dm_exec_requests on every cycle.
	ReadMSSQLPlanEnrichment(ctx context.Context, serverID uuid.UUID) ([]domain.MSSQLSessionEnrichment, error)
	WritePGMetrics(ctx context.Context, metrics []domain.PGCombinedMetric) error
	SQLServerQueryStatsWriter
}

func NewCollectorApp(
	scheduler *scheduler.JobSchedulerService,
	mssqlRepo MSSQLRepo,
	pgRepo PGRepo,
	writer MetricsWriter,
	filter *ruleengine.FilterService,
) *CollectorApp {
	return &CollectorApp{
		scheduler:      scheduler,
		mssqlRepo:      mssqlRepo,
		pgRepo:         pgRepo,
		writer:         writer,
		filter:         filter,
		pgPrevCounters: make(map[uuid.UUID]map[int64]domain.PGQuerySnapshot),
	}
}

func (a *CollectorApp) RunCycle(ctx context.Context, serverID uuid.UUID) {
	jobs, err := a.scheduler.GetJobsToRun(ctx)
	if err != nil {
		slog.Error("Error getting jobs", "err", err)
		return
	}

	for _, job := range jobs {
		if job.Name == "sqlserver_query_snapshot" && a.mssqlRepo != nil {
			go a.collectMSSQLQuerySnapshot(ctx, serverID)
		} else if job.Name == "sqlserver_session_enrichment" && a.mssqlRepo != nil {
			go a.collectMSSQLSessionEnrichment(ctx, serverID)
		} else if job.Name == "pg_queries_v2" && a.pgRepo != nil {
			go a.collectPG(ctx, serverID)
		}
		a.scheduler.MarkAsRun(job.Name)
	}
}

func (a *CollectorApp) collectMSSQLQuerySnapshot(ctx context.Context, serverID uuid.UUID) {
	if a.mssqlRepo == nil {
		return
	}

	start := time.Now()
	sid := serverID.String()

	// Individual collection cycles should not run longer than 30s.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. Read Instance State (Watermark & Start Time) from TimescaleDB
	lastPoll, prevStartTime, err := a.writer.GetInstanceState(ctx, serverID)
	if err != nil {
		slog.Error("MSSQL GetInstanceState error", "target", serverID, "err", err)
	}

	// 2. Get SQL Server start time from source
	currStartTime, err := a.mssqlRepo.GetSqlServerStartTime(ctx)
	if err != nil {
		slog.Error("MSSQL GetSqlServerStartTime error", "target", serverID, "err", err)
		telemetry.CollectorCycles.WithLabelValues(sid, "sqlserver", "error").Inc()
		return
	}

	// 3. Detect restart
	watermark := lastPoll
	if !prevStartTime.IsZero() && !currStartTime.Equal(prevStartTime) {
		slog.Info("MSSQL Restart detected for %s. Resetting watermark.", "val", serverID)
		watermark = time.Time{}
	}

	// 4. Fetch Snapshot from DMV
	snapshots, err := a.mssqlRepo.FetchSnapshot(ctx, watermark)
	if err != nil {
		slog.Error("MSSQL FetchSnapshot error", "target", serverID, "err", err)
		telemetry.CollectorCycles.WithLabelValues(sid, "sqlserver", "error").Inc()
		return
	}

	if len(snapshots) == 0 {
		_ = a.writer.SaveMetrics(ctx, serverID, nil, time.Now().UTC(), currStartTime)
		telemetry.CollectorCycles.WithLabelValues(sid, "sqlserver", "success").Inc()
		return
	}

	// 5. Save Metrics (Staging -> Merge -> Snapshot -> Watermark)
	// This now handles both persistent history and enriched V2 metrics in a single canonical path.
	pollTime := time.Now().UTC()
	err = a.writer.SaveMetrics(ctx, serverID, snapshots, pollTime, currStartTime)
	if err != nil {
		slog.Error("MSSQL SaveMetrics error", "target", serverID, "err", err)
		telemetry.CollectorCycles.WithLabelValues(sid, "sqlserver", "error").Inc()
		return
	}

	telemetry.CollectorCycles.WithLabelValues(sid, "sqlserver", "success").Inc()
	telemetry.CollectorDuration.WithLabelValues(sid, "sqlserver", "query_snapshot").Observe(time.Since(start).Seconds())
}

func (a *CollectorApp) collectMSSQLSessionEnrichment(ctx context.Context, serverID uuid.UUID) {
	if a.mssqlRepo == nil {
		return
	}

	enrichments, err := a.mssqlRepo.FetchSessionEnrichment(ctx)
	if err != nil {
		slog.Error("MSSQL FetchSessionEnrichment error", "target", serverID, "err", err)
		return
	}

	err = a.writer.WriteMSSQLSessionEnrichment(ctx, serverID, enrichments)
	if err != nil {
		slog.Error("MSSQL WriteMSSQLSessionEnrichment error", "err", err)
	}
}

func (a *CollectorApp) collectPG(ctx context.Context, serverID uuid.UUID) {
	if a.pgRepo == nil {
		return
	}

	start := time.Now()
	sid := serverID.String()

	// Individual collection cycles should not run longer than 30s.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	snapshots, err := a.pgRepo.FetchSnapshot(ctx)
	if err != nil {
		slog.Error("PG FetchSnapshot error", "server_id", sid, "err", err)
		telemetry.CollectorCycles.WithLabelValues(sid, "postgres", "error").Inc()
		return
	}

	a.mu.Lock()
	prev := a.pgPrevCounters[serverID]
	if prev == nil {
		prev = make(map[int64]domain.PGQuerySnapshot)
	}
	a.mu.Unlock()

	var deltas []domain.PGQuerySnapshot
	newPrev := make(map[int64]domain.PGQuerySnapshot)

	for _, curr := range snapshots {
		newPrev[curr.QueryID] = curr
		if p, ok := prev[curr.QueryID]; ok {
			delta := curr
			delta.Calls = curr.Calls - p.Calls
			delta.TotalExecTime = curr.TotalExecTime - p.TotalExecTime
			delta.Rows = curr.Rows - p.Rows
			delta.SharedBlksHit = curr.SharedBlksHit - p.SharedBlksHit
			delta.SharedBlksRead = curr.SharedBlksRead - p.SharedBlksRead
			delta.TempBlksWritten = curr.TempBlksWritten - p.TempBlksWritten

			if delta.Calls > 0 {
				deltas = append(deltas, delta)
			}
		}
	}

	a.mu.Lock()
	a.pgPrevCounters[serverID] = newPrev
	a.mu.Unlock()

	if len(deltas) == 0 {
		return
	}

	enrichments, err := a.pgRepo.FetchActivityEnrichment(ctx)
	if err != nil {
		slog.Error("PG FetchActivityEnrichment error", "err", err)
	}

	metrics := enrichment.EnrichPG(deltas, enrichments)

	var filtered []domain.PGCombinedMetric
	for _, m := range metrics {
		if a.filter != nil && a.filter.ShouldIgnorePG(m.DatabaseName, m.ApplicationName) {
			continue
		}
		m.ServerID = serverID
		filtered = append(filtered, m)
	}

	if len(filtered) == 0 {
		return
	}

	sampled := sampler.SamplePG(filtered, 50)

	err = a.writer.WritePGMetrics(ctx, sampled)
	if err != nil {
		slog.Error("PG WriteMetrics error", "err", err)
		telemetry.CollectorCycles.WithLabelValues(sid, "postgres", "error").Inc()
		return
	}

	telemetry.CollectorCycles.WithLabelValues(sid, "postgres", "success").Inc()
	telemetry.CollectorDuration.WithLabelValues(sid, "postgres", "query_snapshot").Observe(time.Since(start).Seconds())
}
