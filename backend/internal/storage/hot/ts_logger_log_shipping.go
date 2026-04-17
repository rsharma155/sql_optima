// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Log shipping health metrics logger for SQL Server log shipping monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

// LogShippingRow is the TimescaleDB-facing representation of one log shipping health snapshot.
type LogShippingRow struct {
	CaptureTimestamp        time.Time
	ServerInstanceName      string
	PrimaryServer           string
	PrimaryDatabase         string
	SecondaryServer         string
	SecondaryDatabase       string
	LastBackupDate          *time.Time
	LastBackupFile          string
	LastRestoreDate         *time.Time
	LastCopiedDate          *time.Time
	RestoreDelayMinutes     int
	RestoreThresholdMinutes int
	Status                  int
	IsPrimary               bool
}

// LogLogShippingHealth batch-inserts log shipping health rows.
func (tl *TimescaleLogger) LogLogShippingHealth(ctx context.Context, instanceName string, rows []LogShippingRow) error {
	if len(rows) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO sqlserver_log_shipping_health (
			capture_timestamp, server_instance_name,
			primary_server, primary_database,
			secondary_server, secondary_database,
			last_backup_date, last_backup_file,
			last_restore_date, last_copied_date,
			restore_delay_minutes, restore_threshold_minutes,
			status, is_primary
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`

	for _, r := range rows {
		batch.Queue(query,
			r.CaptureTimestamp, r.ServerInstanceName,
			r.PrimaryServer, r.PrimaryDatabase,
			r.SecondaryServer, r.SecondaryDatabase,
			r.LastBackupDate, r.LastBackupFile,
			r.LastRestoreDate, r.LastCopiedDate,
			r.RestoreDelayMinutes, r.RestoreThresholdMinutes,
			r.Status, r.IsPrimary,
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("log shipping batch insert failed at row %d: %w", i, err)
		}
	}

	log.Printf("[TSLogger] LogLogShippingHealth: inserted %d rows for %s", len(rows), instanceName)
	return nil
}

// GetLogShippingHealth returns the most recent log shipping health snapshot for an instance.
func (tl *TimescaleLogger) GetLogShippingHealth(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	query := `
		SELECT DISTINCT ON (primary_database, secondary_database)
			capture_timestamp,
			primary_server,
			primary_database,
			secondary_server,
			secondary_database,
			last_backup_date,
			last_backup_file,
			last_restore_date,
			last_copied_date,
			restore_delay_minutes,
			restore_threshold_minutes,
			status,
			is_primary
		FROM sqlserver_log_shipping_health
		WHERE server_instance_name = $1
		  AND capture_timestamp >= NOW() - INTERVAL '1 hour'
		ORDER BY primary_database, secondary_database, capture_timestamp DESC
	`

	rows, err := tl.pool.Query(ctx, query, instanceName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var (
			captureTs        time.Time
			primaryServer    string
			primaryDB        string
			secondaryServer  string
			secondaryDB      string
			lastBackupDate   *time.Time
			lastBackupFile   string
			lastRestoreDate  *time.Time
			lastCopiedDate   *time.Time
			restoreDelay     int
			restoreThreshold int
			status           int
			isPrimary        bool
		)
		if err := rows.Scan(
			&captureTs,
			&primaryServer, &primaryDB,
			&secondaryServer, &secondaryDB,
			&lastBackupDate, &lastBackupFile,
			&lastRestoreDate, &lastCopiedDate,
			&restoreDelay, &restoreThreshold,
			&status, &isPrimary,
		); err != nil {
			continue
		}

		row := map[string]interface{}{
			"capture_timestamp":        captureTs,
			"primary_server":           primaryServer,
			"primary_database":         primaryDB,
			"secondary_server":         secondaryServer,
			"secondary_database":       secondaryDB,
			"last_backup_file":         lastBackupFile,
			"restore_delay_minutes":    restoreDelay,
			"restore_threshold_minutes": restoreThreshold,
			"status":                   status,
			"is_primary":               isPrimary,
		}
		if lastBackupDate != nil {
			row["last_backup_date"] = *lastBackupDate
		}
		if lastRestoreDate != nil {
			row["last_restore_date"] = *lastRestoreDate
		}
		if lastCopiedDate != nil {
			row["last_copied_date"] = *lastCopiedDate
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
