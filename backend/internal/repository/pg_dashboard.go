// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL dashboard data fetcher for connections, throughput (TPS), and cache hit ratio metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"log"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

// FetchPgCoreThroughputTelemetry computes:
// - Transactions/sec = (delta xact_commit + delta xact_rollback) / 60
// - Cache hit % = delta blks_hit / (delta blks_hit + delta blks_read) * 100
//
// It down-samples pg_stat_database into 1-minute buckets, producing up to 30 minutes of history.
func (c *PgRepository) FetchPgCoreThroughputTelemetry(instanceName string, prev models.PgCoreDashboardCache) models.PgCoreDashboardCache {
	metrics := prev
	metrics.InstanceName = instanceName
	metrics.Timestamp = time.Now().Format("15:04:05")

	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		log.Printf("[POSTGRES] FetchPgCoreThroughputTelemetry: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
			if !ok || db == nil {
				log.Printf("[POSTGRES] FetchPgCoreThroughputTelemetry: reconnect failed for %s", instanceName)
				return metrics
			}
			log.Printf("[POSTGRES] FetchPgCoreThroughputTelemetry: reconnect succeeded for %s", instanceName)
		} else {
			log.Printf("[POSTGRES] FetchPgCoreThroughputTelemetry: reconnect failed for %s", instanceName)
			return metrics
		}
	}

	now := time.Now()
	nowMs := now.UnixMilli()
	minuteKey := now.Format("2006-01-02 15:04")

	// Ensure internal maps exist (even if prev is empty).
	if metrics.PrevDbCounters == nil {
		metrics.PrevDbCounters = make(map[string]models.PgDbCounters)
	}
	if metrics.AggByDB == nil {
		metrics.AggByDB = make(map[string]models.PgThroughputMinuteAgg)
	}
	if metrics.HistoryByDB == nil {
		metrics.HistoryByDB = make(map[string][]models.PgThroughputPoint)
	}
	if metrics.KnownDatabases == nil {
		metrics.KnownDatabases = make(map[string]struct{})
	}

	// 1) Snapshot current pg_stat_database counters.
	snap := make(map[string]models.PgDbCounters)
	var queryErr error

	rows, err := db.Query(`
		SELECT
			datname,
			xact_commit,
			xact_rollback,
			blks_read,
			blks_hit
		FROM pg_stat_database
		WHERE datname IS NOT NULL
		  AND datname NOT LIKE 'template%';
	`)
	if err != nil {
		queryErr = err
	}

	if queryErr == nil {
		defer rows.Close()
		for rows.Next() {
			var dbName string
			var xactCommit, xactRollback, blksRead, blksHit int64
			if err := rows.Scan(&dbName, &xactCommit, &xactRollback, &blksRead, &blksHit); err == nil {
				snap[dbName] = models.PgDbCounters{
					XactCommit:   xactCommit,
					XactRollback: xactRollback,
					BlksRead:     blksRead,
					BlksHit:      blksHit,
				}
			}
		}
	}

	// If query failed, try to reconnect and retry once
	if queryErr != nil {
		log.Printf("[POSTGRES] Query failed for %s, attempting reconnect: %v", instanceName, queryErr)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, dbOk := c.conns[instanceName]
			c.mutex.RUnlock()

			if dbOk && db != nil {
				rows2, err2 := db.Query(`
					SELECT
						datname,
						xact_commit,
						xact_rollback,
						blks_read,
						blks_hit
					FROM pg_stat_database
					WHERE datname IS NOT NULL
					  AND datname NOT LIKE 'template%';
				`)
				if err2 == nil {
					defer rows2.Close()
					for rows2.Next() {
						var dbName string
						var xactCommit, xactRollback, blksRead, blksHit int64
						if err := rows2.Scan(&dbName, &xactCommit, &xactRollback, &blksRead, &blksHit); err == nil {
							snap[dbName] = models.PgDbCounters{
								XactCommit:   xactCommit,
								XactRollback: xactRollback,
								BlksRead:     blksRead,
								BlksHit:      blksHit,
							}
						}
					}
					queryErr = nil
					log.Printf("[POSTGRES] Reconnection successful for %s, query retry succeeded", instanceName)
				} else {
					log.Printf("[POSTGRES] Query retry failed for %s after reconnect: %v", instanceName, err2)
				}
			}
		}
	}

	// If query still failed, still update timestamp and keep previous data.
	if queryErr != nil {
		if queryErr != sql.ErrNoRows {
			log.Printf("[POSTGRES] pg_stat_database snapshot failed for %s: %v", instanceName, queryErr)
		}
		return metrics
	}

	// 2) Track new databases and backfill their history with zeros to keep series aligned.
	for dbName := range snap {
		if _, exists := metrics.KnownDatabases[dbName]; exists {
			continue
		}
		metrics.KnownDatabases[dbName] = struct{}{}

		// Initialize aligned history for this DB (same length as MinuteKeys).
		zeroPoints := make([]models.PgThroughputPoint, len(metrics.MinuteKeys))
		for i := range zeroPoints {
			zeroPoints[i] = models.PgThroughputPoint{
				Tps:           0,
				CacheHitPct:   0,
				TxnDelta:      0,
				BlksReadDelta: 0,
				BlksHitDelta:  0,
			}
		}
		metrics.HistoryByDB[dbName] = zeroPoints

		// Ensure minute-bucket accumulator exists.
		metrics.AggByDB[dbName] = models.PgThroughputMinuteAgg{}
	}

	// If this is the first ever scrape, we only store previous counters.
	if metrics.LastPollUnixMs == 0 {
		metrics.PrevDbCounters = snap
		metrics.LastPollUnixMs = nowMs
		metrics.AggMinuteKey = minuteKey
		// No deltas to accumulate yet.
		for dbName := range snap {
			if _, ok := metrics.AggByDB[dbName]; !ok {
				metrics.AggByDB[dbName] = models.PgThroughputMinuteAgg{}
			}
		}
		return metrics
	}

	// 3) If minute bucket advanced, finalize previous bucket and roll history.
	if metrics.AggMinuteKey == "" {
		metrics.AggMinuteKey = minuteKey
	} else if metrics.AggMinuteKey != minuteKey {
		prevMinuteTime, err := time.ParseInLocation("2006-01-02 15:04", metrics.AggMinuteKey, time.Local)
		if err == nil {
			currMinuteTime, err2 := time.ParseInLocation("2006-01-02 15:04", minuteKey, time.Local)
			if err2 == nil {
				// Finalize the previous minute and backfill any gap minutes with zeros.
				finalizeMinute := func(mk time.Time) {
					metrics.MinuteKeys = append(metrics.MinuteKeys, mk.Format("2006-01-02 15:04"))

					for dbName := range metrics.KnownDatabases {
						agg := metrics.AggByDB[dbName]
						denom := agg.BlksHitDelta + agg.BlksReadDelta

						tps := float64(agg.TxnDelta) / 60.0
						cacheHitPct := 0.0
						if denom > 0 {
							cacheHitPct = (float64(agg.BlksHitDelta) / float64(denom)) * 100.0
						}

						point := models.PgThroughputPoint{
							Tps:           tps,
							CacheHitPct:   cacheHitPct,
							TxnDelta:      agg.TxnDelta,
							BlksReadDelta: agg.BlksReadDelta,
							BlksHitDelta:  agg.BlksHitDelta,
						}

						metrics.HistoryByDB[dbName] = append(metrics.HistoryByDB[dbName], point)
					}

					// Trim to max history.
					if len(metrics.MinuteKeys) > models.MaxPgThroughputHistoryMinutes {
						excess := len(metrics.MinuteKeys) - models.MaxPgThroughputHistoryMinutes
						metrics.MinuteKeys = metrics.MinuteKeys[excess:]

						for dbName := range metrics.KnownDatabases {
							h := metrics.HistoryByDB[dbName]
							if len(h) > models.MaxPgThroughputHistoryMinutes {
								metrics.HistoryByDB[dbName] = h[len(h)-models.MaxPgThroughputHistoryMinutes:]
							}
						}
					}
				}

				// Finalize the bucket represented by metrics.AggMinuteKey (prev bucket).
				finalizeMinute(prevMinuteTime)

				// Backfill intermediate minutes with zeros if the collector was delayed.
				for gap := prevMinuteTime.Add(time.Minute); gap.Before(currMinuteTime); gap = gap.Add(time.Minute) {
					// Zero out agg for gap minutes.
					for dbName := range metrics.KnownDatabases {
						metrics.AggByDB[dbName] = models.PgThroughputMinuteAgg{}
					}
					finalizeMinute(gap)
				}

				// Reset accumulator for the new minute bucket.
				for dbName := range metrics.KnownDatabases {
					metrics.AggByDB[dbName] = models.PgThroughputMinuteAgg{}
				}
				metrics.AggMinuteKey = minuteKey
			}
		}
	}

	// 4) Compute deltas since last poll and accumulate into the current minute bucket.
	for dbName, cur := range snap {
		prevC, hasPrev := metrics.PrevDbCounters[dbName]
		if !hasPrev {
			continue // no delta on first sight for this DB
		}

		// Counter resets can produce negative deltas; clamp to 0.
		dCommit := cur.XactCommit - prevC.XactCommit
		dRollback := cur.XactRollback - prevC.XactRollback
		dRead := cur.BlksRead - prevC.BlksRead
		dHit := cur.BlksHit - prevC.BlksHit

		if dCommit < 0 {
			dCommit = 0
		}
		if dRollback < 0 {
			dRollback = 0
		}
		if dRead < 0 {
			dRead = 0
		}
		if dHit < 0 {
			dHit = 0
		}

		txnDelta := dCommit + dRollback
		agg := metrics.AggByDB[dbName]
		agg.TxnDelta += txnDelta
		agg.BlksReadDelta += dRead
		agg.BlksHitDelta += dHit
		metrics.AggByDB[dbName] = agg
	}

	// 5) Persist current snapshot as the "previous" counters for next delta computation.
	metrics.PrevDbCounters = snap
	metrics.LastPollUnixMs = nowMs

	// Ensure accumulator exists for DBs even if deltas were absent this poll.
	for dbName := range snap {
		if _, ok := metrics.AggByDB[dbName]; !ok {
			metrics.AggByDB[dbName] = models.PgThroughputMinuteAgg{}
		}
	}

	return metrics
}
