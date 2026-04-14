// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Cluster name resolution and identification for multi-node deployments.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
)

func (c *PgRepository) FetchClusterName(instanceName string) (string, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		return "", fmt.Errorf("connection not found")
	}
	var clusterName string
	err := db.QueryRow("SELECT COALESCE(setting,'') FROM pg_settings WHERE name='cluster_name'").Scan(&clusterName)
	if err != nil {
		return "", err
	}
	return clusterName, nil
}

