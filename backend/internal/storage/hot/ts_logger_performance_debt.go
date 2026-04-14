// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Performance debt tracking logger.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type PerformanceDebtFindingRow struct {
	CaptureTimestamp   time.Time       `json:"capture_timestamp"`
	ServerInstanceName string          `json:"server_instance_name"`
	DatabaseName       string          `json:"database_name"`
	Section            string          `json:"section"`
	FindingType        string          `json:"finding_type"`
	Severity           string          `json:"severity"`
	Title              string          `json:"title"`
	ObjectName         string          `json:"object_name"`
	ObjectType         string          `json:"object_type"`
	FindingKey         string          `json:"finding_key"`
	Details            json.RawMessage `json:"details"`
	Recommendation     string          `json:"recommendation"`
	FixScript          string          `json:"fix_script"`
}

// PerformanceDebtFingerprintHash returns a stable hash for a finding row.
// It is used to avoid re-inserting identical findings every collection cycle.
func PerformanceDebtFingerprintHash(r PerformanceDebtFindingRow) string {
	var b strings.Builder
	b.Grow(512)
	b.WriteString(r.DatabaseName)
	b.WriteString("|")
	b.WriteString(r.Section)
	b.WriteString("|")
	b.WriteString(r.FindingType)
	b.WriteString("|")
	b.WriteString(r.Severity)
	b.WriteString("|")
	b.WriteString(r.Title)
	b.WriteString("|")
	b.WriteString(r.ObjectName)
	b.WriteString("|")
	b.WriteString(r.ObjectType)
	b.WriteString("|")
	b.WriteString(string(r.Details))
	b.WriteString("|")
	b.WriteString(r.Recommendation)
	b.WriteString("|")
	b.WriteString(r.FixScript)
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// GetLatestPerformanceDebtFingerprintHashesByKey returns a map of latest finding hashes by finding_key
// in the given lookback window. Use this to avoid inserting identical snapshots repeatedly.
func (tl *TimescaleLogger) GetLatestPerformanceDebtFingerprintHashesByKey(ctx context.Context, instanceName string, lookback time.Duration) (map[string]string, error) {
	if lookback <= 0 {
		lookback = 2 * time.Hour
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	q := `
		WITH ranked AS (
			SELECT
				finding_key,
				database_name,
				section,
				finding_type,
				severity,
				title,
				object_name,
				object_type,
				details::text AS details_json,
				recommendation,
				fix_script,
				ROW_NUMBER() OVER (PARTITION BY finding_key ORDER BY capture_timestamp DESC) AS rn
			FROM sqlserver_performance_debt_findings
			WHERE server_instance_name = $1
			  AND capture_timestamp >= NOW() - ($2::bigint * INTERVAL '1 second')
		)
		SELECT finding_key, database_name, section, finding_type, severity, title, object_name, object_type, details_json, recommendation, fix_script
		FROM ranked
		WHERE rn = 1
	`
	secs := int64(lookback.Seconds())
	rows, err := tl.pool.Query(ctx, q, instanceName, secs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var k string
		var db, sec, ft, sev, title, objName, objType, detailsJSON, rec, fix string
		if err := rows.Scan(&k, &db, &sec, &ft, &sev, &title, &objName, &objType, &detailsJSON, &rec, &fix); err != nil {
			continue
		}
		out[k] = PerformanceDebtFingerprintHash(PerformanceDebtFindingRow{
			DatabaseName:   db,
			Section:        sec,
			FindingType:    ft,
			Severity:       sev,
			Title:          title,
			ObjectName:     objName,
			ObjectType:     objType,
			Details:        json.RawMessage([]byte(detailsJSON)),
			Recommendation: rec,
			FixScript:      fix,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogPerformanceDebtFindings(ctx context.Context, rows []PerformanceDebtFindingRow) error {
	if len(rows) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_performance_debt_findings (
				capture_timestamp, server_instance_name, database_name,
				section, finding_type, severity, title,
				object_name, object_type, finding_key,
				details, recommendation, fix_script
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13)
		`, r.CaptureTimestamp, r.ServerInstanceName, r.DatabaseName,
			r.Section, r.FindingType, r.Severity, r.Title,
			r.ObjectName, r.ObjectType, r.FindingKey,
			string(r.Details), r.Recommendation, r.FixScript,
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("performance debt insert failed: %w", err)
		}
	}
	return nil
}

// CleanupPerformanceDebtFindings enforces retention:
// - Keep at most keepPerKey rows per (instance,finding_key)
// - Delete anything older than maxAge
//
// This runs as a simple SQL DELETE and does not require Timescale retention policies.
func (tl *TimescaleLogger) CleanupPerformanceDebtFindings(ctx context.Context, instanceName string, keepPerKey int, maxAge time.Duration) error {
	if keepPerKey <= 0 {
		keepPerKey = 10
	}
	if maxAge <= 0 {
		maxAge = 90 * 24 * time.Hour
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	secs := int64(maxAge.Seconds())
	_, err := tl.pool.Exec(ctx, `
		WITH ranked AS (
			SELECT
				capture_timestamp,
				finding_key,
				ROW_NUMBER() OVER (PARTITION BY finding_key ORDER BY capture_timestamp DESC) AS rn
			FROM sqlserver_performance_debt_findings
			WHERE server_instance_name = $1
		)
		DELETE FROM sqlserver_performance_debt_findings t
		USING ranked r
		WHERE t.server_instance_name = $1
		  AND t.finding_key = r.finding_key
		  AND t.capture_timestamp = r.capture_timestamp
		  AND (r.rn > $2 OR t.capture_timestamp < NOW() - ($3::bigint * INTERVAL '1 second'))
	`, instanceName, keepPerKey, secs)
	return err
}

// GetLatestPerformanceDebtFindings returns latest snapshot within lookback window.
func (tl *TimescaleLogger) GetLatestPerformanceDebtFindings(ctx context.Context, instanceName string, lookback time.Duration) ([]map[string]interface{}, error) {
	if lookback <= 0 {
		lookback = 2 * time.Hour
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Dedupe across captures: return only the latest row per finding_key within lookback.
	q := `
		SELECT DISTINCT ON (finding_key)
		       capture_timestamp, server_instance_name, database_name,
		       section, finding_type, severity, title,
		       object_name, object_type, finding_key,
		       details, recommendation, fix_script
		FROM sqlserver_performance_debt_findings
		WHERE server_instance_name = $1
		  AND capture_timestamp >= NOW() - ($2::bigint * INTERVAL '1 second')
		ORDER BY finding_key, capture_timestamp DESC
		LIMIT 2000
	`
	secs := int64(lookback.Seconds())
	rows, err := tl.pool.Query(ctx, q, instanceName, secs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var server, db, section, ft, sev, title, objName, objType, key, rec, fix string
		var details []byte
		if err := rows.Scan(&ts, &server, &db, &section, &ft, &sev, &title, &objName, &objType, &key, &details, &rec, &fix); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"capture_timestamp": ts,
			"server_instance_name": server,
			"database_name": db,
			"section": section,
			"finding_type": ft,
			"severity": sev,
			"title": title,
			"object_name": objName,
			"object_type": objType,
			"finding_key": key,
			"details": json.RawMessage(details),
			"recommendation": rec,
			"fix_script": fix,
		})
	}
	return out, rows.Err()
}

