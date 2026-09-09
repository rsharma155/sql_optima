// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Hot/cold time-range federation helpers for dashboard lookbacks beyond
//          Timescale retention (Phase: Trino/Iceberg adoption by handlers).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package federation

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TimeRange is a half-open or closed wall-clock window in UTC.
type TimeRange struct {
	From time.Time
	To   time.Time
}

var safeIdent = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// NeedsColdLookback reports whether from is older than hotRetentionDays before to.
// hotRetentionDays should match the table's Timescale retention (or purge floor).
func NeedsColdLookback(from, to time.Time, hotRetentionDays int) bool {
	if hotRetentionDays <= 0 || !from.Before(to) {
		return false
	}
	cutoff := to.UTC().AddDate(0, 0, -hotRetentionDays)
	return from.UTC().Before(cutoff)
}

// SplitRange splits [from,to] into cold (older) and hot (newer) segments at the
// hot retention boundary. ok is false when cold is not needed.
func SplitRange(from, to time.Time, hotRetentionDays int) (hot, cold TimeRange, ok bool) {
	from, to = from.UTC(), to.UTC()
	if !NeedsColdLookback(from, to, hotRetentionDays) {
		return TimeRange{From: from, To: to}, TimeRange{}, false
	}
	boundary := to.AddDate(0, 0, -hotRetentionDays)
	if !boundary.After(from) {
		// Entire window is cold-side of the boundary.
		return TimeRange{From: boundary, To: to}, TimeRange{From: from, To: boundary}, true
	}
	return TimeRange{From: boundary, To: to}, TimeRange{From: from, To: boundary}, true
}

// SanitizeIcebergTableName returns a safe SQL identifier for Iceberg/Trino, or "".
// Accepts optional schema.table and returns the leaf name only.
func SanitizeIcebergTableName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	if !safeIdent.MatchString(name) {
		return ""
	}
	return name
}

// AllowedHistoryTables is the allowlist for structured cold history queries.
// Keep in sync with cold registration TableName values.
var AllowedHistoryTables = map[string]struct{}{
	"sqlserver_cpu_history":        {},
	"sqlserver_wait_history":       {},
	"sqlserver_connection_history": {},
	"sqlserver_memory_history":     {},
	"pg_db_load_ts":                {},
	"postgres_wait_event_stats":    {},
}

// IsAllowedHistoryTable reports whether table may be queried via BuildIcebergHistorySQL.
func IsAllowedHistoryTable(table string) bool {
	safe := SanitizeIcebergTableName(table)
	if safe == "" {
		return false
	}
	_, ok := AllowedHistoryTables[safe]
	return ok
}

// BuildIcebergHistorySQL builds a read-only Trino SELECT for a cold history window.
// serverID must be a UUID string; from/to are rendered as UTC timestamps.
func BuildIcebergHistorySQL(table, serverID string, from, to time.Time) (string, error) {
	return BuildIcebergHistorySQLOnColumn(table, serverID, from, to, "capture_timestamp")
}

// BuildIcebergHistorySQLOnColumn is like BuildIcebergHistorySQL but allows an alternate
// timestamp column (e.g. capture_timestamp_ms for Parquet-exported CPU history).
func BuildIcebergHistorySQLOnColumn(table, serverID string, from, to time.Time, timeColumn string) (string, error) {
	safe := SanitizeIcebergTableName(table)
	if safe == "" || !IsAllowedHistoryTable(safe) {
		return "", errors.New("table is not allowlisted for cold history")
	}
	sid, err := uuid.Parse(strings.TrimSpace(serverID))
	if err != nil {
		return "", errors.New("invalid server_id")
	}
	from, to = from.UTC(), to.UTC()
	if !from.Before(to) {
		return "", errors.New("from must be before to")
	}
	tc := SanitizeIcebergTableName(timeColumn)
	if tc == "" {
		return "", errors.New("invalid time column")
	}
	if tc == "capture_timestamp_ms" {
		return fmt.Sprintf(
			`SELECT * FROM iceberg.default.%s WHERE server_id = '%s' AND %s >= %d AND %s < %d ORDER BY %s ASC LIMIT 10000`,
			safe, sid.String(), tc, from.UnixMilli(), tc, to.UnixMilli(), tc,
		), nil
	}
	return fmt.Sprintf(
		`SELECT * FROM iceberg.default.%s WHERE server_id = '%s' AND %s >= TIMESTAMP '%s' AND %s < TIMESTAMP '%s' ORDER BY %s ASC LIMIT 10000`,
		safe,
		sid.String(),
		tc,
		from.Format("2006-01-02 15:04:05.000"),
		tc,
		to.Format("2006-01-02 15:04:05.000"),
		tc,
	), nil
}

// SeriesPointTime extracts a comparable UTC time from a series map using common keys.
func SeriesPointTime(row map[string]interface{}, keys ...string) (time.Time, bool) {
	if len(keys) == 0 {
		keys = []string{"timestamp", "capture_timestamp", "bucket", "time"}
	}
	for _, k := range keys {
		v, ok := row[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case time.Time:
			return t.UTC(), true
		case string:
			if parsed, err := time.Parse(time.RFC3339, t); err == nil {
				return parsed.UTC(), true
			}
			if parsed, err := time.Parse("2006-01-02 15:04:05", t); err == nil {
				return parsed.UTC(), true
			}
		case float64:
			// epoch seconds or ms
			if t > 1e12 {
				return time.UnixMilli(int64(t)).UTC(), true
			}
			return time.Unix(int64(t), 0).UTC(), true
		case int64:
			if t > 1e12 {
				return time.UnixMilli(t).UTC(), true
			}
			return time.Unix(t, 0).UTC(), true
		}
	}
	return time.Time{}, false
}

// MergeTimeSeries concatenates cold then hot points ordered by time.
// When timestamps collide, the hot point wins (replaces cold).
func MergeTimeSeries(cold, hot []map[string]interface{}, timeKeys ...string) []map[string]interface{} {
	type keyed struct {
		t   time.Time
		row map[string]interface{}
	}
	byUnix := make(map[int64]keyed, len(cold)+len(hot))
	order := make([]int64, 0, len(cold)+len(hot))

	add := func(rows []map[string]interface{}, overwrite bool) {
		for _, row := range rows {
			t, ok := SeriesPointTime(row, timeKeys...)
			if !ok {
				continue
			}
			u := t.UnixNano()
			if _, exists := byUnix[u]; exists && !overwrite {
				continue
			}
			if _, exists := byUnix[u]; !exists {
				order = append(order, u)
			}
			byUnix[u] = keyed{t: t, row: row}
		}
	}
	add(cold, false)
	add(hot, true)

	// Stable chronological order
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	out := make([]map[string]interface{}, 0, len(order))
	for _, u := range order {
		out = append(out, byUnix[u].row)
	}
	return out
}
