package hot

import (
	"context"
	"fmt"
	"time"
)

type PostgresSessionStateCountRow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerInstanceName string    `json:"server_instance_name"`
	ActiveCount        int       `json:"active_count"`
	IdleCount          int       `json:"idle_count"`
	IdleInTxnCount     int       `json:"idle_in_txn_count"`
	WaitingCount       int       `json:"waiting_count"`
	TotalCount         int       `json:"total_count"`
}

func (tl *TimescaleLogger) LogPostgresSessionStateCounts(ctx context.Context, instanceName string, active, idle, idleInTxn, waiting, total int) error {
	sig := pgFnv64(instanceName, active, idle, idleInTxn, waiting, total)
	tl.mu.Lock()
	if tl.prevEnterpriseBatchHash == nil {
		tl.prevEnterpriseBatchHash = make(map[string]uint64)
	}
	key := "pg_sess_state|" + instanceName
	if prev, ok := tl.prevEnterpriseBatchHash[key]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevEnterpriseBatchHash[key] = sig
	tl.mu.Unlock()

	q := `
		INSERT INTO postgres_session_state_counts (
			capture_timestamp, server_instance_name,
			active_count, idle_count, idle_in_txn_count, waiting_count, total_count
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`
	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, q, now, instanceName, active, idle, idleInTxn, waiting, total)
	return err
}

func (tl *TimescaleLogger) GetPostgresSessionStateCountsHistory(ctx context.Context, instanceName string, limit int) ([]PostgresSessionStateCountRow, error) {
	if limit <= 0 {
		limit = 180
	}
	q := `
		SELECT capture_timestamp, server_instance_name,
		       active_count, idle_count, idle_in_txn_count, waiting_count, total_count
		FROM postgres_session_state_counts
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresSessionStateCountRow
	for rows.Next() {
		var r PostgresSessionStateCountRow
		if err := rows.Scan(
			&r.CaptureTimestamp, &r.ServerInstanceName,
			&r.ActiveCount, &r.IdleCount, &r.IdleInTxnCount, &r.WaitingCount, &r.TotalCount,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (r PostgresSessionStateCountRow) String() string {
	return fmt.Sprintf("%s a=%d idle=%d iit=%d w=%d t=%d",
		r.ServerInstanceName, r.ActiveCount, r.IdleCount, r.IdleInTxnCount, r.WaitingCount, r.TotalCount)
}

