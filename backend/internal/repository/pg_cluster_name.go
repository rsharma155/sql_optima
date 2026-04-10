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

