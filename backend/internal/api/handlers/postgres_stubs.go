// SQL Optima — remaining Postgres handler methods (see pg_stub_impl.go for wired routes).
package handlers

import (
	"encoding/json"
	"net/http"
	"time"
)

func (h *PostgresHandlers) TimeseriesMetrics(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	metric := r.URL.Query().Get("metric")
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	type point struct {
		Time  time.Time `json:"time"`
		Value float64   `json:"value"`
	}
	writePoints := func(data []point) {
		if data == nil {
			data = []point{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
	}

	tl := h.metricsSvc.GetTimescaleDBLogger()
	if tl == nil {
		writePoints(nil)
		return
	}

	switch metric {
	case "cpu_usage_pct":
		rows, err := tl.GetHostCPUUsagePctSeries(r.Context(), sid, from, to, 300)
		if err != nil {
			writePoints(nil)
			return
		}
		data := make([]point, 0, len(rows))
		for _, row := range rows {
			data = append(data, point{Time: row.Time, Value: row.Value})
		}
		writePoints(data)
		return
	case "memory_usage_pct":
		rows, err := tl.GetHostMemoryUsagePctSeries(r.Context(), sid, from, to, 300)
		if err != nil {
			writePoints(nil)
			return
		}
		data := make([]point, 0, len(rows))
		for _, row := range rows {
			data = append(data, point{Time: row.Time, Value: row.Value})
		}
		writePoints(data)
		return
	}

	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		writePoints(nil)
		return
	}

	colMap := map[string]string{
		"cpu_load":          "GREATEST(active_sessions - waiting_sessions - idle_in_txn_sessions, 0)",
		"io_wait_load":      "waiting_sessions",
		"lock_wait_load":    "blocking_sessions",
		"idle_in_txn_load":  "idle_in_txn_sessions",
		"tps":               "tps",
		"connections_usage": "connections_usage_pct",
		"cache_hit":         "cache_hit_ratio_pct",
	}

	col, ok := colMap[metric]
	if !ok {
		writePoints(nil)
		return
	}

	q := `
		SELECT capture_timestamp AS time, ` + col + ` AS value
		FROM postgres_control_center_stats
		WHERE server_id = $1
		  AND capture_timestamp BETWEEN $2 AND $3
		ORDER BY capture_timestamp
		LIMIT 300`

	rows, err := pool.Query(r.Context(), q, sid, from, to)
	if err != nil {
		writePoints(nil)
		return
	}
	defer rows.Close()

	var data []point
	for rows.Next() {
		var t time.Time
		var v float64
		if err := rows.Scan(&t, &v); err != nil {
			continue
		}
		data = append(data, point{Time: t, Value: v})
	}
	writePoints(data)
}

func (h *PostgresHandlers) IdleInTransaction(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	res, err := h.metricsSvc.GetPostgresIdleInTransactionSessions(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
