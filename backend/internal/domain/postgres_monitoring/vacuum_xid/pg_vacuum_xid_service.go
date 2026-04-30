// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Service layer for PostgreSQL vacuum_xid monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package vacuum_xid

type VacuumXidService struct {
	repo *VacuumXidRepository
}

func NewVacuumXidService(repo *VacuumXidRepository) *VacuumXidService {
	return &VacuumXidService{repo: repo}
}
