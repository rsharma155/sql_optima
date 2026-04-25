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
	"context"
	"database/sql"
	"fmt"
	"log"
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

	baseSelect := fmt.Sprintf(`
		SELECT %s AS dimension_value,
		       MAX(statement_text) as query_text,
		       MAX(database_name) AS database_name,
		       SUM(%s) AS metric_value,
		       SUM(total_executions) AS total_executions,
		       AVG(total_cpu_ms / NULLIF(total_executions, 0)) AS avg_cpu_ms,
		       AVG(total_elapsed_ms / NULLIF(total_executions, 0)) AS avg_duration_ms,
		       AVG(total_logical_reads / NULLIF(total_executions, 0)) AS avg_reads
		FROM sqlserver_query_metrics_v2
		WHERE UPPER(instance_id) = UPPER($1)`,
		dimensionCol, metricCol)

	var rows pgx.Rows
	var err error
	if strings.TrimSpace(params.From) != "" && strings.TrimSpace(params.To) != "" {
		start, end, errParse := parseTimeRangeRFC3339(params.From, params.To)
		if errParse != nil {
			return nil, errParse
		}
		q := baseSelect + `
		  AND ts >= $2
		  AND ts <= $3
		GROUP BY ` + dimensionCol + `
		ORDER BY metric_value DESC
		LIMIT $4`
		rows, err = tl.pool.Query(ctx, q, params.InstanceName, start, end, params.Limit)
	} else {
		q := baseSelect + fmt.Sprintf(`
		  AND ts > now() - INTERVAL '%s'
		GROUP BY %s
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
		var dimValue, queryText, dbName sql.NullString
		var metricValue, totalExecutions float64
		var avgCPU, avgDuration, avgReads sql.NullFloat64

		if err := rows.Scan(&dimValue, &queryText, &dbName, &metricValue, &totalExecutions, &avgCPU, &avgDuration, &avgReads); err != nil {
			log.Printf("[TSLogger] GetQueryStatsDashboard scan error: %v", err)
			continue
		}

		results = append(results, map[string]interface{}{
			"dimension":        dimValue.String,
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

func (tl *TimescaleLogger) GetQueryStatsTimeSeries(ctx context.Context, instanceName, metric string, timeRange string) ([]map[string]interface{}, error) {
	tr := map[string]string{
		"15m": "15 minutes",
		"1h":  "1 hour",
		"24h": "24 hours",
	}[timeRange]
	if tr == "" {
		tr = "1 hour"
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

	query := fmt.Sprintf(`
		SELECT time_bucket('5 min', ts) AS time,
		       SUM(%s) AS value
		FROM sqlserver_query_metrics_v2
		WHERE UPPER(instance_id) = UPPER($1)
		  AND ts > now() - INTERVAL '%s'
		GROUP BY time
		ORDER BY time
	`, metricCol, tr)

	rows, err := tl.pool.Query(ctx, query, instanceName)
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
