// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository implementation for PostgreSQL storage_bloat monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package storage_bloat

import "github.com/jackc/pgx/v5/pgxpool"

type StorageBloatRepository struct {
	pool *pgxpool.Pool
}

func NewStorageBloatRepository(pool *pgxpool.Pool) *StorageBloatRepository {
	return &StorageBloatRepository{pool: pool}
}
