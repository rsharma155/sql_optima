// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB storage-layer for performance debt findings (unused/missing indexes).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// normalizePerformanceDebtDetails ensures values satisfy the JSONB column (empty string is invalid).
func normalizePerformanceDebtDetails(details string) string {
	details = strings.TrimSpace(details)
	if details == "" {
		return "{}"
	}
	if json.Valid([]byte(details)) {
		return details
	}
	b, err := json.Marshal(map[string]string{"message": details})
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (tl *TimescaleLogger) LogPerformanceDebtFindings(ctx context.Context, serverID uuid.UUID, findings []PerformanceDebtFindingRow) error {
	if len(findings) == 0 {
		return nil
	}

	// Deduplicate: skip findings whose finding_key + recommendation already
	// exists in the last 12 h for this server. This prevents duplicate rows
	// from accumulating across collection cycles when the underlying condition
	// has not changed.
	const dedupeQ = `
		SELECT finding_key, recommendation
		FROM sqlserver_performance_debt_findings
		WHERE server_id = $1
		  AND capture_timestamp >= NOW() - INTERVAL '12 hours'`

	existing := make(map[string]string)
	if rows, err := tl.pool.Query(ctx, dedupeQ, serverID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var key, rec string
			if scanErr := rows.Scan(&key, &rec); scanErr == nil {
				existing[key] = rec
			}
		}
	}

	var toInsert []PerformanceDebtFindingRow
	for _, f := range findings {
		prevRec, seen := existing[f.FindingKey]
		if !seen || prevRec != f.Recommendation {
			toInsert = append(toInsert, f)
		}
	}
	if len(toInsert) == 0 {
		return nil
	}

	now := time.Now().UTC()
	q := `
		INSERT INTO sqlserver_performance_debt_findings (
			capture_timestamp, server_id, database_name, section, finding_type,
			severity, title, object_name, object_type, finding_key,
			impact_score, details, recommendation, fix_script
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	batch := &pgx.Batch{}
	for _, f := range toInsert {
		batch.Queue(q,
			now, serverID, f.DatabaseName, f.Section, f.FindingType,
			f.Severity, f.Title, f.ObjectName, f.ObjectType, f.FindingKey,
			f.ImpactScore, normalizePerformanceDebtDetails(f.Details), f.Recommendation, f.FixScript,
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range toInsert {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetPerformanceDebtFindings(ctx context.Context, serverID uuid.UUID, lookback time.Duration, database string) ([]map[string]interface{}, error) {
	secs := int(lookback.Seconds())
	if secs <= 0 {
		secs = 86400 * 7 // 7 days
	}

	var rows pgx.Rows
	var err error

	if database == "" {
		q := `
			SELECT DISTINCT ON (finding_key)
			       capture_timestamp, database_name, section, finding_type, severity, title,
			       object_name, finding_key, impact_score, details, recommendation, fix_script
			FROM sqlserver_performance_debt_findings
			WHERE server_id = $1
			  AND capture_timestamp >= NOW() - ($2 * interval '1 second')
			ORDER BY finding_key, capture_timestamp DESC
		`
		rows, err = tl.pool.Query(ctx, q, serverID, secs)
	} else {
		q := `
			SELECT DISTINCT ON (finding_key)
			       capture_timestamp, database_name, section, finding_type, severity, title,
			       object_name, finding_key, impact_score, details, recommendation, fix_script
			FROM sqlserver_performance_debt_findings
			WHERE server_id = $1
			  AND capture_timestamp >= NOW() - ($2 * interval '1 second')
			  AND database_name = $3
			ORDER BY finding_key, capture_timestamp DESC
		`
		rows, err = tl.pool.Query(ctx, q, serverID, secs, database)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var db, section, fType, severity, title, obj, key, det, rec, fix string
		var impact float64
		if err := rows.Scan(&ts, &db, &section, &fType, &severity, &title, &obj, &key, &impact, &det, &rec, &fix); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"capture_timestamp": ts,
			"database_name":     db,
			"section":           section,
			"finding_type":      fType,
			"severity":          severity,
			"title":             title,
			"object_name":       obj,
			"finding_key":       key,
			"impact_score":      impact,
			"details":           det,
			"recommendation":    rec,
			"fix_script":        fix,
		})
	}
	return results, rows.Err()
}
