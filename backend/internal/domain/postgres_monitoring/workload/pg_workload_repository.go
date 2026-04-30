// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository for PostgreSQL Workload.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package workload

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

type WorkloadRepository struct {
	pool *pgxpool.Pool
}

func NewWorkloadRepository(pool *pgxpool.Pool) *WorkloadRepository {
	return &WorkloadRepository{pool: pool}
}
