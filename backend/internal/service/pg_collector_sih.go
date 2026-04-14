// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Storage & Index Health (SIH) ingestion tick — Timescale deltas for index/table usage, growth, definitions, and daily unused candidates.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"log"
	"time"

	"github.com/rsharma155/sql_optima/internal/collectors"
)

// runPostgresStorageIndexHealthTick collects SIH metrics for one PostgreSQL instance when Timescale logging is enabled.
// Cadences match SQL Server SIH: 15m index, 15m table usage, 6h size history, 24h index definitions + daily unused refresh.
func (s *MetricsService) runPostgresStorageIndexHealthTick(ctx context.Context, instanceName string) {
	if s == nil || s.PgRepo == nil || s.tsLogger == nil {
		return
	}
	capture := time.Now().UTC()
	now := capture
	due15mIndex := s.sihDue(s.sihLastIndex15m, instanceName, now, 15*time.Minute)
	due15mTable := s.sihDue(s.sihLastTable15m, instanceName, now, 15*time.Minute)
	due6hGrowth := s.sihDue(s.sihLastGrowth6h, instanceName, now, 6*time.Hour)
	dueDailyDefs := s.sihDue(s.sihLastDefsDaily, instanceName, now, 24*time.Hour)

	// PostgreSQL pg_stat_user_* views are scoped to the connected database.
	// Our default connection pool uses dbname=postgres for discovery; for SIH we need to iterate real DBs.
	dbs, err := s.PgRepo.GetDatabases(instanceName)
	if err != nil || len(dbs) == 0 {
		log.Printf("[Collector][SIH] GetDatabases failed or empty for %s: %v", instanceName, err)
		return
	}

	var totalIdxRows, totalTblRows, totalDefRows int
	var insertedIdx, insertedTbl, insertedGrowth, insertedDefs int

	for _, dbName := range dbs {
		// Avoid blocking the whole tick on one bad DB.
		dbCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		pgdb, derr := s.PgRepo.OpenConnForDatabase(dbCtx, instanceName, dbName)
		if derr != nil {
			cancel()
			log.Printf("[Collector][SIH] OpenConnForDatabase failed for %s db=%s: %v", instanceName, dbName, derr)
			continue
		}

		if due15mIndex {
			idxRows, err := collectors.CollectPostgresIndexUsage(dbCtx, pgdb)
			if err != nil {
				log.Printf("[Collector][SIH] CollectPostgresIndexUsage failed for %s db=%s: %v", instanceName, dbName, err)
			} else if len(idxRows) > 0 {
				totalIdxRows += len(idxRows)
				if n, perr := collectors.PersistPostgresIndexUsageDeltas(dbCtx, s.tsLogger, instanceName, idxRows, capture); perr != nil {
					log.Printf("[Collector][SIH] PersistPostgresIndexUsageDeltas failed for %s db=%s: %v", instanceName, dbName, perr)
				} else {
					insertedIdx += n
				}
			}
		}

		if due15mTable || due6hGrowth {
			tblRows, err := collectors.CollectPostgresTableUsageAndSize(dbCtx, pgdb)
			if err != nil {
				log.Printf("[Collector][SIH] CollectPostgresTableUsageAndSize failed for %s db=%s: %v", instanceName, dbName, err)
			} else if len(tblRows) > 0 {
				totalTblRows += len(tblRows)
				if due15mTable {
					if n, perr := collectors.PersistPostgresTableUsageDeltas(dbCtx, s.tsLogger, instanceName, tblRows, capture); perr != nil {
						log.Printf("[Collector][SIH] PersistPostgresTableUsageDeltas failed for %s db=%s: %v", instanceName, dbName, perr)
					} else {
						insertedTbl += n
					}
				}
				if due6hGrowth {
					if n, perr := collectors.PersistPostgresTableSizeHistory(dbCtx, s.tsLogger, instanceName, tblRows, capture); perr != nil {
						log.Printf("[Collector][SIH] PersistPostgresTableSizeHistory failed for %s db=%s: %v", instanceName, dbName, perr)
					} else {
						insertedGrowth += n
					}
				}
			}
		}

		if dueDailyDefs {
			dayBucket := time.Date(capture.Year(), capture.Month(), capture.Day(), 0, 0, 0, 0, time.UTC)
			defRows, err := collectors.CollectPostgresIndexDefinitions(dbCtx, pgdb)
			if err != nil {
				log.Printf("[Collector][SIH] CollectPostgresIndexDefinitions failed for %s db=%s: %v", instanceName, dbName, err)
			} else if len(defRows) > 0 {
				totalDefRows += len(defRows)
				if n, perr := collectors.PersistPostgresIndexDefinitions(dbCtx, s.tsLogger, instanceName, defRows, dayBucket); perr != nil {
					log.Printf("[Collector][SIH] PersistPostgresIndexDefinitions failed for %s db=%s: %v", instanceName, dbName, perr)
				} else {
					insertedDefs += n
				}
			}
		}

		_ = pgdb.Close()
		cancel()
	}

	if due15mIndex {
		log.Printf("[Collector][SIH] postgres index usage persisted for %s dbs=%d rows=%d inserted=%d", instanceName, len(dbs), totalIdxRows, insertedIdx)
	}
	if due15mTable {
		log.Printf("[Collector][SIH] postgres table usage persisted for %s dbs=%d rows=%d inserted=%d", instanceName, len(dbs), totalTblRows, insertedTbl)
	}
	if due6hGrowth {
		log.Printf("[Collector][SIH] postgres table_size_history for %s dbs=%d rows=%d inserted=%d", instanceName, len(dbs), totalTblRows, insertedGrowth)
	}
	if dueDailyDefs {
		log.Printf("[Collector][SIH] postgres index_definitions for %s dbs=%d rows=%d inserted=%d", instanceName, len(dbs), totalDefRows, insertedDefs)
	}

	if dueDailyDefs {
		if n, err := s.tsLogger.RefreshIndexUnusedCandidatesDaily(ctx, "postgres", instanceName, capture, 100); err != nil {
			log.Printf("[Collector][SIH] Daily unused index snapshot failed for postgres %s: %v", instanceName, err)
		} else {
			log.Printf("[Collector][SIH] Daily unused index snapshot rows for postgres %s: %d", instanceName, n)
		}
	}
}
