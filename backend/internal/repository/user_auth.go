// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: User authentication repository for TimescaleDB-backed user management.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var authErrorLog *log.Logger

func init() {
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("[UserAuth] Warning: Failed to create log directory: %v", err)
		return
	}

	errorLogPath := filepath.Join(logDir, "auth_error.log")
	file, err := os.OpenFile(errorLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[UserAuth] Warning: Failed to open auth error log file: %v", err)
		return
	}

	authErrorLog = log.New(file, "[AUTH_ERROR] ", log.Ldate|log.Ltime|log.Lshortfile)
	log.Printf("[UserAuth] Auth error log file: %s", errorLogPath)
}

// UserRepository handles user authentication and management.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new user repository.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// AuthenticateUser verifies username/password and returns user info if valid.
func (r *UserRepository) AuthenticateUser(ctx context.Context, username, password string) (*User, error) {
	query := `SELECT user_id, username, password_hash, role, created_at FROM optima_users WHERE username = $1`
	var u User
	var hash string
	var createdAt time.Time
	err := r.pool.QueryRow(ctx, query, username).Scan(&u.UserID, &u.Username, &hash, &u.Role, &createdAt)
	if err != nil {
		errMsg := fmt.Sprintf("Login failed for user '%s': user not found in database. Time: %s, Error: %v", username, time.Now().Format(time.RFC3339), err)
		log.Printf("[UserAuth] %s", errMsg)
		if authErrorLog != nil {
			authErrorLog.Printf("%s", errMsg)
		}
		return nil, fmt.Errorf("invalid credentials")
	}
	u.CreatedAt = createdAt

	log.Printf("[UserAuth] Attempting login for user '%s'", username)
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		errMsg := fmt.Sprintf("Password mismatch for user '%s'. Time: %s, Error: %v", username, time.Now().Format(time.RFC3339), err)
		log.Printf("[UserAuth] %s", errMsg)
		if authErrorLog != nil {
			authErrorLog.Printf("%s", errMsg)
		}
		return nil, fmt.Errorf("invalid credentials")
	}

	log.Printf("[UserAuth] Login successful for user '%s' (role: %s)", username, u.Role)
	return &u, nil
}

// CreateUser creates a new user with a bcrypt-hashed password.
func (r *UserRepository) CreateUser(ctx context.Context, username, password, role string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	query := `INSERT INTO optima_users (username, password_hash, role) VALUES ($1, $2, $3) RETURNING user_id, username, role, created_at`
	var u User
	err = r.pool.QueryRow(ctx, query, username, string(hash), role).Scan(&u.UserID, &u.Username, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	log.Printf("[UserRepo] Created user %s with role %s", username, role)
	return &u, nil
}

// CountUsers returns the number of rows in optima_users.
func (r *UserRepository) CountUsers(ctx context.Context) (int, error) {
	if r == nil || r.pool == nil {
		return 0, fmt.Errorf("user repository not configured")
	}
	var n int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM optima_users`).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// GetAllUsers returns all users (without password hashes).
func (r *UserRepository) GetAllUsers(ctx context.Context) ([]User, error) {
	query := `SELECT user_id, username, role, created_at FROM optima_users ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.UserID, &u.Username, &u.Role, &u.CreatedAt); err != nil {
			log.Printf("[UserRepo] Scan error: %v", err)
			continue
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// DeleteUser removes a user by ID.
func (r *UserRepository) DeleteUser(ctx context.Context, userID int) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM optima_users WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// UpdateUserRole changes a user's role.
func (r *UserRepository) UpdateUserRole(ctx context.Context, userID int, role string) error {
	result, err := r.pool.Exec(ctx, `UPDATE optima_users SET role = $1 WHERE user_id = $2`, role, userID)
	if err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// User represents a user in the optima_users table.
type User struct {
	UserID    int       `json:"user_id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}
