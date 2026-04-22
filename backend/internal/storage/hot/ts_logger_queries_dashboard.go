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
		"cpu":        "delta_cpu_ms",
		"duration":   "delta_duration_ms",
		"reads":      "delta_logical_reads",
		"executions": "delta_executions",
	}[params.Metric]
	if metricCol == "" {
		metricCol = "delta_cpu_ms"
	}

	baseSelect := fmt.Sprintf(`
		SELECT %s AS dimension_value,
		       MAX(query_text) as query_text,
		       MAX(database_name) AS database_name,
		       SUM(%s) AS metric_value,
		       SUM(delta_executions) AS total_executions,
		       AVG(avg_cpu_ms) AS avg_cpu_ms,
		       AVG(avg_duration_ms) AS avg_duration_ms,
		       AVG(avg_reads) AS avg_reads
		FROM monitor.sqlserver_query_store_interval
		WHERE UPPER(server_instance_name) = UPPER($1)`,
		dimensionCol, metricCol)

	var rows pgx.Rows
	var err error
	if strings.TrimSpace(params.From) != "" && strings.TrimSpace(params.To) != "" {
		start, end, errParse := parseTimeRangeRFC3339(params.From, params.To)
		if errParse != nil {
			return nil, errParse
		}
		q := baseSelect + `
		  AND bucket_end >= $2
		  AND bucket_end <= $3
		GROUP BY ` + dimensionCol + `
		ORDER BY metric_value DESC
		LIMIT $4`
		rows, err = tl.pool.Query(ctx, q, params.InstanceName, start, end, params.Limit)
	} else {
		q := baseSelect + fmt.Sprintf(`
		  AND bucket_end > now() - INTERVAL '%s'
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
		var metricValue, totalExecutions, avgCPU, avgDuration, avgReads float64

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
			"avg_cpu_ms":       avgCPU,
			"avg_duration_ms":  avgDuration,
			"avg_reads":        avgReads,
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
		"cpu":        "delta_cpu_ms",
		"duration":   "delta_duration_ms",
		"reads":      "delta_logical_reads",
		"executions": "delta_executions",
	}[metric]
	if metricCol == "" {
		metricCol = "delta_cpu_ms"
	}

	query := fmt.Sprintf(`
		SELECT time_bucket('5 min', bucket_end) AS time,
		       SUM(%s) AS value
		FROM monitor.sqlserver_query_store_interval
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND bucket_end > now() - INTERVAL '%s'
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
