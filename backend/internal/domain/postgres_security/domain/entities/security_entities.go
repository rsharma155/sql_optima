// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Security domain entities.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package entities

import "github.com/google/uuid"

import "time"

// RoleSnapshot represents a snapshot of database roles and privileges.
type RoleSnapshot struct {
	TS             time.Time `json:"capture_timestamp"`
	ServerID       uuid.UUID `json:"server_id" db:"server_id"`
	Rolname        string    `json:"rolname"`
	Rolsuper       bool      `json:"rolsuper"`
	Rolcreatedb    bool      `json:"rolcreatedb"`
	Rolcreaterole  bool      `json:"rolcreaterole"`
	Rolreplication bool      `json:"rolreplication"`
	Rolcanlogin    bool      `json:"rolcanlogin"`
}

// FailedLoginEvent represents a failed login attempt.
type FailedLoginEvent struct {
	TS         time.Time `json:"capture_timestamp"`
	ServerID   uuid.UUID `json:"server_id" db:"server_id"`
	Username   string    `json:"username"`
	ClientAddr string    `json:"client_addr"`
	Message    string    `json:"message"`
}

// DDLActivity represents DDL activity snapshot.
type DDLActivity struct {
	TS         time.Time `json:"capture_timestamp"`
	ServerID   uuid.UUID `json:"server_id" db:"server_id"`
	Schemaname string    `json:"schemaname"`
	Relname    string    `json:"relname"`
	NTupIns    int64     `json:"n_tup_ins"`
	NTupUpd    int64     `json:"n_tup_upd"`
	NTupDel    int64     `json:"n_tup_del"`
}
