package repository

import (
	"fmt"
	"log"
	"time"
)

type PgTableMaintenanceStat struct {
	CaptureTimestamp time.Time  `json:"capture_timestamp"`
	SchemaName       string     `json:"schema"`
	TableName        string     `json:"table"`
	TotalBytes       int64      `json:"total_bytes"`
	TotalSizePretty  string     `json:"total_size"`
	LiveTuples       int64      `json:"live_tuples"`
	DeadTuples       int64      `json:"dead_tuples"`
	DeadPct          float64    `json:"dead_pct"`
	SeqScans         int64      `json:"seq_scans"`
	IdxScans         int64      `json:"idx_scans"`
	LastVacuum       *time.Time `json:"last_vacuum,omitempty"`
	LastAutovacuum   *time.Time `json:"last_autovacuum,omitempty"`
	LastAnalyze      *time.Time `json:"last_analyze,omitempty"`
	LastAutoanalyze  *time.Time `json:"last_autoanalyze,omitempty"`
}

func deadPct(live, dead int64) float64 {
	den := live + dead
	if den <= 0 || dead <= 0 {
		return 0
	}
	p := (float64(dead) / float64(den)) * 100.0
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func (c *PgRepository) GetTableMaintenanceStats(instanceName string, limit int) ([]PgTableMaintenanceStat, error) {
	if limit <= 0 {
		limit = 200
	}
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// Table-level stats. Use pg_total_relation_size for bytes and pretty formatting.
	q := `
		SELECT
			now() AT TIME ZONE 'UTC' AS capture_timestamp,
			s.schemaname,
			s.relname,
			pg_total_relation_size(quote_ident(s.schemaname)||'.'||quote_ident(s.relname)) AS total_bytes,
			pg_size_pretty(pg_total_relation_size(quote_ident(s.schemaname)||'.'||quote_ident(s.relname))) AS total_pretty,
			COALESCE(s.n_live_tup,0) AS live_tuples,
			COALESCE(s.n_dead_tup,0) AS dead_tuples,
			COALESCE(s.seq_scan,0) AS seq_scans,
			COALESCE(s.idx_scan,0) AS idx_scans,
			s.last_vacuum,
			s.last_autovacuum,
			s.last_analyze,
			s.last_autoanalyze
		FROM pg_stat_user_tables s
		WHERE s.schemaname NOT IN ('pg_catalog','information_schema')
		ORDER BY pg_total_relation_size(quote_ident(s.schemaname)||'.'||quote_ident(s.relname)) DESC
		LIMIT $1
	`

	rows, err := db.Query(q, limit)
	if err != nil {
		log.Printf("[POSTGRES] GetTableMaintenanceStats query error for %s: %v", instanceName, err)
		return nil, err
	}
	defer rows.Close()

	var out []PgTableMaintenanceStat
	for rows.Next() {
		var r PgTableMaintenanceStat
		var lastVac, lastAutoVac, lastAna, lastAutoAna *time.Time
		if err := rows.Scan(
			&r.CaptureTimestamp,
			&r.SchemaName,
			&r.TableName,
			&r.TotalBytes,
			&r.TotalSizePretty,
			&r.LiveTuples,
			&r.DeadTuples,
			&r.SeqScans,
			&r.IdxScans,
			&lastVac,
			&lastAutoVac,
			&lastAna,
			&lastAutoAna,
		); err != nil {
			continue
		}
		r.DeadPct = deadPct(r.LiveTuples, r.DeadTuples)
		r.LastVacuum = lastVac
		r.LastAutovacuum = lastAutoVac
		r.LastAnalyze = lastAna
		r.LastAutoanalyze = lastAutoAna
		out = append(out, r)
	}
	return out, rows.Err()
}

