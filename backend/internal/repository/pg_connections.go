package repository

import (
	"database/sql"
	"log"
)

// CollectPgConnections fetches PostgreSQL connection stats
func (c *PgRepository) CollectPgConnections(db *sql.DB) (int, int, int, error) {
	query := `
		SELECT 
			COALESCE(SUM(CASE WHEN state = 'active' THEN 1 ELSE 0 END), 0) AS active_connections,
			COALESCE(SUM(CASE WHEN state = 'idle' THEN 1 ELSE 0 END), 0) AS idle_connections,
			COUNT(*) AS total_connections
		FROM pg_stat_activity
		WHERE pid != pg_backend_pid()
	`

	var active, idle, total int
	err := db.QueryRow(query).Scan(&active, &idle, &total)
	if err != nil {
		log.Printf("[PostgreSQL] Connection Stats Query Error: %v", err)
	}
	return active, idle, total, err
}

// CollectPgDatabases fetches list of databases
func (c *PgRepository) CollectPgDatabases(db *sql.DB) ([]string, error) {
	query := `
		SELECT datname 
		FROM pg_database 
		WHERE datistemplate = false 
		ORDER BY datname
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[PostgreSQL] Databases Query Error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err == nil {
			results = append(results, dbName)
		}
	}
	return results, nil
}
