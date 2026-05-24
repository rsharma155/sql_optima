// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/postgres_snapshots.go
// Purpose: Typed Parquet schemas for PostgreSQL settings and roles snapshots.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// PGSettingsSnapshotRow is the Parquet schema for postgres_settings_snapshot.
type PGSettingsSnapshotRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	Name               string `parquet:"name=name,                 type=BYTE_ARRAY, converted=STRING"`
	Setting            string `parquet:"name=setting,              type=BYTE_ARRAY, converted=STRING"`
	Unit               string `parquet:"name=unit,                 type=BYTE_ARRAY, converted=STRING"`
	Source             string `parquet:"name=source,               type=BYTE_ARRAY, converted=STRING"`
}

// PGRolesSnapshotRow is the Parquet schema for monitor.pg_roles_snapshot.
type PGRolesSnapshotRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	RolName            string `parquet:"name=rolname,              type=BYTE_ARRAY, converted=STRING"`
	RolSuper           bool   `parquet:"name=rolsuper,             type=BOOLEAN"`
	RolCreateDB        bool   `parquet:"name=rolcreatedb,          type=BOOLEAN"`
	RolCreateRole      bool   `parquet:"name=rolcreaterole,        type=BOOLEAN"`
	RolReplication     bool   `parquet:"name=rolreplication,       type=BOOLEAN"`
	RolCanLogin        bool   `parquet:"name=rolcanlogin,          type=BOOLEAN"`
}
