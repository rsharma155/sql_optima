// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Service layer for PostgreSQL memory_cache monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package memory_cache

type MemoryCacheService struct {
	repo *MemoryCacheRepository
}

func NewMemoryCacheService(repo *MemoryCacheRepository) *MemoryCacheService {
	return &MemoryCacheService{repo: repo}
}
