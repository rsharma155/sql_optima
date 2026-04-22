// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server instance status management.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

func (c *SqlServerRepository) GetInstanceStatus(instanceName string) string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	if status, ok := c.status[instanceName]; ok {
		return status
	}
	return "unknown"
}

func (c *SqlServerRepository) GetAllInstanceStatuses() map[string]string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	statuses := make(map[string]string, len(c.status))
	for name, status := range c.status {
		statuses[name] = status
	}
	return statuses
}

func (c *SqlServerRepository) UpdateInstanceStatus(instanceName string) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	c.mutex.Lock()
	defer c.mutex.Unlock()
	if !ok || db == nil {
		c.status[instanceName] = "offline"
		return
	}

	if err := db.Ping(); err != nil {
		c.status[instanceName] = "offline"
	} else {
		c.status[instanceName] = "online"
	}
}