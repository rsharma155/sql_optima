// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Cluster name resolution and identification for multi-node deployments.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"fmt"
	"strings"
)

func (c *PgRepository) FetchClusterName(ctx context.Context, instanceName string) (string, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		return "", fmt.Errorf("connection not found")
	}
	var clusterName string
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, "SELECT /* SQL_OPTIMA */   COALESCE(setting,'') FROM pg_settings WHERE name='cluster_name'").Scan(&clusterName)
	if err != nil {
		return "", err
	}
	return clusterName, nil
}
