// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Service layer for PostgreSQL storage_bloat monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package storage_bloat

type StorageBloatService struct {
	repo *StorageBloatRepository
}

func NewStorageBloatService(repo *StorageBloatRepository) *StorageBloatService {
	return &StorageBloatService{repo: repo}
}
