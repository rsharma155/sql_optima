// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Database size calculation and storage usage tracking per database.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"time"
)

type PgDatabaseSizeRow struct {
	Database string `json:"database"`
	Bytes    int64  `json:"bytes"`
}

type PgDatabaseSizeStats struct {
	Instance          string              `json:"instance"`
	Timestamp         string              `json:"timestamp"`
	TotalBytes        int64               `json:"total_bytes"`
	GrowthBytesPerHr  float64             `json:"growth_bytes_per_hr,omitempty"`
	ByDatabase        []PgDatabaseSizeRow  `json:"by_database"`
	Error             string              `json:"error,omitempty"`
}

func (c *PgRepository) GetDatabaseSizeStats(instanceName string) PgDatabaseSizeStats {
	out := PgDatabaseSizeStats{
		Instance:  instanceName,
		Timestamp: time.Now().Format("15:04:05"),
	}

	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		out.Error = "connection not found"
		return out
	}

	rows, err := db.Query(`
		SELECT datname, pg_database_size(datname)
		FROM pg_database
		WHERE datistemplate = false
		ORDER BY pg_database_size(datname) DESC;
	`)
	if err != nil {
		if err == sql.ErrNoRows {
			return out
		}
		out.Error = err.Error()
		return out
	}
	defer rows.Close()

	var total int64
	var list []PgDatabaseSizeRow
	for rows.Next() {
		var name string
		var bytes int64
		if scanErr := rows.Scan(&name, &bytes); scanErr == nil {
			list = append(list, PgDatabaseSizeRow{Database: name, Bytes: bytes})
			total += bytes
		}
	}
	out.TotalBytes = total
	out.ByDatabase = list

	// Growth estimate from last sample.
	c.mutex.Lock()
	prevBytes, hadPrev := c.lastDbSizeBytes[instanceName]
	prevAt, hadAt := c.lastDbSizeAt[instanceName]
	c.lastDbSizeBytes[instanceName] = total
	c.lastDbSizeAt[instanceName] = time.Now()
	c.mutex.Unlock()

	if hadPrev && hadAt {
		dt := time.Since(prevAt).Hours()
		if dt > 0.01 {
			out.GrowthBytesPerHr = float64(total-prevBytes) / dt
		}
	}
	return out
}

