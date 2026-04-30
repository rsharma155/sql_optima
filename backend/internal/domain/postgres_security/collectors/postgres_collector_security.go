// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Security data collectors.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"context"
	"database/sql"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_security/domain/entities"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_security/domain/repositories"
	"time"
)

type PostgresSecurityCollector struct {
	repo       *repositories.PostgresSecurityRepository
	instanceID string
}

func NewPostgresSecurityCollector(repo *repositories.PostgresSecurityRepository, instanceID string) *PostgresSecurityCollector {
	return &PostgresSecurityCollector{
		repo:       repo,
		instanceID: instanceID,
	}
}

func (c *PostgresSecurityCollector) CollectRoleSnapshot(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT rolname, rolsuper, rolcreatedb, rolcreaterole, rolreplication, rolcanlogin
		FROM pg_roles`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var roles []entities.RoleSnapshot
	now := time.Now().UTC()
	for rows.Next() {
		var s entities.RoleSnapshot
		s.TS = now
		s.InstanceID = c.instanceID
		err := rows.Scan(&s.Rolname, &s.Rolsuper, &s.Rolcreatedb, &s.Rolcreaterole, &s.Rolreplication, &s.Rolcanlogin)
		if err != nil {
			return err
		}
		roles = append(roles, s)
	}

	return c.repo.SaveRoleSnapshot(ctx, roles)
}

func (c *PostgresSecurityCollector) CollectDDLActivity(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT schemaname, relname, n_tup_ins, n_tup_upd, n_tup_del
		FROM pg_stat_user_tables`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var activities []entities.DDLActivity
	now := time.Now().UTC()
	for rows.Next() {
		var a entities.DDLActivity
		a.TS = now
		a.InstanceID = c.instanceID
		err := rows.Scan(&a.Schemaname, &a.Relname, &a.NTupIns, &a.NTupUpd, &a.NTupDel)
		if err != nil {
			return err
		}
		activities = append(activities, a)
	}

	return c.repo.SaveDDLActivity(ctx, activities)
}

func (c *PostgresSecurityCollector) ParseFailedLogins(ctx context.Context, logLines []string) error {
	// Simple placeholder for log parsing logic
	// In a real scenario, this would be called by a log tailer
	for _, line := range logLines {
		// Mock parsing
		event := entities.FailedLoginEvent{
			TS:         time.Now().UTC(),
			InstanceID: c.instanceID,
			Username:   "unknown",
			ClientAddr: "127.0.0.1",
			Message:    line,
		}
		_ = c.repo.SaveFailedLoginEvent(ctx, event)
	}
	return nil
}
