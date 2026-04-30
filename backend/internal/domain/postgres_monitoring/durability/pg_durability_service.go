// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Service layer for PostgreSQL durability monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package durability

type DurabilityService struct {
	repo *DurabilityRepository
}

func NewDurabilityService(repo *DurabilityRepository) *DurabilityService {
	return &DurabilityService{repo: repo}
}
