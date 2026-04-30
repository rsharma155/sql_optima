// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository implementation for PostgreSQL durability monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package durability

import "github.com/jackc/pgx/v5/pgxpool"

type DurabilityRepository struct {
	pool *pgxpool.Pool
}

func NewDurabilityRepository(pool *pgxpool.Pool) *DurabilityRepository {
	return &DurabilityRepository{pool: pool}
}
