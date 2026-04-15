You are an expert Golang developer and PostgreSQL DBA. We are upgrading the locks and blocking dashboard for our monitoring tool, sql_optima.
We are shifting from "Snapshot Monitoring" to "Stateful Incident Monitoring." The goal is to detect blocking incidents, reconstruct blocking chains to find the root blocker, and calculate severity scores to aid immediate DBA intervention.

Phase 1 Execution Plan:

1. Database Schema (TimescaleDB)
Execute this schema setup. We will use TimescaleDB hypertables to manage the timeseries data efficiently.

SQL

-- 1.1 Session State Snapshot
CREATE TABLE monitor.session_snapshot (
    collected_at timestamptz NOT NULL,
    pid int,
    usename text,
    datname text,
    application_name text,
    client_addr inet,
    state text,
    wait_event_type text,
    wait_event text,
    xact_start timestamptz,
    query_start timestamptz,
    state_change timestamptz,
    query text
);
SELECT create_hypertable('monitor.session_snapshot', 'collected_at', if_not_exists=>TRUE);

-- 1.2 Locks Snapshot
CREATE TABLE monitor.lock_snapshot (
    collected_at timestamptz NOT NULL,
    pid int,
    locktype text,
    mode text,
    granted boolean,
    relation oid,
    transactionid xid
);
SELECT create_hypertable('monitor.lock_snapshot', 'collected_at', if_not_exists=>TRUE);

-- 1.3 Blocking Pairs (The core dependency graph)
CREATE TABLE monitor.blocking_pairs (
    collected_at timestamptz NOT NULL,
    blocked_pid int,
    blocking_pid int
);
SELECT create_hypertable('monitor.blocking_pairs', 'collected_at', if_not_exists=>TRUE);

-- 1.4 Incident State Tracking
CREATE TABLE monitor.blocking_incident (
    incident_id BIGSERIAL PRIMARY KEY,
    started_at timestamptz NOT NULL,
    ended_at timestamptz,
    root_blocker_pid int,
    root_blocker_query text,
    peak_blocked_sessions int DEFAULT 0,
    status text DEFAULT 'active' -- 'active' or 'resolved'
);
2. Golang Collector Architecture (Adaptive Polling)
Write the Go routines to collect this data. Implement an Adaptive Ticker: Poll every 15 seconds normally. If a blocking pair is detected, shift the polling to every 5 seconds until the incident resolves.

Here is the architectural baseline for the Go code:

Go
package main

import (
	"context"
	"database/sql"
	"log"
	"time"
)

type AgentState struct {
	ActiveIncidentID int64
	IsWartime        bool
}

func startCollector(db *sql.DB) {
	state := &AgentState{}
	interval := 15 * time.Second
	ticker := time.NewTicker(interval)

	for {
		<-ticker.C
		
		// 1. Snapshot Data (Run these concurrently in production)
		collectSessions(db)
		collectLocks(db)
		victims := collectBlockingPairs(db)

		// 2. State Machine / Adaptive Ticker Logic
		if victims > 0 {
			if !state.IsWartime {
				log.Println("Blocking detected! Shifting to 5s wartime polling.")
				ticker.Reset(5 * time.Second)
				state.IsWartime = true
				state.ActiveIncidentID = openIncident(db)
			}
			updateIncident(db, state.ActiveIncidentID, victims)
		} else {
			if state.IsWartime {
				log.Println("Blocking resolved. Returning to 15s peacetime polling.")
				ticker.Reset(15 * time.Second)
				state.IsWartime = false
				closeIncident(db, state.ActiveIncidentID)
				state.ActiveIncidentID = 0
			}
		}
	}
}

func collectBlockingPairs(db *sql.DB) int {
	query := `
		WITH inserted AS (
			INSERT INTO monitor.blocking_pairs (collected_at, blocked_pid, blocking_pid)
			SELECT now(), a.pid, unnest(pg_blocking_pids(a.pid))
			FROM pg_stat_activity a
			WHERE cardinality(pg_blocking_pids(a.pid)) > 0
			RETURNING blocked_pid
		)
		SELECT count(DISTINCT blocked_pid) FROM inserted;
	`
	var victims int
	err := db.QueryRow(query).Scan(&victims)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Error collecting pairs: %v", err)
	}
	return victims
}

// Implement openIncident, updateIncident, and closeIncident stubs.
// openIncident should calculate the root blocker using:
// SELECT blocking_pid FROM monitor.blocking_pairs WHERE blocking_pid NOT IN (SELECT blocked_pid FROM monitor.blocking_pairs)
3. API & Dashboard Requirements
Generate the necessary API endpoints and frontend layout to support the following:

Realtime Status Row (KPIs):

Active Blocking Sessions (Red if > 0).

Root Blocker PID & Query snippet.

Idle in Transaction Risk (Count of sessions where state='idle in transaction' and age > 30s).

Incident Timeline (Chart):

A line chart plotting blocked_sessions over the last 24 hours (querying blocking_pairs bucketed by minute), with shaded red regions indicating the started_at to ended_at windows from the blocking_incident table.

Top Offenders (Tables):

Top Locked Tables (Join lock_snapshot on pg_class where granted=false).

Live Blocking Graph (Visualizing the chain from blocking_pairs).

Severity Score Logic (Frontend computation):

Score = (victims * 10) + (chain_depth * 5) + (idle_in_txn_count * 30) + (incident_duration_mins * 2)

Coloring: 0-20 Green, 20-50 Yellow, 50-80 Orange, 80+ Red.

Please begin by generating the complete Golang collector logic and the corresponding openIncident/closeIncident database interactions.