// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Service for PostgreSQL Workload.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package workload

type WorkloadService struct {
	repo *WorkloadRepository
}

func NewWorkloadService(repo *WorkloadRepository) *WorkloadService {
	return &WorkloadService{repo: repo}
}
