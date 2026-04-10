package hot

import (
	"context"
	"hash/fnv"
	"sort"
	"time"
)

type PostgresWaitEventRow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerInstanceName string    `json:"server_instance_name"`
	WaitEventType      string    `json:"wait_event_type"`
	WaitEvent          string    `json:"wait_event"`
	SessionsCount      int       `json:"sessions_count"`
}

type PostgresDbIORow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerInstanceName string    `json:"server_instance_name"`
	DatabaseName       string    `json:"database_name"`
	BlksRead           int64     `json:"blks_read"`
	BlksHit            int64     `json:"blks_hit"`
	TempFiles          int64     `json:"temp_files"`
	TempBytes          int64     `json:"temp_bytes"`
}

type PostgresSettingSnapshotRow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerInstanceName string    `json:"server_instance_name"`
	Name               string    `json:"name"`
	Setting            string    `json:"setting"`
	Unit               string    `json:"unit"`
	Source             string    `json:"source"`
}

func hashRows(parts []string) uint64 {
	h := fnv.New64a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum64()
}

func (tl *TimescaleLogger) LogPostgresWaitEvents(ctx context.Context, instanceName string, rows []PostgresWaitEventRow) error {
	// Stable signature for dedup.
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.WaitEventType+"|"+r.WaitEvent+"|"+itoa(r.SessionsCount))
	}
	sort.Strings(keys)
	sig := hashRows(keys)

	tl.mu.Lock()
	if prev := tl.prevPgWaitEventsHash[instanceName]; prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgWaitEventsHash[instanceName] = sig
	tl.mu.Unlock()

	q := `INSERT INTO postgres_wait_event_stats (
		capture_timestamp, server_instance_name, wait_event_type, wait_event, sessions_count
	) VALUES ($1,$2,$3,$4,$5)`
	for _, r := range rows {
		_, err := tl.pool.Exec(ctx, q, r.CaptureTimestamp, r.ServerInstanceName, r.WaitEventType, r.WaitEvent, r.SessionsCount)
		if err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetPostgresWaitEventsHistory(ctx context.Context, instanceName string, limit int) ([]PostgresWaitEventRow, error) {
	if limit <= 0 {
		limit = 180
	}
	q := `
		SELECT capture_timestamp, server_instance_name, COALESCE(wait_event_type,''), COALESCE(wait_event,''), sessions_count
		FROM postgres_wait_event_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PostgresWaitEventRow
	for rows.Next() {
		var r PostgresWaitEventRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerInstanceName, &r.WaitEventType, &r.WaitEvent, &r.SessionsCount); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetPostgresLockWaitHistory returns a time series of session counts waiting with wait_event_type = 'Lock'
// (aggregated across specific lock wait_event values per collector snapshot).
func (tl *TimescaleLogger) GetPostgresLockWaitHistory(ctx context.Context, instanceName string, windowMinutes, maxPoints int) ([]string, []int, error) {
	if maxPoints <= 0 {
		maxPoints = 500
	}
	if windowMinutes <= 0 {
		windowMinutes = 180
	}
	from := time.Now().UTC().Add(-time.Duration(windowMinutes) * time.Minute)
	q := `
		SELECT capture_timestamp, COALESCE(SUM(sessions_count), 0)::int
		FROM postgres_wait_event_stats
		WHERE server_instance_name = $1
		  AND wait_event_type = 'Lock'
		  AND capture_timestamp >= $2
		GROUP BY capture_timestamp
		ORDER BY capture_timestamp ASC
		LIMIT $3
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, from, maxPoints)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var labels []string
	var counts []int
	for rows.Next() {
		var ts time.Time
		var n int
		if err := rows.Scan(&ts, &n); err != nil {
			continue
		}
		labels = append(labels, ts.UTC().Format(time.RFC3339))
		counts = append(counts, n)
	}
	return labels, counts, rows.Err()
}

func (tl *TimescaleLogger) LogPostgresDbIOStats(ctx context.Context, instanceName string, rows []PostgresDbIORow) error {
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.DatabaseName+"|"+itoa64(r.BlksRead)+"|"+itoa64(r.BlksHit)+"|"+itoa64(r.TempFiles)+"|"+itoa64(r.TempBytes))
	}
	sort.Strings(keys)
	sig := hashRows(keys)

	tl.mu.Lock()
	if prev := tl.prevPgDbIOHash[instanceName]; prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgDbIOHash[instanceName] = sig
	tl.mu.Unlock()

	q := `INSERT INTO postgres_db_io_stats (
		capture_timestamp, server_instance_name, database_name, blks_read, blks_hit, temp_files, temp_bytes
	) VALUES ($1,$2,$3,$4,$5,$6,$7)`
	for _, r := range rows {
		_, err := tl.pool.Exec(ctx, q, r.CaptureTimestamp, r.ServerInstanceName, r.DatabaseName, r.BlksRead, r.BlksHit, r.TempFiles, r.TempBytes)
		if err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetPostgresDbIOHistory(ctx context.Context, instanceName string, limit int) ([]PostgresDbIORow, error) {
	if limit <= 0 {
		limit = 500
	}
	q := `
		SELECT capture_timestamp, server_instance_name, database_name, blks_read, blks_hit, temp_files, temp_bytes
		FROM postgres_db_io_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PostgresDbIORow
	for rows.Next() {
		var r PostgresDbIORow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerInstanceName, &r.DatabaseName, &r.BlksRead, &r.BlksHit, &r.TempFiles, &r.TempBytes); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogPostgresSettingsSnapshot(ctx context.Context, instanceName string, rows []PostgresSettingSnapshotRow) error {
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.Name+"|"+r.Setting+"|"+r.Unit+"|"+r.Source)
	}
	sort.Strings(keys)
	sig := hashRows(keys)

	tl.mu.Lock()
	if prev := tl.prevPgSettingsHash[instanceName]; prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgSettingsHash[instanceName] = sig
	tl.mu.Unlock()

	q := `INSERT INTO postgres_settings_snapshot (
		capture_timestamp, server_instance_name, name, setting, unit, source
	) VALUES ($1,$2,$3,$4,$5,$6)`
	for _, r := range rows {
		_, err := tl.pool.Exec(ctx, q, r.CaptureTimestamp, r.ServerInstanceName, r.Name, r.Setting, r.Unit, r.Source)
		if err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetPostgresSettingsSnapshotLatestTwo(ctx context.Context, instanceName string) (latestTs time.Time, prevTs time.Time, latest []PostgresSettingSnapshotRow, prev []PostgresSettingSnapshotRow, err error) {
	// Find two most recent distinct capture timestamps.
	tsRows, err := tl.pool.Query(ctx, `
		SELECT DISTINCT capture_timestamp
		FROM postgres_settings_snapshot
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT 2
	`, instanceName)
	if err != nil {
		return time.Time{}, time.Time{}, nil, nil, err
	}
	defer tsRows.Close()
	var tss []time.Time
	for tsRows.Next() {
		var t time.Time
		if err := tsRows.Scan(&t); err == nil {
			tss = append(tss, t)
		}
	}
	if len(tss) == 0 {
		return time.Time{}, time.Time{}, []PostgresSettingSnapshotRow{}, []PostgresSettingSnapshotRow{}, nil
	}
	latestTs = tss[0]
	if len(tss) > 1 {
		prevTs = tss[1]
	}

	load := func(ts time.Time) ([]PostgresSettingSnapshotRow, error) {
		if ts.IsZero() {
			return []PostgresSettingSnapshotRow{}, nil
		}
		rows, err := tl.pool.Query(ctx, `
			SELECT capture_timestamp, server_instance_name, name, COALESCE(setting,''), COALESCE(unit,''), COALESCE(source,'')
			FROM postgres_settings_snapshot
			WHERE server_instance_name = $1 AND capture_timestamp = $2
		`, instanceName, ts)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []PostgresSettingSnapshotRow
		for rows.Next() {
			var r PostgresSettingSnapshotRow
			if err := rows.Scan(&r.CaptureTimestamp, &r.ServerInstanceName, &r.Name, &r.Setting, &r.Unit, &r.Source); err == nil {
				out = append(out, r)
			}
		}
		return out, rows.Err()
	}
	latest, err = load(latestTs)
	if err != nil {
		return
	}
	prev, err = load(prevTs)
	return
}

// Small helpers (avoid fmt in hot path).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [32]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [64]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

