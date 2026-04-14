// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Extended Events file target processing worker.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/models"
)

// XeFileTargetWorker periodically polls a SQL Server Extended Events file target
// using sys.fn_xe_file_target_read_file and persists both:
// - parsed events into sql_server_xevents (SQLite)
// - polling cursor (last_file_name/last_offset) into sql_server_xevent_state (SQLite)
type XeFileTargetWorker struct {
	sqlitePath string
	db         *sql.DB
}

func NewXeFileTargetWorker(sqlitePath string) (*XeFileTargetWorker, error) {
	// modernc.org/sqlite driver uses the DSN format "file:<path>?mode=rwc"
	abs, err := filepath.Abs(sqlitePath)
	if err != nil {
		return nil, err
	}

	sqliteDSN := fmt.Sprintf("file:%s?mode=rwc&_pragma=foreign_keys(1)", abs)
	sdb, err := sql.Open("sqlite", sqliteDSN)
	if err != nil {
		return nil, err
	}

	worker := &XeFileTargetWorker{
		sqlitePath: abs,
		db:         sdb,
	}
	if err := worker.initSchema(); err != nil {
		_ = sdb.Close()
		return nil, err
	}
	return worker, nil
}

func (w *XeFileTargetWorker) initSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS xevent_state (
			server_instance_name TEXT PRIMARY KEY,
			last_file_name TEXT NULL,
			last_offset INTEGER NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS sql_server_xevents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_instance_name TEXT NOT NULL,
			event_type TEXT NOT NULL,
			event_timestamp TEXT NULL,
			event_data_xml TEXT NOT NULL,
			parsed_payload_json TEXT NULL,
			file_name TEXT NOT NULL,
			file_offset INTEGER NOT NULL,
			inserted_at TEXT NOT NULL,
			UNIQUE(server_instance_name, file_name, file_offset)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sql_server_xevents_ts ON sql_server_xevents(event_timestamp);`,
		`CREATE INDEX IF NOT EXISTS idx_sql_server_xevents_server_offset ON sql_server_xevents(server_instance_name, file_offset);`,
	}

	for _, s := range stmts {
		if _, err := w.db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func (w *XeFileTargetWorker) getState(serverInstance string) (lastFile *string, lastOffset *int64, ok bool, err error) {
	row := w.db.QueryRow(
		`SELECT last_file_name, last_offset
		 FROM xevent_state
		 WHERE server_instance_name = ?`,
		serverInstance,
	)

	var lastFileName sql.NullString
	var lastOff sql.NullInt64
	if err := row.Scan(&lastFileName, &lastOff); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, false, nil
		}
		return nil, nil, false, err
	}

	if lastFileName.Valid {
		tmp := lastFileName.String
		lastFile = &tmp
	}
	if lastOff.Valid {
		tmp := lastOff.Int64
		lastOffset = &tmp
	}
	return lastFile, lastOffset, true, nil
}

func (w *XeFileTargetWorker) upsertState(serverInstance string, lastFile *string, lastOffset *int64) error {
	now := time.Now().Format("2006-01-02 15:04:05")

	var lastFileArg interface{} = nil
	if lastFile != nil {
		lastFileArg = *lastFile
	}
	var lastOffsetArg interface{} = nil
	if lastOffset != nil {
		lastOffsetArg = *lastOffset
	}

	_, err := w.db.Exec(
		`INSERT INTO xevent_state (server_instance_name, last_file_name, last_offset, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(server_instance_name) DO UPDATE SET
		    last_file_name=excluded.last_file_name,
		    last_offset=excluded.last_offset,
		    updated_at=excluded.updated_at`,
		serverInstance,
		lastFileArg,
		lastOffsetArg,
		now,
	)
	return err
}

func (w *XeFileTargetWorker) insertEvents(events []models.SqlServerXeEvent) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := w.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO sql_server_xevents (
			server_instance_name, event_type, event_timestamp, event_data_xml, parsed_payload_json,
			file_name, file_offset, inserted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	insertedAt := time.Now().Format("2006-01-02 15:04:05")
	for _, e := range events {
		if _, err := stmt.Exec(
			e.ServerInstanceName,
			e.EventType,
			nullIfEmpty(e.EventTimestamp),
			e.EventDataXML,
			nullIfEmpty(e.ParsedPayloadJSON),
			e.FileName,
			e.FileOffset,
			insertedAt,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func nullIfEmpty(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// XeEventEnvelope is a best-effort parser for XEvent XML fragments returned by CAST(event_data AS XML).
// We extract timestamp plus <data>/<action> name-value pairs into a map.
type XeEventEnvelope struct {
	XMLName   xml.Name    `xml:"event"`
	Timestamp string      `xml:"timestamp,attr"`
	Data      []XeElement `xml:"data"`
	Actions   []XeElement `xml:"action"`
}

type XeElement struct {
	Name string `xml:"name,attr"`
	// Some xevent values are encoded as <type ...>VALUE</type> inside <data>/<action>.
	TypeNodes []struct {
		Value string `xml:",chardata"`
	} `xml:"type"`
	ValueNodes []string `xml:"value"`
	InnerXML   string   `xml:",innerxml"`
}

func (w *XeFileTargetWorker) parseEventXML(eventType string, serverInstance string, eventDataXML string, fileName string, fileOffset int64) models.SqlServerXeEvent {
	ev := models.SqlServerXeEvent{
		ServerInstanceName: serverInstance,
		EventType:          eventType,
		EventTimestamp:     "",
		EventDataXML:       eventDataXML,
		ParsedPayloadJSON:  "",
		FileName:           fileName,
		FileOffset:         fileOffset,
	}

	var env XeEventEnvelope
	if err := xml.Unmarshal([]byte(eventDataXML), &env); err == nil {
		ev.EventTimestamp = strings.TrimSpace(env.Timestamp)

		payload := make(map[string]string)
		for _, d := range env.Data {
			payload[d.Name] = extractXeElementValue(d)
		}
		for _, a := range env.Actions {
			// If action name collides, last wins (fine for observability).
			payload[a.Name] = extractXeElementValue(a)
		}

		if len(payload) > 0 {
			if b, err := json.Marshal(payload); err == nil {
				ev.ParsedPayloadJSON = string(b)
			}
		}
	}

	return ev
}

func extractXeElementValue(el XeElement) string {
	if len(el.TypeNodes) > 0 {
		v := strings.TrimSpace(el.TypeNodes[0].Value)
		if v != "" {
			return v
		}
	}
	if len(el.ValueNodes) > 0 {
		v := strings.TrimSpace(el.ValueNodes[0])
		if v != "" {
			return v
		}
	}
	// Fallback: strip and keep inner XML for debugging.
	return strings.TrimSpace(el.InnerXML)
}

func (w *XeFileTargetWorker) readXeEvents(
	db *sql.DB,
	lastFileName *string,
	lastOffset *int64,
	serverInstance string,
) (events []models.SqlServerXeEvent, maxFileName *string, maxOffset *int64, err error) {

	// Important: ORDER BY so our "max" is the last row deterministically.
	/* 	const q = `
		SELECT
			object_name AS event_type,
			CAST(event_data AS XML) AS event_data_xml,
			file_name,
			file_offset
		FROM sys.fn_xe_file_target_read_file(?, NULL, ?, ?)
		ORDER BY file_name, file_offset;
	` */

	// Important: ORDER BY so our "max" is the last row deterministically.
	// FIX: Use @p1, @p2, @p3 for the mssql driver instead of ?
	const q = `
		SELECT 
			object_name AS event_type,
			CAST(event_data AS XML) AS event_data_xml,
			file_name,
			file_offset
		FROM sys.fn_xe_file_target_read_file(@p1, NULL, @p2, @p3)
		ORDER BY file_name, file_offset;
	`

	pattern := config.DefaultXeFileTargetPattern

	var fileName sql.NullString
	var fileOffset int64
	var eventType string
	var eventXMLBytes []byte

	rows, err := db.Query(q, pattern, lastFileName, lastOffset)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&eventType, &eventXMLBytes, &fileName, &fileOffset); err != nil {
			return nil, nil, nil, err
		}

		fn := fileName.String
		evXML := strings.TrimSpace(string(eventXMLBytes))
		parsed := w.parseEventXML(eventType, serverInstance, evXML, fn, fileOffset)
		events = append(events, parsed)

		// Since we ordered by (file_name, file_offset), the last row is the max.
		if maxFileName == nil {
			maxFileName = &fn
			maxOffset = &fileOffset
		} else {
			tmpFn := fn
			tmpOff := fileOffset
			maxFileName = &tmpFn
			maxOffset = &tmpOff
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}

	return events, maxFileName, maxOffset, nil
}

// StartXeFileTargetWorker is implemented at the MetricsService layer where we have access to instances.
func (s *MetricsService) StartXeFileTargetWorker(ctx context.Context) {
	worker, err := NewXeFileTargetWorker(config.DefaultXeSQLitePath)
	if err != nil {
		log.Printf("[XE] Failed to init SQLite worker: %v", err)
		return
	}

	// Share the database connection with MetricsService so GetRecentXEvents can use it
	s.xeDb = worker.db
	s.xeSqlitePath = worker.sqlitePath

	log.Printf("[XE] Starting Extended Events file target worker. SQLite: %s", worker.sqlitePath)

	// Poll sequentially per tick for simplicity; can be parallelized later.
	ticker := time.NewTicker(config.DefaultXeFileTargetInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = worker.db.Close()
			return
		case <-ticker.C:
			for _, inst := range s.Config.Instances {
				if inst.Type != "sqlserver" {
					continue
				}

				instanceName := inst.Name
				dbConn, ok := s.MsRepo.GetConn(instanceName)
				if !ok || dbConn == nil {
					log.Printf("[XE] No MSSQL connection for %s", instanceName)
					continue
				}

				lastFile, lastOffset, stateExists, stErr := worker.getState(instanceName)
				if stErr != nil {
					log.Printf("[XE] State read failed for %s: %v", instanceName, stErr)
					continue
				}

				events, maxFile, maxOffset, rErr := worker.readXeEvents(dbConn, lastFile, lastOffset, instanceName)
				// Rollover resilience:
				// - if read fails (bad offset), retry with NULL cursor
				// - if read returns nothing but we had state, retry with NULL cursor (new xel file)
				if rErr != nil {
					log.Printf("[XE] Read failed for %s; retrying with NULL cursor. Error: %v", instanceName, rErr)
					events2, maxFile2, maxOffset2, rErr2 := worker.readXeEvents(dbConn, nil, nil, instanceName)
					if rErr2 != nil {
						log.Printf("[XE] Read reset cursor failed for %s: %v", instanceName, rErr2)
						// Still reset state cursor so next tick doesn't keep failing.
						_ = worker.upsertState(instanceName, nil, nil)
						continue
					}
					events, maxFile, maxOffset = events2, maxFile2, maxOffset2

					// If reset cursor returned nothing, update persisted cursor to NULL so we don't
					// keep attempting to read using an invalid file/offset.
					if len(events) == 0 {
						_ = worker.upsertState(instanceName, nil, nil)
						continue
					}
				} else if len(events) == 0 && stateExists {
					// No events returned from the current cursor; likely rollover. Retry with NULL cursor once.
					events2, maxFile2, maxOffset2, rErr2 := worker.readXeEvents(dbConn, nil, nil, instanceName)
					if rErr2 != nil {
						log.Printf("[XE] Read rollover retry failed for %s: %v", instanceName, rErr2)
						continue
					}
					events, maxFile, maxOffset = events2, maxFile2, maxOffset2
				}

				if len(events) == 0 || maxFile == nil || maxOffset == nil {
					continue
				}

				if insErr := worker.insertEvents(events); insErr != nil {
					log.Printf("[XE] Insert failed for %s: %v", instanceName, insErr)
					continue
				}

				if upErr := worker.upsertState(instanceName, maxFile, maxOffset); upErr != nil {
					log.Printf("[XE] State update failed for %s: %v", instanceName, upErr)
					continue
				}

				log.Printf("[XE] %s: inserted %d event(s); cursor -> %s @ %d",
					instanceName, len(events), *maxFile, *maxOffset)
			}
		}
	}
}
