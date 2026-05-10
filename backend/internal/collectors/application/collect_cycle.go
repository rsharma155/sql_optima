package application

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/rsharma155/sql_optima/internal/collectors/domain"
	"github.com/rsharma155/sql_optima/internal/collectors/domain/enrichment"
	"github.com/rsharma155/sql_optima/internal/collectors/domain/ruleengine"
	"github.com/rsharma155/sql_optima/internal/collectors/domain/sampler"
	"github.com/rsharma155/sql_optima/internal/collectors/domain/scheduler"
)

type CollectorApp struct {
	scheduler       *scheduler.JobSchedulerService
	mssqlRepo       MSSQLRepo
	pgRepo          PGRepo
	writer          MetricsWriter
	filter          *ruleengine.FilterService
	mssqlWatermarks map[string]time.Time
	mssqlPrevCounters map[string]map[string]domain.MSSQLQuerySnapshot
	pgPrevCounters  map[string]map[int64]domain.PGQuerySnapshot
	mu              sync.Mutex
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
	GetInstanceState(ctx context.Context, instanceID string) (lastPoll time.Time, startTime time.Time, err error)
	SaveMetrics(ctx context.Context, instanceID string, snapshots []domain.MSSQLQuerySnapshot, pollTime time.Time, sqlStartTime time.Time) error
}

type MetricsWriter interface {
	WriteMSSQLMetrics(ctx context.Context, metrics []domain.MSSQLCombinedMetric) error
	WriteMSSQLSessionEnrichment(ctx context.Context, instanceID string, enrichments []domain.MSSQLSessionEnrichment) error
	// ReadMSSQLPlanEnrichment returns recently-seen plan enrichments for an
	// instance. It lets the 1-minute query-snapshot path correlate
	// plan_handle -> login_name/application_name/is_user_workload using rows
	// already populated by the 30-second session-enrichment job, instead of
	// re-querying sys.dm_exec_requests on every cycle.
	ReadMSSQLPlanEnrichment(ctx context.Context, instanceID string) ([]domain.MSSQLSessionEnrichment, error)
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
		scheduler:       scheduler,
		mssqlRepo:       mssqlRepo,
		pgRepo:          pgRepo,
		writer:          writer,
		filter:          filter,
		mssqlWatermarks: make(map[string]time.Time),
		mssqlPrevCounters: make(map[string]map[string]domain.MSSQLQuerySnapshot),
		pgPrevCounters:  make(map[string]map[int64]domain.PGQuerySnapshot),
	}
}

func (a *CollectorApp) RunCycle(ctx context.Context, instanceID string) {
	jobs, err := a.scheduler.GetJobsToRun(ctx)
	if err != nil {
		log.Printf("Error getting jobs: %v", err)
		return
	}

	for _, job := range jobs {
		if job.Name == "sqlserver_query_snapshot" && a.mssqlRepo != nil {
			go a.collectMSSQLQuerySnapshot(ctx, instanceID)
		} else if job.Name == "sqlserver_session_enrichment" && a.mssqlRepo != nil {
			go a.collectMSSQLSessionEnrichment(ctx, instanceID)
		} else if job.Name == "pg_queries_v2" && a.pgRepo != nil {
			go a.collectPG(ctx, instanceID)
		}
		a.scheduler.MarkAsRun(job.Name)
	}
}

func (a *CollectorApp) collectMSSQLQuerySnapshot(ctx context.Context, instanceID string) {
	if a.mssqlRepo == nil {
		return
	}

	// 1. Read Instance State (Watermark & Start Time) from TimescaleDB
	lastPoll, prevStartTime, err := a.writer.GetInstanceState(ctx, instanceID)
	if err != nil {
		log.Printf("MSSQL GetInstanceState error for %s: %v", instanceID, err)
	}

	// 2. Get SQL Server start time from source
	currStartTime, err := a.mssqlRepo.GetSqlServerStartTime(ctx)
	if err != nil {
		log.Printf("MSSQL GetSqlServerStartTime error for %s: %v", instanceID, err)
		return
	}

	// 3. Detect restart
	watermark := lastPoll
	if !prevStartTime.IsZero() && !currStartTime.Equal(prevStartTime) {
		log.Printf("MSSQL Restart detected for %s. Resetting watermark.", instanceID)
		watermark = time.Time{}
	}

	// 4. Fetch Snapshot from DMV
	snapshots, err := a.mssqlRepo.FetchSnapshot(ctx, watermark)
	if err != nil {
		log.Printf("MSSQL FetchSnapshot error for %s: %v", instanceID, err)
		return
	}

	if len(snapshots) == 0 {
		_ = a.writer.SaveMetrics(ctx, instanceID, nil, time.Now().UTC(), currStartTime)
		return
	}

	// 5. Save Metrics (Staging -> Merge -> Snapshot -> Watermark)
	pollTime := time.Now().UTC()
	err = a.writer.SaveMetrics(ctx, instanceID, snapshots, pollTime, currStartTime)
	if err != nil {
		log.Printf("MSSQL SaveMetrics error for %s: %v", instanceID, err)
		return
	}

	// 6. Compute Deltas, Enrich and Write to V2 Metrics
	a.mu.Lock()
	prev := a.mssqlPrevCounters[instanceID]
	if prev == nil {
		prev = make(map[string]domain.MSSQLQuerySnapshot)
	}
	a.mu.Unlock()

	var deltas []domain.MSSQLQuerySnapshot
	newPrev := make(map[string]domain.MSSQLQuerySnapshot)

	for _, curr := range snapshots {
		key := string(curr.PlanHandle)
		newPrev[key] = curr
		if p, ok := prev[key]; ok {
			delta := curr
			delta.TotalCPUMs = curr.TotalCPUMs - p.TotalCPUMs
			delta.TotalExecutions = curr.TotalExecutions - p.TotalExecutions
			delta.TotalLogicalReads = curr.TotalLogicalReads - p.TotalLogicalReads
			delta.TotalPhysicalReads = curr.TotalPhysicalReads - p.TotalPhysicalReads
			delta.TotalRows = curr.TotalRows - p.TotalRows

			if delta.TotalExecutions > 0 {
				deltas = append(deltas, delta)
			}
		} else {
			deltas = append(deltas, curr)
		}
	}

	a.mu.Lock()
	a.mssqlPrevCounters[instanceID] = newPrev
	a.mu.Unlock()

	if len(deltas) == 0 {
		return
	}

	// Read enrichment from sqlserver_plan_enrichment, populated by the
	// independent 30s session-enrichment job. Falls back to an empty set on
	// error — EnrichMSSQL defaults IsUserWorkload=1 and leaves login/app
	// fields empty when no enrichment is found, which is the safe behaviour
	// for the first cycle after start-up.
	enrichments, err := a.writer.ReadMSSQLPlanEnrichment(ctx, instanceID)
	if err != nil {
		log.Printf("MSSQL ReadMSSQLPlanEnrichment error for %s: %v", instanceID, err)
		enrichments = nil
	}
	metrics := enrichment.EnrichMSSQL(deltas, enrichments)

	var filtered []domain.MSSQLCombinedMetric
	for _, m := range metrics {
		if a.filter != nil && a.filter.ShouldIgnoreMSSQL(m.DatabaseName, m.LoginName, m.ApplicationName) {
			continue
		}
		m.InstanceID = instanceID
		filtered = append(filtered, m)
	}

	if len(filtered) == 0 {
		return
	}

	sampled := sampler.SampleMSSQL(filtered, 50)
	err = a.writer.WriteMSSQLMetrics(ctx, sampled)
	if err != nil {
		log.Printf("MSSQL WriteMSSQLMetrics error: %v", err)
	}
}

func (a *CollectorApp) collectMSSQLSessionEnrichment(ctx context.Context, instanceID string) {
	if a.mssqlRepo == nil {
		return
	}

	enrichments, err := a.mssqlRepo.FetchSessionEnrichment(ctx)
	if err != nil {
		log.Printf("MSSQL FetchSessionEnrichment error: %v", err)
		return
	}

	err = a.writer.WriteMSSQLSessionEnrichment(ctx, instanceID, enrichments)
	if err != nil {
		log.Printf("MSSQL WriteMSSQLSessionEnrichment error: %v", err)
	}
}

func (a *CollectorApp) collectPG(ctx context.Context, instanceID string) {
	if a.pgRepo == nil {
		return
	}
	snapshots, err := a.pgRepo.FetchSnapshot(ctx)
	if err != nil {
		log.Printf("PG FetchSnapshot error: %v", err)
		return
	}

	a.mu.Lock()
	prev := a.pgPrevCounters[instanceID]
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
	a.pgPrevCounters[instanceID] = newPrev
	a.mu.Unlock()

	if len(deltas) == 0 {
		return
	}

	enrichments, err := a.pgRepo.FetchActivityEnrichment(ctx)
	if err != nil {
		log.Printf("PG FetchActivityEnrichment error: %v", err)
	}

	metrics := enrichment.EnrichPG(deltas, enrichments)

	var filtered []domain.PGCombinedMetric
	for _, m := range metrics {
		if a.filter != nil && a.filter.ShouldIgnorePG(m.DatabaseName, m.ApplicationName) {
			continue
		}
		m.InstanceID = instanceID
		filtered = append(filtered, m)
	}

	if len(filtered) == 0 {
		return
	}

	sampled := sampler.SamplePG(filtered, 50)

	err = a.writer.WritePGMetrics(ctx, sampled)
	if err != nil {
		log.Printf("PG WriteMetrics error: %v", err)
	}
}
