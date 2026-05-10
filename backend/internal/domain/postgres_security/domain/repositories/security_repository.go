// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Security data repositories.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repositories

import (
	"context"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_security/domain/entities"
)

type PostgresSecurityRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresSecurityRepository(pool *pgxpool.Pool) *PostgresSecurityRepository {
	return &PostgresSecurityRepository{pool: pool}
}

func (r *PostgresSecurityRepository) SaveRoleSnapshot(ctx context.Context, roles []entities.RoleSnapshot) error {
	for _, s := range roles {
		_, err := r.pool.Exec(ctx, `
			INSERT INTO monitor.pg_roles_snapshot (ts, instance_id, rolname, rolsuper, rolcreatedb, rolcreaterole, rolreplication, rolcanlogin)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			s.TS, s.InstanceID, s.Rolname, s.Rolsuper, s.Rolcreatedb, s.Rolcreaterole, s.Rolreplication, s.Rolcanlogin)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresSecurityRepository) SaveFailedLoginEvent(ctx context.Context, e entities.FailedLoginEvent) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO monitor.pg_failed_login_events (ts, instance_id, username, client_addr, message)
		VALUES ($1, $2, $3, $4, $5)`,
		e.TS, e.InstanceID, e.Username, e.ClientAddr, e.Message)
	return err
}

func (r *PostgresSecurityRepository) SaveDDLActivity(ctx context.Context, activities []entities.DDLActivity) error {
	for _, a := range activities {
		_, err := r.pool.Exec(ctx, `
			INSERT INTO monitor.pg_ddl_activity_ts (ts, instance_id, schemaname, relname, n_tup_ins, n_tup_upd, n_tup_del)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			a.TS, a.InstanceID, a.Schemaname, a.Relname, a.NTupIns, a.NTupUpd, a.NTupDel)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresSecurityRepository) GetKPIData(ctx context.Context, instance string, from, to string) (map[string]interface{}, error) {
	var failedLogins int64
	r.pool.QueryRow(ctx, "SELECT count(*) FROM monitor.pg_failed_login_events WHERE ts BETWEEN $1 AND $2 AND instance_id = $3", from, to, instance).Scan(&failedLogins)

	var superusers int64
	r.pool.QueryRow(ctx, "SELECT count(*) FROM monitor.pg_roles_snapshot WHERE rolsuper AND instance_id = $1 AND ts = (SELECT max(ts) FROM monitor.pg_roles_snapshot WHERE instance_id = $1)", instance).Scan(&superusers)

	var replPrivs int64
	r.pool.QueryRow(ctx, "SELECT count(*) FROM monitor.pg_roles_snapshot WHERE rolreplication AND instance_id = $1 AND ts = (SELECT max(ts) FROM monitor.pg_roles_snapshot WHERE instance_id = $1)", instance).Scan(&replPrivs)

	var newRoles int64
	r.pool.QueryRow(ctx, "SELECT count(*) FROM monitor.pg_roles_snapshot WHERE ts > now() - interval '7 days' AND instance_id = $1", instance).Scan(&newRoles)

	return map[string]interface{}{
		"failed_logins":   failedLogins,
		"superusers":      superusers,
		"repl_privileges": replPrivs,
		"new_roles":       newRoles,
	}, nil
}

func (r *PostgresSecurityRepository) GetFailedLoginTrend(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT time_bucket('10 min', ts) AS bucket, count(*)
		FROM monitor.pg_failed_login_events
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3
		GROUP BY bucket ORDER BY bucket ASC`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var count int64
		if err := rows.Scan(&bucket, &count); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"bucket": bucket,
			"count":  count,
		})
	}
	return results, nil
}

func (r *PostgresSecurityRepository) GetSuperusers(ctx context.Context, instance string) ([]map[string]interface{}, error) {
	query := `
		SELECT rolname, rolcreatedb, rolcreaterole, rolreplication
		FROM monitor.pg_roles_snapshot
		WHERE rolsuper AND instance_id = $1 AND ts = (SELECT max(ts) FROM monitor.pg_roles_snapshot WHERE instance_id = $1)`
	
	rows, err := r.pool.Query(ctx, query, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var name string
		var createdb, createrole, repl bool
		if err := rows.Scan(&name, &createdb, &createrole, &repl); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"rolname":        name,
			"rolcreatedb":    createdb,
			"rolcreaterole":  createrole,
			"rolreplication": repl,
		})
	}
	return results, nil
}

func (r *PostgresSecurityRepository) GetElevatedRoles(ctx context.Context, instance string) ([]map[string]interface{}, error) {
	query := `
		SELECT rolname, rolsuper, rolcreaterole, rolreplication
		FROM monitor.pg_roles_snapshot
		WHERE (rolsuper OR rolreplication OR rolcreaterole) AND instance_id = $1 
		AND ts = (SELECT max(ts) FROM monitor.pg_roles_snapshot WHERE instance_id = $1)`
	
	rows, err := r.pool.Query(ctx, query, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var name string
		var super, createrole, repl bool
		if err := rows.Scan(&name, &super, &createrole, &repl); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"rolname":        name,
			"rolsuper":       super,
			"rolcreaterole":  createrole,
			"rolreplication": repl,
		})
	}
	return results, nil
}

func (r *PostgresSecurityRepository) GetAllRoles(ctx context.Context, instance string) ([]map[string]interface{}, error) {
	query := `
		SELECT rolname, rolsuper, rolcreaterole, rolreplication, rolcanlogin, rolcreatedb
		FROM monitor.pg_roles_snapshot
		WHERE instance_id = $1 
		AND ts = (SELECT max(ts) FROM monitor.pg_roles_snapshot WHERE instance_id = $1)`
	
	rows, err := r.pool.Query(ctx, query, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var name string
		var super, createrole, repl, canlogin, createdb bool
		if err := rows.Scan(&name, &super, &createrole, &repl, &canlogin, &createdb); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"rolname":        name,
			"rolsuper":       super,
			"rolcreaterole":  createrole,
			"rolreplication": repl,
			"rolcanlogin":    canlogin,
			"rolcreatedb":    createdb,
		})
	}
	return results, nil
}

func (r *PostgresSecurityRepository) GetDMLActivityTrend(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT time_bucket('10 min', ts) AS bucket,
		       sum(n_tup_ins) as ins,
		       sum(n_tup_upd) as upd,
		       sum(n_tup_del) as del
		FROM monitor.pg_ddl_activity_ts
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3
		GROUP BY bucket ORDER BY bucket ASC`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var ins, upd, del float64
		if err := rows.Scan(&bucket, &ins, &upd, &del); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"bucket": bucket,
			"ins":    ins,
			"upd":    upd,
			"del":    del,
		})
	}
	return results, nil
}

func (r *PostgresSecurityRepository) GetConnectionOrigins(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT client_addr, count(*) as fails
		FROM monitor.pg_failed_login_events
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3
		GROUP BY client_addr
		ORDER BY fails DESC
		LIMIT 5`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var client string
		var fails int64
		if err := rows.Scan(&client, &fails); err != nil {
			return nil, err
		}
		if client == "" {
			client = "unknown"
		}
		results = append(results, map[string]interface{}{
			"client_addr": client,
			"fails":       fails,
		})
	}
	return results, nil
}

func (r *PostgresSecurityRepository) GetRoleModificationsTrend(ctx context.Context, instance string, from, to string) ([]map[string]interface{}, error) {
	query := `
		SELECT time_bucket('1 hour', ts) AS bucket, count(distinct rolname) as total_roles
		FROM monitor.pg_roles_snapshot
		WHERE ts BETWEEN $1 AND $2 AND instance_id = $3
		GROUP BY bucket ORDER BY bucket ASC`
	
	rows, err := r.pool.Query(ctx, query, from, to, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var total int64
		if err := rows.Scan(&bucket, &total); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"bucket": bucket,
			"total":  total,
		})
	}
	return results, nil
}
