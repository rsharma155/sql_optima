// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: CLI tool for managing database migrations via goose.
//
// Usage:
//
//	go run ./cmd/migrate -dir migrations status
//	go run ./cmd/migrate -dir migrations up
//	go run ./cmd/migrate -dir migrations down
//	go run ./cmd/migrate -dir migrations create add_new_table sql
//
// Environment:
//
//	GOOSE_DBSTRING  — PostgreSQL connection string (required)
//	GOOSE_DRIVER    — defaults to "postgres"
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx stdlib driver for database/sql
	"github.com/pressly/goose/v3"
)

func main() {
	dir := flag.String("dir", "migrations", "directory containing migration files")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: migrate [-dir DIR] COMMAND [ARGS]")
		fmt.Fprintln(os.Stderr, "commands: up, down, status, create, redo, version, up-to, down-to")
		os.Exit(1)
	}

	driver := os.Getenv("GOOSE_DRIVER")
	if driver == "" {
		driver = "postgres"
	}

	dsn := os.Getenv("GOOSE_DBSTRING")
	if dsn == "" {
		dsn = os.Getenv("TIMESCALE_DSN") // fall back to the app's connection string
	}
	if dsn == "" {
		log.Fatal("GOOSE_DBSTRING (or TIMESCALE_DSN) must be set to a PostgreSQL connection string")
	}

	// Use the pgx stdlib driver registered as "pgx".
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(context.Background()); err != nil {
		log.Fatalf("database unreachable: %v", err)
	}

	goose.SetDialect("postgres")

	command := strings.ToLower(args[0])
	cmdArgs := args[1:]

	if err := goose.RunContext(context.Background(), command, db, *dir, cmdArgs...); err != nil {
		log.Fatalf("goose %s: %v", command, err)
	}
}
