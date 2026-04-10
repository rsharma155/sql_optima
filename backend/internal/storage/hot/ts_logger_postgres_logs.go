package hot

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type PostgresLogEventRow struct {
	CaptureTimestamp  time.Time              `json:"capture_timestamp"`
	ServerInstanceName string                `json:"server_instance_name"`
	Severity          string                 `json:"severity"`
	SQLState          string                 `json:"sqlstate,omitempty"`
	Message           string                 `json:"message"`
	UserName          string                 `json:"user_name,omitempty"`
	DatabaseName      string                 `json:"database_name,omitempty"`
	ApplicationName   string                 `json:"application_name,omitempty"`
	ClientAddr        string                 `json:"client_addr,omitempty"`
	PID               int64                  `json:"pid,omitempty"`
	Context           string                 `json:"context,omitempty"`
	Detail            string                 `json:"detail,omitempty"`
	Hint              string                 `json:"hint,omitempty"`
	Raw               map[string]interface{} `json:"raw,omitempty"`
}

type PostgresLogSummary struct {
	WindowMinutes int    `json:"window_minutes"`
	ErrorCount    int    `json:"error_count"`
	FatalCount    int    `json:"fatal_count"`
	PanicCount    int    `json:"panic_count"`
	AuthFailCount int    `json:"auth_fail_count"`
	OOMCount      int    `json:"oom_count"`
	LastEventAt   string `json:"last_event_at,omitempty"`
	LastMessage   string `json:"last_message,omitempty"`
}

func normalizePgLogSeverity(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case "debug", "info", "notice", "warning", "error", "fatal", "panic":
		return v
	case "warn":
		return "warning"
	case "err":
		return "error"
	default:
		if v == "" {
			return "unknown"
		}
		return v
	}
}

func (tl *TimescaleLogger) LogPostgresLogEvents(ctx context.Context, instanceName string, rows []PostgresLogEventRow) error {
	if len(rows) == 0 {
		return nil
	}
	q := `
		INSERT INTO postgres_log_events (
			capture_timestamp, server_instance_name,
			severity, sqlstate, message,
			user_name, database_name, application_name, client_addr,
			pid, context, detail, hint, raw
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`
	b := &pgx.Batch{}
	for _, r := range rows {
		se := normalizePgLogSeverity(r.Severity)
		ts := r.CaptureTimestamp
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		b.Queue(q,
			ts, instanceName,
			se, r.SQLState, r.Message,
			r.UserName, r.DatabaseName, r.ApplicationName, r.ClientAddr,
			r.PID, r.Context, r.Detail, r.Hint, r.Raw,
		)
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetPostgresLogSummary(ctx context.Context, instanceName string, windowMinutes int) (*PostgresLogSummary, error) {
	if windowMinutes <= 0 {
		windowMinutes = 60
	}
	q := `
		WITH w AS (
			SELECT *
			FROM postgres_log_events
			WHERE server_instance_name = $1
			  AND capture_timestamp >= NOW() - ($2::text || ' minutes')::interval
		)
		SELECT
			COUNT(*) FILTER (WHERE severity = 'error') AS err_cnt,
			COUNT(*) FILTER (WHERE severity = 'fatal') AS fatal_cnt,
			COUNT(*) FILTER (WHERE severity = 'panic') AS panic_cnt,
			COUNT(*) FILTER (WHERE message ILIKE '%password authentication failed%' OR message ILIKE '%authentication failed%') AS auth_fail_cnt,
			COUNT(*) FILTER (WHERE message ILIKE '%out of memory%' OR message ILIKE '%cannot allocate memory%') AS oom_cnt,
			MAX(capture_timestamp) AS last_at,
			(SELECT message FROM w ORDER BY capture_timestamp DESC LIMIT 1) AS last_msg
		FROM w
	`
	var s PostgresLogSummary
	s.WindowMinutes = windowMinutes
	var lastAt *time.Time
	if err := tl.pool.QueryRow(ctx, q, instanceName, windowMinutes).Scan(
		&s.ErrorCount, &s.FatalCount, &s.PanicCount, &s.AuthFailCount, &s.OOMCount, &lastAt, &s.LastMessage,
	); err != nil {
		return nil, err
	}
	if lastAt != nil {
		s.LastEventAt = lastAt.UTC().Format(time.RFC3339)
	}
	return &s, nil
}

func (tl *TimescaleLogger) GetPostgresLogEvents(ctx context.Context, instanceName string, limit int, severity string) ([]PostgresLogEventRow, error) {
	if limit <= 0 {
		limit = 200
	}
	sev := strings.ToLower(strings.TrimSpace(severity))
	q := `
		SELECT capture_timestamp, server_instance_name, severity,
		       COALESCE(sqlstate,''), message,
		       COALESCE(user_name,''), COALESCE(database_name,''), COALESCE(application_name,''), COALESCE(client_addr,''),
		       COALESCE(pid,0), COALESCE(context,''), COALESCE(detail,''), COALESCE(hint,''), raw
		FROM postgres_log_events
		WHERE server_instance_name = $1
	`
	args := []any{instanceName}
	if sev != "" && sev != "all" {
		q += " AND severity = $2"
		args = append(args, sev)
		q += " ORDER BY capture_timestamp DESC LIMIT $3"
		args = append(args, limit)
	} else {
		q += " ORDER BY capture_timestamp DESC LIMIT $2"
		args = append(args, limit)
	}

	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresLogEventRow
	for rows.Next() {
		var r PostgresLogEventRow
		if err := rows.Scan(
			&r.CaptureTimestamp, &r.ServerInstanceName, &r.Severity,
			&r.SQLState, &r.Message,
			&r.UserName, &r.DatabaseName, &r.ApplicationName, &r.ClientAddr,
			&r.PID, &r.Context, &r.Detail, &r.Hint, &r.Raw,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

