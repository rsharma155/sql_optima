// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Extended Events models for SQL Server trace event capture and parsing.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package models

// SqlServerXeEvent represents one row/event parsed from sys.fn_xe_file_target_read_file output.
// eventDataXML is stored as-is so the UI / drilldowns can reuse it later.
type SqlServerXeEvent struct {
	ServerInstanceName string `json:"server_instance_name"`
	EventType          string `json:"event_type"`
	EventTimestamp     string `json:"event_timestamp"`
	EventDataXML       string `json:"event_data_xml"`
	ParsedPayloadJSON  string `json:"parsed_payload_json"`

	FileName   string `json:"file_name"`
	FileOffset int64  `json:"file_offset"`
}

// XeFileTargetState is persisted in SQLite so we can poll incrementally.
type XeFileTargetState struct {
	ServerInstanceName string
	LastFileName       *string
	LastOffset         *int64
}

