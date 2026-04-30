// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server log shipping health monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"fmt"
)

type LogShippingHealth struct {
	PrimaryServer           string
	PrimaryDatabase         string
	SecondaryServer         string
	SecondaryDatabase       string
	LastBackupDate          sql.NullTime
	LastBackupFile          string
	LastRestoreDate         sql.NullTime
	LastCopiedDate          sql.NullTime
	RestoreDelayMinutes     int
	RestoreThresholdMinutes int
	Status                  int
	IsPrimary               bool
}

func (c *SqlServerRepository) FetchLogShippingHealth(instanceName string) ([]LogShippingHealth, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for %s", instanceName)
	}

	var results []LogShippingHealth

	var primaryCount int
	_ = db.QueryRow(`SELECT /* SQL_OPTIMA */   COUNT(*) FROM msdb.dbo.log_shipping_primary_databases WITH (NOLOCK)`).Scan(&primaryCount)
	if primaryCount > 0 {
		rows, err := db.Query(`
			/* SQL_OPTIMA */ SELECT  
				ISNULL(CAST(SERVERPROPERTY('ServerName') AS NVARCHAR(128)), '') AS primary_server,
				lp.primary_database,
				'' AS secondary_server,
				'' AS secondary_database,
				lpm.last_backup_date,
				ISNULL(lpm.last_backup_file, '') AS last_backup_file,
				CAST(NULL AS DATETIME) AS last_restored_date,
				CAST(NULL AS DATETIME) AS last_copied_date,
				0 AS restore_delay,
				0 AS restore_threshold,
				ISNULL(lpm.status, 1) AS status
			FROM msdb.dbo.log_shipping_primary_databases lp WITH (NOLOCK)
			LEFT JOIN msdb.dbo.log_shipping_monitor_primary lpm WITH (NOLOCK) 
				ON lpm.primary_database = lp.primary_database
		`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var h LogShippingHealth
				h.IsPrimary = true
				if err := rows.Scan(&h.PrimaryServer, &h.PrimaryDatabase, &h.SecondaryServer, &h.SecondaryDatabase,
					&h.LastBackupDate, &h.LastBackupFile, &h.LastRestoreDate, &h.LastCopiedDate,
					&h.RestoreDelayMinutes, &h.RestoreThresholdMinutes, &h.Status); err == nil {
					results = append(results, h)
				}
			}
		}
	}

	var secondaryCount int
	_ = db.QueryRow(`SELECT /* SQL_OPTIMA */   COUNT(*) FROM msdb.dbo.log_shipping_secondary_databases WITH (NOLOCK)`).Scan(&secondaryCount)
	if secondaryCount > 0 {
		rows, err := db.Query(`
			/* SQL_OPTIMA */ SELECT  
				ISNULL(lss.primary_server,    '')     AS primary_server,
				ISNULL(lss.primary_database,  '')     AS primary_database,
				ISNULL(CAST(SERVERPROPERTY('ServerName') AS NVARCHAR(128)), '') AS secondary_server,
				ISNULL(lss.secondary_database,'')     AS secondary_database,
				lsm.last_backup_date,
				ISNULL(lsm.last_backup_file,  '')     AS last_backup_file,
				lsm.last_restored_date,
				lsm.last_copied_date,
				lss.restore_delay,
				lss.restore_threshold,
				ISNULL(lsm.status, 1) AS status
			FROM msdb.dbo.log_shipping_secondary_databases   lss WITH (NOLOCK)
			LEFT JOIN msdb.dbo.log_shipping_monitor_secondary lsm WITH (NOLOCK)
			   ON lsm.secondary_database = lss.secondary_database
		`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var h LogShippingHealth
				h.IsPrimary = false
				if err := rows.Scan(&h.PrimaryServer, &h.PrimaryDatabase, &h.SecondaryServer, &h.SecondaryDatabase,
					&h.LastBackupDate, &h.LastBackupFile, &h.LastRestoreDate, &h.LastCopiedDate,
					&h.RestoreDelayMinutes, &h.RestoreThresholdMinutes, &h.Status); err == nil {
					results = append(results, h)
				}
			}
		}
	}

	return results, nil
}
