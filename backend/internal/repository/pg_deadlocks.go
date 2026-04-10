package repository

import (
	"database/sql"
	"fmt"
)

type PgDeadlockTotalRow struct {
	DatabaseName   string `json:"database_name"`
	DeadlocksTotal int64  `json:"deadlocks_total"`
}

func (c *PgRepository) GetDeadlocksTotalByDB(instanceName string) ([]PgDeadlockTotalRow, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	q := `
		SELECT datname, deadlocks
		FROM pg_stat_database
		WHERE datname IS NOT NULL AND datname <> ''
		ORDER BY deadlocks DESC, datname
	`
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PgDeadlockTotalRow
	for rows.Next() {
		var r PgDeadlockTotalRow
		var dl sql.NullInt64
		if err := rows.Scan(&r.DatabaseName, &dl); err != nil {
			continue
		}
		if dl.Valid {
			r.DeadlocksTotal = dl.Int64
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

