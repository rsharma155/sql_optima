// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unified incident feed logger — persists blocking events, long queries,
//
//	deadlock spikes to TimescaleDB for dashboard consumption.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"time"

	"github.com/jackc/pgx/v5"
)

type PgIncidentFeedRow struct {
	Ts           time.Time       `json:"capture_timestamp"`
	ServerID     uuid.UUID       `json:"server_id" db:"server_id"`
	IncidentType string          `json:"incident_type"`
	Severity     string          `json:"severity"`
	PID          int             `json:"pid,omitempty"`
	BlockedCount int             `json:"blocked_count,omitempty"`
	DurationMs   float64         `json:"duration_ms,omitempty"`
	Usename      string          `json:"usename,omitempty"`
	Datname      string          `json:"datname,omitempty"`
	QuerySnippet string          `json:"query_snippet,omitempty"`
	DetailJSON   json.RawMessage `json:"detail,omitempty"`
}

// LogPgIncidentFeed writes a batch of incident rows captured at collection time.
func (tl *TimescaleLogger) LogPgIncidentFeed(ctx context.Context, rows []PgIncidentFeedRow) error {
	if len(rows) == 0 {
		return nil
	}

	q := `
		INSERT INTO monitor.pg_incident_feed_ts (
			capture_timestamp, server_id, incident_type, severity, pid,
			blocked_count, duration_ms, usename, datname,
			query_snippet, detail_json
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`

	batch := &pgx.Batch{}
	for _, r := range rows {
		// Truncate query snippet to 200 chars if needed (already handled by collector usually, but safety first)
		snippet := r.QuerySnippet
		if len(snippet) > 200 {
			snippet = snippet[:197] + "..."
		}

		batch.Queue(q,
			r.Ts, r.ServerID, r.IncidentType, r.Severity, r.PID,
			r.BlockedCount, r.DurationMs, r.Usename, r.Datname,
			snippet, r.DetailJSON,
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("batch insert failed: %v", err)
		}
	}

	return nil
}

// GetPgIncidentFeed returns the most recent incidents for an serverID.
// lookbackMins controls the time window; limit caps total rows returned.
func (tl *TimescaleLogger) GetPgIncidentFeed(ctx context.Context, serverID uuid.UUID, lookbackMins int, limit int) ([]PgIncidentFeedRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if lookbackMins <= 0 {
		lookbackMins = 1440 // default 24 hours
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := `
		SELECT capture_timestamp, server_id, incident_type, severity, pid,
		       blocked_count, duration_ms, usename, datname,
		       query_snippet, detail_json
		FROM monitor.pg_incident_feed_ts
		WHERE server_id = $1
		  AND capture_timestamp >= NOW() - ($2 * INTERVAL '1 minute')
		ORDER BY capture_timestamp DESC
		LIMIT $3
	`

	rows, err := tl.pool.Query(ctx, q, serverID, lookbackMins, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PgIncidentFeedRow
	for rows.Next() {
		var r PgIncidentFeedRow
		err := rows.Scan(
			&r.Ts, &r.ServerID, &r.IncidentType, &r.Severity, &r.PID,
			&r.BlockedCount, &r.DurationMs, &r.Usename, &r.Datname,
			&r.QuerySnippet, &r.DetailJSON,
		)
		if err != nil {
			continue
		}
		result = append(result, r)
	}

	return result, nil
}
