// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Cross-engine Storage & Index Health DTOs (Timescale hypertables; engine = sqlserver | postgres). PostgreSQL ingestion uses collectors in pg_* source files.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package models

import "time"

type IndexUsageStat struct {
	Time        time.Time `json:"time"`
	Engine      string    `json:"engine"` // sqlserver | postgres
	ServerID    string    `json:"server_id"`
	DBName      string    `json:"db_name"`
	SchemaName  string    `json:"schema_name"`
	TableName   string    `json:"table_name"`
	IndexName   string    `json:"index_name"`
	Seeks       int64     `json:"seeks"`
	Scans       int64     `json:"scans"`
	Lookups     int64     `json:"lookups"`
	Updates     int64     `json:"updates"`
	IndexSizeMB float64   `json:"index_size_mb"`
	IsUnique    bool      `json:"is_unique"`
	IsPK        bool      `json:"is_pk"`
	FillFactor  int       `json:"fillfactor"`
	LastUserSeek   *time.Time `json:"last_user_seek,omitempty"`
	LastUserScan   *time.Time `json:"last_user_scan,omitempty"`
	LastUserLookup *time.Time `json:"last_user_lookup,omitempty"`
}

type TableUsageStat struct {
	Time         time.Time `json:"time"`
	Engine       string    `json:"engine"` // sqlserver | postgres
	ServerID     string    `json:"server_id"`
	DBName       string    `json:"db_name"`
	SchemaName   string    `json:"schema_name"`
	TableName    string    `json:"table_name"`
	SeqScans     int64     `json:"seq_scans"`
	IdxScans     int64     `json:"idx_scans"`
	RowsRead     int64     `json:"rows_read"`
	RowsModified int64     `json:"rows_modified"`
	TableSizeMB  float64   `json:"table_size_mb"`
	IndexSizeMB  float64   `json:"index_size_mb"`
	RowCount     int64     `json:"row_count"`
}

type TableSizeHistory struct {
	Time        time.Time `json:"time"`
	Engine      string    `json:"engine"` // sqlserver | postgres
	ServerID    string    `json:"server_id"`
	DBName      string    `json:"db_name"`
	SchemaName  string    `json:"schema_name"`
	TableName   string    `json:"table_name"`
	TableSizeMB float64   `json:"table_size_mb"`
	IndexSizeMB float64   `json:"index_size_mb"`
	RowCount    int64     `json:"row_count"`
}

type IndexDefinition struct {
	Time            time.Time `json:"time"`
	Engine          string    `json:"engine"`
	ServerID        string    `json:"server_id"`
	DBName          string    `json:"db_name"`
	SchemaName      string    `json:"schema_name"`
	TableName       string    `json:"table_name"`
	IndexName       string    `json:"index_name"`
	KeyColumns      string    `json:"key_columns"`
	IncludeColumns  string    `json:"include_columns"`
	FilterDefinition string   `json:"filter_definition"`
	IsUnique        bool      `json:"is_unique"`
	IsPK            bool      `json:"is_pk"`
	IndexType       string    `json:"index_type"`
}

