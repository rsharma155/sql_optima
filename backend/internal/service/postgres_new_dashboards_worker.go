package service

import (
	"context"
	"log"
	"sync"
	"time"

	pg_backup_coll "github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/collectors"
	pg_obs_coll "github.com/rsharma155/sql_optima/internal/domain/postgres_observability/collectors"
	pg_security_coll "github.com/rsharma155/sql_optima/internal/domain/postgres_security/collectors"
)

func (s *MetricsService) StartPostgresNewDashboardsCollectors(ctx context.Context) {
	log.Println("[PostgresWorker] Starting new dashboard collection loops...")
	// Start 3 separate loops for different domains to handle different frequencies

	// 1. Observability (Sessions, Waits, Load)
	go s.startObservabilityLoop(ctx)

	// 2. Backup & DR
	go s.startBackupDRLoop(ctx)

	// 3. Security
	go s.startSecurityLoop(ctx)
}

func (s *MetricsService) startObservabilityLoop(ctx context.Context) {
	interval := s.FetchInterval(ctx, "Postgres Session Activity", 60*time.Second)
	log.Printf("[PostgresWorker] Observability loop started with interval %v", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runObservabilityCollection()
			// Re-fetch interval in case it changed
			newInterval := s.FetchInterval(ctx, "Postgres Session Activity", 60*time.Second)
			if newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
				log.Printf("[PostgresWorker] Observability loop interval updated to %v", interval)
			}
		}
	}
}

func (s *MetricsService) runObservabilityCollection() {
	if s.PgObservabilityRepo == nil {
		log.Println("[PostgresWorker] Error: PgObservabilityRepo is nil")
		return
	}

	var wg sync.WaitGroup
	for _, inst := range s.Config.Instances {
		if inst.Type != "postgres" {
			continue
		}
		wg.Add(1)
		go func(instanceName string) {
			defer wg.Done()
			db, ok := s.PgRepo.GetConn(instanceName)
			if !ok || db == nil {
				log.Printf("[PostgresWorker] No connection for instance %s", instanceName)
				return
			}
			collector := pg_obs_coll.NewPostgresObservabilityCollector(s.PgObservabilityRepo, instanceName)
			
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			if err := collector.CollectSessionActivity(ctx, db); err != nil {
				log.Printf("[PostgresWorker] [%s] SessionActivity failed: %v", instanceName, err)
			}
			if err := collector.CollectWaitSummary(ctx, db); err != nil {
				log.Printf("[PostgresWorker] [%s] WaitSummary failed: %v", instanceName, err)
			}
			if err := collector.CollectDBLoad(ctx, db); err != nil {
				log.Printf("[PostgresWorker] [%s] DBLoad failed: %v", instanceName, err)
			}
			if err := collector.CollectQueryWaitProfile(ctx, db); err != nil {
				log.Printf("[PostgresWorker] [%s] QueryWaitProfile failed: %v", instanceName, err)
			}
		}(inst.Name)
	}
	wg.Wait()
}

func (s *MetricsService) startBackupDRLoop(ctx context.Context) {
	interval := s.FetchInterval(ctx, "Postgres Backup Archiver", 300*time.Second)
	log.Printf("[PostgresWorker] BackupDR loop started with interval %v", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runBackupDRCollection()
			newInterval := s.FetchInterval(ctx, "Postgres Backup Archiver", 300*time.Second)
			if newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
				log.Printf("[PostgresWorker] BackupDR loop interval updated to %v", interval)
			}
		}
	}
}

func (s *MetricsService) runBackupDRCollection() {
	if s.PgBackupRepo == nil {
		log.Println("[PostgresWorker] Error: PgBackupRepo is nil")
		return
	}

	var wg sync.WaitGroup
	for _, inst := range s.Config.Instances {
		if inst.Type != "postgres" {
			continue
		}
		wg.Add(1)
		go func(instanceName string) {
			defer wg.Done()
			db, ok := s.PgRepo.GetConn(instanceName)
			if !ok || db == nil {
				return
			}
			collector := pg_backup_coll.NewPostgresBackupCollector(s.PgBackupRepo, instanceName)
			
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			_ = collector.CollectArchiverStats(ctx, db)
			_ = collector.CollectWALRate(ctx, db)
			_ = collector.CollectBaseBackupHistory(ctx, db)
		}(inst.Name)
	}
	wg.Wait()
}

func (s *MetricsService) startSecurityLoop(ctx context.Context) {
	interval := s.FetchInterval(ctx, "Postgres Roles Snapshot", 900*time.Second)
	log.Printf("[PostgresWorker] Security loop started with interval %v", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runSecurityCollection()
			newInterval := s.FetchInterval(ctx, "Postgres Roles Snapshot", 900*time.Second)
			if newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
				log.Printf("[PostgresWorker] Security loop interval updated to %v", interval)
			}
		}
	}
}

func (s *MetricsService) runSecurityCollection() {
	if s.PgSecurityRepo == nil {
		log.Println("[PostgresWorker] Error: PgSecurityRepo is nil")
		return
	}

	var wg sync.WaitGroup
	for _, inst := range s.Config.Instances {
		if inst.Type != "postgres" {
			continue
		}
		wg.Add(1)
		go func(instanceName string) {
			defer wg.Done()
			db, ok := s.PgRepo.GetConn(instanceName)
			if !ok || db == nil {
				return
			}
			collector := pg_security_coll.NewPostgresSecurityCollector(s.PgSecurityRepo, instanceName)
			
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			_ = collector.CollectRoleSnapshot(ctx, db)
			_ = collector.CollectDDLActivity(ctx, db)
		}(inst.Name)
	}
	wg.Wait()
}
