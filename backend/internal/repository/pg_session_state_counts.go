package repository

import (
	"fmt"
)

type PgSessionStateCounts struct {
	Active        int `json:"active"`
	Idle          int `json:"idle"`
	IdleInTxn     int `json:"idle_in_txn"`
	Waiting       int `json:"waiting"`
	Total         int `json:"total"`
}

func (c *PgRepository) GetSessionStateCounts(instanceName string) (*PgSessionStateCounts, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	q := `
		SELECT
			COUNT(*) FILTER (WHERE state = 'active') AS active_cnt,
			COUNT(*) FILTER (WHERE state = 'idle') AS idle_cnt,
			COUNT(*) FILTER (WHERE state = 'idle in transaction') AS idle_in_txn_cnt,
			COUNT(*) FILTER (WHERE wait_event_type IS NOT NULL AND wait_event_type <> '') AS waiting_cnt,
			COUNT(*) AS total_cnt
		FROM pg_stat_activity
		WHERE pid <> pg_backend_pid()
	`
	var out PgSessionStateCounts
	if err := db.QueryRow(q).Scan(&out.Active, &out.Idle, &out.IdleInTxn, &out.Waiting, &out.Total); err != nil {
		return nil, err
	}
	return &out, nil
}

