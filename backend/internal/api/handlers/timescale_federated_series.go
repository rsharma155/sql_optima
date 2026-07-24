// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Shared hot+cold federation helpers for Timescale dashboard series.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/storage/cold/federation"
)

type coldRowMapper func(result *coldQueryResponse) []map[string]interface{}

func (h *TimescaleHandlers) serveFederatedSeries(
	w http.ResponseWriter,
	r *http.Request,
	table string,
	timeColumn string,
	hotFn func(ctx context.Context, id uuid.UUID, from, to string) ([]map[string]interface{}, error),
	mapCold coldRowMapper,
	publicErr string,
) {
	w.Header().Set("Content-Type", "application/json")
	cfg := (*config.Config)(nil)
	if h.metricsSvc != nil {
		cfg = h.metricsSvc.Config
	}
	id, ok := ParseServerID(r, cfg)
	if !ok || id == uuid.Nil {
		// Prefer registry-aware lookup when config miss (UI sends ?instance=<name>).
		if rid, _, outcome := resolveInstanceParamWithRepo(r.Context(), r, cfg, h.metricsSvc); outcome == lookupFound {
			id = rid
			ok = true
		}
	}
	if !ok || id == uuid.Nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	hot, err := hotFn(r.Context(), id, from, to)
	if err != nil {
		slog.Error("federated series hot read failed", "table", table, "server_id", id, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": publicErr})
		return
	}
	if hot == nil {
		hot = []map[string]interface{}{}
	}
	normalizeHistoryPointTimestamps(hot)

	fromT, toT, perr := parseTimeRange(from, to)
	source := "hot"
	if perr == nil && h.coldQuery != nil && federation.NeedsColdLookback(fromT, toT, coldHotRetentionDays()) {
		coldRows, cerr := h.coldQuery.fetchAllowlistedHistoryMaps(r.Context(), table, timeColumn, id, fromT, toT, mapCold)
		if cerr != nil {
			slog.Warn("federated series cold read failed; returning hot only", "table", table, "server_id", id, "err", cerr)
		} else if len(coldRows) > 0 {
			normalizeHistoryPointTimestamps(coldRows)
			hot = federation.MergeTimeSeries(coldRows, hot)
			source = "hot+cold"
		}
	}

	w.Header().Set("X-Data-Source", source)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"points":    hot,
		"source":    source,
		"row_count": len(hot),
	})
}

func normalizeHistoryPointTimestamps(rows []map[string]interface{}) {
	for _, row := range rows {
		if row == nil {
			continue
		}
		ts, ok := row["timestamp"]
		if !ok || ts == nil {
			if ct, ok2 := row["capture_timestamp"]; ok2 {
				row["timestamp"] = ct
				ts = ct
			}
		}
		if ts != nil {
			if _, has := row["capture_timestamp"]; !has {
				row["capture_timestamp"] = ts
			}
		}
		// Memory PLE alias used by dashboard charts
		if ple, ok := row["page_life_expectancy"]; ok {
			if _, has := row["ple_seconds"]; !has {
				row["ple_seconds"] = ple
			}
			if _, has := row["ple"]; !has {
				row["ple"] = ple
			}
		}
	}
}

func (h *ColdQueryHandlers) fetchAllowlistedHistoryMaps(
	ctx context.Context,
	table, timeColumn string,
	serverID uuid.UUID,
	from, to time.Time,
	mapCold coldRowMapper,
) ([]map[string]interface{}, error) {
	_, coldRange, ok := federation.SplitRange(from, to, coldHotRetentionDays())
	if !ok {
		return nil, nil
	}
	sqlText, err := federation.BuildIcebergHistorySQLOnColumn(table, serverID.String(), coldRange.From, coldRange.To, timeColumn)
	if err != nil {
		return nil, err
	}
	result, err := h.execTrinoQuery(ctx, sqlText)
	if err != nil {
		return nil, err
	}
	if mapCold == nil {
		return nil, nil
	}
	return mapCold(result), nil
}

func mapColdRowsGeneric(result *coldQueryResponse, build func(get func(...string) interface{}, ts time.Time) map[string]interface{}) []map[string]interface{} {
	if result == nil || len(result.Rows) == 0 {
		return nil
	}
	idx := map[string]int{}
	for i, c := range result.Columns {
		idx[strings.ToLower(c)] = i
	}
	out := make([]map[string]interface{}, 0, len(result.Rows))
	for _, row := range result.Rows {
		get := func(names ...string) interface{} {
			for _, n := range names {
				if i, ok := idx[strings.ToLower(n)]; ok && i < len(row) {
					return row[i]
				}
			}
			return nil
		}
		ts := coldCPUTimestamp(get("capture_timestamp", "capture_timestamp_ms", "timestamp"))
		if ts.IsZero() {
			continue
		}
		out = append(out, build(get, ts))
	}
	return out
}

func coldCPUTimestamp(v interface{}) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t.UTC()
	case string:
		if p, err := time.Parse(time.RFC3339, t); err == nil {
			return p.UTC()
		}
	case float64:
		if t > 1e12 {
			return time.UnixMilli(int64(t)).UTC()
		}
		return time.Unix(int64(t), 0).UTC()
	case int64:
		if t > 1e12 {
			return time.UnixMilli(t).UTC()
		}
		return time.Unix(t, 0).UTC()
	}
	return time.Time{}
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return f
		}
	}
	return 0
}

