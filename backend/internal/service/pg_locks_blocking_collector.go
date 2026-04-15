// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Adaptive polling collector for PostgreSQL locks & blocking incidents.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	pglocks "github.com/rsharma155/sql_optima/internal/collector/pg_locks_blocking"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type pgBlockingAgentState struct {
	ActiveIncidentID int64
	IsWartime        bool
	StartedAt        time.Time
}

func (s *MetricsService) StartPgLocksBlockingCollector(ctx context.Context) {
	if s.tsLogger == nil {
		log.Printf("[PG LocksBlocking] Timescale not connected; collector disabled")
		return
	}
	// Light schema sanity check.
	if err := s.tsLogger.EnsurePgLocksBlockingSchema(ctx); err != nil {
		log.Printf("[PG LocksBlocking] schema not ready; collector disabled: %v", err)
		return
	}

	for _, inst := range s.Config.Instances {
		if inst.Type != "postgres" {
			continue
		}
		instanceName := inst.Name
		go s.runPgLocksBlockingForInstance(ctx, instanceName)
	}
}

func (s *MetricsService) runPgLocksBlockingForInstance(ctx context.Context, instanceName string) {
	state := &pgBlockingAgentState{}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	log.Printf("[PG LocksBlocking] collector started for %s (15s base)", instanceName)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[PG LocksBlocking] collector stopped for %s", instanceName)
			return
		case <-ticker.C:
			db := s.getPgDB(instanceName)
			if db == nil {
				continue
			}
			now := time.Now().UTC()

			// 1) Snapshots
			if err := s.collectPgSessionsSnapshot(ctx, db, instanceName, now); err != nil {
				log.Printf("[PG LocksBlocking] sessions snapshot error for %s: %v", instanceName, err)
			}
			if err := s.collectPgLocksSnapshot(ctx, db, instanceName, now); err != nil {
				log.Printf("[PG LocksBlocking] locks snapshot error for %s: %v", instanceName, err)
			}

			// 2) Blocking pairs + state machine
			pairs, victims, stats, err := s.collectPgBlockingPairs(ctx, db, instanceName, now)
			if err != nil {
				log.Printf("[PG LocksBlocking] blocking pairs error for %s: %v", instanceName, err)
				continue
			}

			if victims > 0 {
				rootPID, rootQuery := s.pickRootBlocker(ctx, db, stats.RootBlockers)
				if !state.IsWartime {
					ticker.Reset(5 * time.Second)
					state.IsWartime = true
					state.StartedAt = now
					id, err := s.tsLogger.OpenPgBlockingIncident(ctx, instanceName, now, rootPID, rootQuery)
					if err != nil {
						log.Printf("[PG LocksBlocking] open incident error for %s: %v", instanceName, err)
					} else {
						state.ActiveIncidentID = id
						log.Printf("[PG LocksBlocking] incident opened for %s id=%d victims=%d depth=%d", instanceName, id, victims, stats.ChainDepth)
					}
				}

				// Persist pairs (for timeline / reconstruction).
				if len(pairs) > 0 {
					_ = s.tsLogger.LogPgBlockingPairs(ctx, pairs)
				}

				if state.ActiveIncidentID != 0 {
					if err := s.tsLogger.UpdatePgBlockingIncident(ctx, state.ActiveIncidentID, victims, rootPID, rootQuery); err != nil {
						log.Printf("[PG LocksBlocking] update incident error for %s: %v", instanceName, err)
					}
				}
				continue
			}

			// no victims
			if state.IsWartime {
				ticker.Reset(15 * time.Second)
				state.IsWartime = false
				if state.ActiveIncidentID != 0 {
					if err := s.tsLogger.ClosePgBlockingIncident(ctx, state.ActiveIncidentID, now); err != nil {
						log.Printf("[PG LocksBlocking] close incident error for %s: %v", instanceName, err)
					} else {
						log.Printf("[PG LocksBlocking] incident resolved for %s id=%d", instanceName, state.ActiveIncidentID)
					}
				}
				state.ActiveIncidentID = 0
			}
		}
	}
}

func (s *MetricsService) getPgDB(instanceName string) *sql.DB {
	db, ok := s.PgRepo.GetConn(instanceName)
	if ok && db != nil {
		return db
	}
	// If the pool entry exists but is nil or missing, attempt a lightweight call that triggers reconnect logic
	// inside repository methods, then re-fetch.
	_, _ = s.PgRepo.GetLocks(instanceName)
	db, ok = s.PgRepo.GetConn(instanceName)
	if !ok || db == nil {
		return nil
	}
	return db
}

func (s *MetricsService) collectPgSessionsSnapshot(ctx context.Context, db *sql.DB, instanceName string, now time.Time) error {
	q := `
		SELECT
			pid,
			COALESCE(usename::text,''),
			COALESCE(datname::text,''),
			COALESCE(application_name::text,''),
			COALESCE(client_addr::text,''),
			COALESCE(state::text,''),
			COALESCE(wait_event_type::text,''),
			COALESCE(wait_event::text,''),
			xact_start,
			query_start,
			state_change,
			COALESCE(query::text,'')
		FROM pg_stat_activity
		WHERE pid <> pg_backend_pid()
		  AND query NOT ILIKE '%pg_stat_activity%'
	`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()

	out := make([]hot.PgSessionSnapshotRow, 0, 128)
	for rows.Next() {
		var pid int
		var us, dn, app, ca, st, wet, we, query string
		var xactStart, queryStart, stateChange sql.NullTime
		if err := rows.Scan(&pid, &us, &dn, &app, &ca, &st, &wet, &we, &xactStart, &queryStart, &stateChange, &query); err != nil {
			continue
		}
		var xs, qs, sc *time.Time
		if xactStart.Valid {
			t := xactStart.Time.UTC()
			xs = &t
		}
		if queryStart.Valid {
			t := queryStart.Time.UTC()
			qs = &t
		}
		if stateChange.Valid {
			t := stateChange.Time.UTC()
			sc = &t
		}
		out = append(out, hot.PgSessionSnapshotRow{
			CollectedAt:     now,
			ServerID:        instanceName,
			PID:             pid,
			UserName:        us,
			DatabaseName:    dn,
			ApplicationName: app,
			ClientAddr:      ca,
			State:           st,
			WaitEventType:   wet,
			WaitEvent:       we,
			XactStart:       xs,
			QueryStart:      qs,
			StateChange:     sc,
			Query:           query,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return s.tsLogger.LogPgSessionSnapshot(ctx, out)
}

func (s *MetricsService) collectPgLocksSnapshot(ctx context.Context, db *sql.DB, instanceName string, now time.Time) error {
	q := `
		SELECT
			l.pid,
			COALESCE(l.locktype::text,''),
			COALESCE(l.mode::text,''),
			l.granted,
			l.relation,
			COALESCE(r.relname::text,''),
			COALESCE(l.transactionid::text,''),
			CASE WHEN l.granted = false THEN EXTRACT(EPOCH FROM (now() - a.state_change)) ELSE 0 END AS waiting_seconds
		FROM pg_locks l
		LEFT JOIN pg_class r ON l.relation = r.oid
		LEFT JOIN pg_stat_activity a ON l.pid = a.pid
		WHERE l.pid <> pg_backend_pid()
	`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()

	out := make([]hot.PgLockSnapshotRow, 0, 256)
	for rows.Next() {
		var pid int
		var lt, mode, relname, txid string
		var granted bool
		var relOID sql.NullInt64
		var waitSec sql.NullFloat64
		if err := rows.Scan(&pid, &lt, &mode, &granted, &relOID, &relname, &txid, &waitSec); err != nil {
			continue
		}
		var oid uint32
		if relOID.Valid && relOID.Int64 > 0 {
			oid = uint32(relOID.Int64)
		}
		out = append(out, hot.PgLockSnapshotRow{
			CollectedAt:    now,
			ServerID:       instanceName,
			PID:            pid,
			LockType:       lt,
			Mode:           mode,
			Granted:        granted,
			RelationOID:    oid,
			RelationName:   relname,
			TransactionID:  txid,
			WaitingSeconds: func() float64 { if waitSec.Valid { return waitSec.Float64 }; return 0 }(),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return s.tsLogger.LogPgLockSnapshot(ctx, out)
}

func (s *MetricsService) collectPgBlockingPairs(ctx context.Context, db *sql.DB, instanceName string, now time.Time) ([]hot.PgBlockingPairRow, int, pglocks.GraphStats, error) {
	// Build pairs from pg_blocking_pids() for any session currently blocked.
	q := `
		SELECT a.pid AS blocked_pid, unnest(pg_blocking_pids(a.pid)) AS blocking_pid
		FROM pg_stat_activity a
		WHERE cardinality(pg_blocking_pids(a.pid)) > 0
	`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, 0, pglocks.GraphStats{}, err
	}
	defer rows.Close()

	var raw []pglocks.Pair
	out := make([]hot.PgBlockingPairRow, 0, 128)
	for rows.Next() {
		var blocked, blocking int
		if err := rows.Scan(&blocked, &blocking); err != nil {
			continue
		}
		raw = append(raw, pglocks.Pair{BlockedPID: blocked, BlockingPID: blocking})
		out = append(out, hot.PgBlockingPairRow{
			CollectedAt: now,
			ServerID:    instanceName,
			BlockedPID:  blocked,
			BlockingPID: blocking,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, pglocks.GraphStats{}, err
	}
	stats := pglocks.AnalyzePairs(raw)
	return out, stats.VictimsDistinct, stats, nil
}

func (s *MetricsService) pickRootBlocker(ctx context.Context, db *sql.DB, roots []int) (*int, string) {
	if len(roots) == 0 {
		return nil, ""
	}
	// Prefer the smallest PID root for stability.
	pid := roots[0]
	q := `SELECT COALESCE(LEFT(query, 500), '') FROM pg_stat_activity WHERE pid = $1`
	var query string
	_ = db.QueryRowContext(ctx, q, pid).Scan(&query)
	query = strings.TrimSpace(query)
	return &pid, query
}

