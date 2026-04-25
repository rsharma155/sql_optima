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
		if job.Name == "sqlserver_queries_v2" && a.mssqlRepo != nil {
			go a.collectMSSQL(ctx, instanceID)
		} else if job.Name == "pg_queries_v2" && a.pgRepo != nil {
			go a.collectPG(ctx, instanceID)
		}
		a.scheduler.MarkAsRun(job.Name)
	}
}

func (a *CollectorApp) collectMSSQL(ctx context.Context, instanceID string) {
	if a.mssqlRepo == nil {
		return
	}

	// 1. Read Instance State (Watermark & Start Time) from TimescaleDB
	lastPoll, prevStartTime, err := a.writer.GetInstanceState(ctx, instanceID)
	if err != nil {
		log.Printf("MSSQL GetInstanceState error for %s: %v", instanceID, err)
		// Continue with zero values if error
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
		watermark = time.Time{} // Pull all rows on restart
	}

	// 4. Fetch Snapshot from DMV
	snapshots, err := a.mssqlRepo.FetchSnapshot(ctx, watermark)
	if err != nil {
		log.Printf("MSSQL FetchSnapshot error for %s: %v", instanceID, err)
		return
	}

	if len(snapshots) == 0 {
		// Even if no new rows, we might want to update last_successful_run
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
