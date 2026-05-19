// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB storage-layer methods for the Query Analysis dashboard.
//          Provides aggregation and time-series data from the query metrics hypertables.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// QueryStatsDashboardParams drives leaderboard queries over sqlserver_query_stats_interval.
type QueryStatsDashboardParams struct {
	InstanceName string
	Metric       string // cpu, duration, reads, executions
	TimeRange    string // 15m, 1h, 24h (used when From/To are empty)
	Dimension    string // query, database, login, app
	Limit        int
	From         string // RFC3339 inclusive lower bound for bucket_end (optional)
	To           string // RFC3339 inclusive upper bound for bucket_end (optional)
}

func (tl *TimescaleLogger) GetQueryStatsDashboard(ctx context.Context, params QueryStatsDashboardParams) ([]map[string]interface{}, error) {
	if params.Limit <= 0 {
		params.Limit = 20
	}

	tr := map[string]string{
		"15m": "15 minutes",
		"1h":  "1 hour",
		"24h": "24 hours",
	}[params.TimeRange]
	if tr == "" {
		tr = "1 hour"
	}

	dimensionCol := map[string]string{
		"query":    "query_hash",
		"database": "database_name",
		"login":    "login_name",
		"app":      "client_app",
	}[params.Dimension]
	if dimensionCol == "" {
		dimensionCol = "query_hash"
	}

	metricCol := map[string]string{
		"cpu":        "total_cpu_ms",
		"duration":   "total_elapsed_ms",
		"reads":      "total_logical_reads",
		"executions": "total_executions",
	}[params.Metric]
	if metricCol == "" {
		metricCol = "total_cpu_ms"
	}

	dimExpr := "q." + dimensionCol

	baseSelect := fmt.Sprintf(`
		SELECT %s AS dimension_value,
		       MAX(q.statement_text) as query_text,
		       MAX(q.database_name) AS database_name,
		       SUM(q.%s)::float8 AS metric_value,
		       SUM(q.total_executions)::float8 AS total_executions,
		       AVG(q.total_cpu_ms::float8 / NULLIF(q.total_executions, 0)) AS avg_cpu_ms,
		       AVG(q.total_elapsed_ms::float8 / NULLIF(q.total_executions, 0)) AS avg_duration_ms,
		       AVG(q.total_logical_reads::float8 / NULLIF(q.total_executions, 0)) AS avg_reads
		FROM sqlserver_query_metrics_v2 q
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.server_id = q.server_id
		 AND class.query_hash = q.query_hash
		WHERE q.server_id = $1::uuid
		  AND COALESCE(q.is_user_workload, 1) = 1
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'`,
		dimExpr, metricCol)

	var rows pgx.Rows
	var err error
	if strings.TrimSpace(params.From) != "" && strings.TrimSpace(params.To) != "" {
		start, end, errParse := parseTimeRangeRFC3339(params.From, params.To)
		if errParse != nil {
			return nil, errParse
		}
		q := baseSelect + `
		  AND q.capture_timestamp >= $2
		  AND q.capture_timestamp <= $3
		GROUP BY q.` + dimensionCol + `
		ORDER BY metric_value DESC
		LIMIT $4`
		rows, err = tl.pool.Query(ctx, q, params.InstanceName, start, end, params.Limit)
	} else {
		q := baseSelect + fmt.Sprintf(`
		  AND q.capture_timestamp > now() - INTERVAL '%s'
		GROUP BY q.%s
		ORDER BY metric_value DESC
		LIMIT $2`, tr, dimensionCol)
		rows, err = tl.pool.Query(ctx, q, params.InstanceName, params.Limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var dimVal interface{}
		var queryText, dbName sql.NullString
		var metricValue, totalExecutions float64
		var avgCPU, avgDuration, avgReads sql.NullFloat64

		if err := rows.Scan(&dimVal, &queryText, &dbName, &metricValue, &totalExecutions, &avgCPU, &avgDuration, &avgReads); err != nil {
			slog.Error("[TSLogger] GetQueryStatsDashboard scan error", "err", err)
			continue
		}

		finalDim := ""
		if dimensionCol == "query_hash" {
			if h, ok := dimVal.(int64); ok {
				finalDim = fmt.Sprintf("0x%X", uint64(h))
			}
		} else {
			if s, ok := dimVal.(string); ok {
				finalDim = s
			}
		}

		results = append(results, map[string]interface{}{
			"dimension":        finalDim,
			"query_text":       queryText.String,
			"database_name":    dbName.String,
			"metric_value":     metricValue,
			"total_executions": totalExecutions,
			"avg_cpu_ms":       avgCPU.Float64,
			"avg_duration_ms":  avgDuration.Float64,
			"avg_reads":        avgReads.Float64,
		})
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetQueryStatsTimeSeries(ctx context.Context, serverID, metric string, from, to string, dbName string) ([]map[string]interface{}, error) {
	// Parse dates to determine range for dynamic bucketing
	var bucket = "5 min"
	tFrom, errF := time.Parse(time.RFC3339, from)
	tTo, errT := time.Parse(time.RFC3339, to)
	if errF == nil && errT == nil {
		dur := tTo.Sub(tFrom)
		if dur > 48*time.Hour {
			bucket = "1 hour"
		} else if dur > 12*time.Hour {
			bucket = "15 min"
		}
	}

	metricCol := map[string]string{
		"cpu":        "total_cpu_ms",
		"duration":   "total_elapsed_ms",
		"reads":      "total_logical_reads",
		"executions": "total_executions",
	}[metric]
	if metricCol == "" {
		metricCol = "total_cpu_ms"
	}

	dbFilter := ""
	if dbName != "" && dbName != "all" {
		dbFilter = fmt.Sprintf("AND q.database_name = '%s'", strings.ReplaceAll(dbName, "'", "''"))
	}

	query := fmt.Sprintf(`
		SELECT time_bucket('%s', q.capture_timestamp) AS time,
		       SUM(q.%s)::float8 AS value
		FROM sqlserver_query_metrics_v2 q
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.server_id = q.server_id
		 AND class.query_hash = q.query_hash
		WHERE q.server_id = $1::uuid
		  AND q.capture_timestamp >= $2 AND q.capture_timestamp <= $3
		  AND q.query_text_raw NOT LIKE '%%%%/* SQL_OPTIMA%%%%'
		  AND q.statement_text NOT LIKE '%%%%sys.all_objects%%%%'
		  AND q.statement_text NOT LIKE '%%%%[sys].all_objects%%%%'
		  AND q.statement_text NOT LIKE '%%%%sp_MShistory_cleanup%%%%'
		  AND UPPER(q.statement_text) NOT LIKE '%%%%SYS.%%%%'
		  AND UPPER(q.statement_text) NOT LIKE '%%%%[SYS].%%%%'
		  AND COALESCE(q.is_user_workload, 1) = 1
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
		  %s
		GROUP BY time
		ORDER BY time
	`, bucket, metricCol, dbFilter)

	rows, err := tl.pool.Query(ctx, query, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var value float64

		if err := rows.Scan(&ts, &value); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"time":  ts,
			"value": value,
		})
	}
	return results, rows.Err()
}
