// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Service layer for PostgreSQL replication monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package replication

type ReplicationService struct {
	repo *ReplicationRepository
}

func NewReplicationService(repo *ReplicationRepository) *ReplicationService {
	return &ReplicationService{repo: repo}
}
