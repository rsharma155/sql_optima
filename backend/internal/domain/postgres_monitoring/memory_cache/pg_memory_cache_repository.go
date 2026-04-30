// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository implementation for PostgreSQL memory_cache monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package memory_cache

import "github.com/jackc/pgx/v5/pgxpool"

type MemoryCacheRepository struct {
	pool *pgxpool.Pool
}

func NewMemoryCacheRepository(pool *pgxpool.Pool) *MemoryCacheRepository {
	return &MemoryCacheRepository{pool: pool}
}
