// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Extended Events models for SQL Server trace event capture and parsing.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package models

import "github.com/google/uuid"

// SqlServerXeEvent represents one row/event parsed from sys.fn_xe_file_target_read_file output.
// eventDataXML is stored as-is so the UI / drilldowns can reuse it later.
type SqlServerXeEvent struct {
	ServerID          uuid.UUID `json:"server_id"`
	EventType         string    `json:"event_type"`
	EventTimestamp    string    `json:"event_timestamp"`
	EventDataXML      string    `json:"event_data_xml"`
	ParsedPayloadJSON string    `json:"parsed_payload_json"`

	FileName   string `json:"file_name"`
	FileOffset int64  `json:"file_offset"`
}

// XeFileTargetState is persisted in SQLite so we can poll incrementally.
type XeFileTargetState struct {
	ServerID     uuid.UUID
	LastFileName *string
	LastOffset   *int64
}
