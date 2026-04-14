// Run this to reset the admin password
// Usage: go run reset_password.go
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Standalone utility for password reset functionality, likely CLI tool for administrator password management.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func main() {
	ctx := context.Background()

	// Connect to database
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		dbUser := getEnv("DB_USER", "dbmonitor")
		dbPassword := getEnv("DB_PASSWORD", "")
		tsHost := getEnv("TIMESCALEDB_HOST", "localhost")
		tsPort := getEnv("TIMESCALEDB_PORT", "5432")
		dbName := getEnv("DB_NAME", "dbmonitor_metrics")
		connStr = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			dbUser, dbPassword, tsHost, tsPort, dbName)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// New password: MUST be provided via env to avoid leaking credentials in source control.
	newPassword := os.Getenv("NEW_ADMIN_PASSWORD")
	if newPassword == "" {
		log.Fatalf("NEW_ADMIN_PASSWORD is required (refusing to use a hardcoded password)")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	// Update or insert admin user
	var userID int
	err = pool.QueryRow(ctx, `
		INSERT INTO optima_users (username, password_hash, role, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (username) DO UPDATE SET password_hash = $2, role = $3
		RETURNING user_id
	`, "admin", string(hash), "admin").Scan(&userID)

	if err != nil {
		log.Fatalf("Failed to update password: %v", err)
	}

	fmt.Printf("Password reset successfully! User ID: %d\n", userID)
	fmt.Println("New password: (hidden)")
}
