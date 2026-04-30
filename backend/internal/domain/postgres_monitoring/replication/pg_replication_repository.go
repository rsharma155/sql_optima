// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository implementation for PostgreSQL replication monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package replication

import "github.com/jackc/pgx/v5/pgxpool"

type ReplicationRepository struct {
	pool *pgxpool.Pool
}

func NewReplicationRepository(pool *pgxpool.Pool) *ReplicationRepository {
	return &ReplicationRepository{pool: pool}
}
