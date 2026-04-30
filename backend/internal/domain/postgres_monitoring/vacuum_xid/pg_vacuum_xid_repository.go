// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository implementation for PostgreSQL vacuum_xid monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package vacuum_xid

import "github.com/jackc/pgx/v5/pgxpool"

type VacuumXidRepository struct {
	pool *pgxpool.Pool
}

func NewVacuumXidRepository(pool *pgxpool.Pool) *VacuumXidRepository {
	return &VacuumXidRepository{pool: pool}
}
