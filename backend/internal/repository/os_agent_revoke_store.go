// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Persist and query revoked OS-agent JWT ids (jti) for instant token kill.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OSAgentRevokeStore backs middleware.OSAgentRevokeChecker with TimescaleDB + a small memory cache.
type OSAgentRevokeStore struct {
	pool *pgxpool.Pool

	mu    sync.RWMutex
	cache map[string]time.Time // jti → expires_at (UTC); positive hits only
}

// NewOSAgentRevokeStore creates a revoke store. pool must be non-nil.
func NewOSAgentRevokeStore(pool *pgxpool.Pool) *OSAgentRevokeStore {
	return &OSAgentRevokeStore{
		pool:  pool,
		cache: make(map[string]time.Time),
	}
}

// IsRevoked reports whether jti is on the revoke list and not yet past expires_at.
func (s *OSAgentRevokeStore) IsRevoked(ctx context.Context, jti string) (bool, error) {
	if s == nil || s.pool == nil || jti == "" {
		return false, nil
	}
	now := time.Now().UTC()

	s.mu.RLock()
	if exp, ok := s.cache[jti]; ok {
		s.mu.RUnlock()
		return exp.After(now), nil
	}
	s.mu.RUnlock()

	var expiresAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT expires_at
		FROM optima_os_agent_revoked_tokens
		WHERE jti = $1 AND expires_at > now()
	`, jti).Scan(&expiresAt)
	if err != nil {
		// No rows → not revoked. Other errors propagate (fail closed in middleware).
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	s.mu.Lock()
	s.cache[jti] = expiresAt.UTC()
	s.mu.Unlock()
	return true, nil
}

// Revoke inserts (or upserts) a jti until expiresAt. serverID may be uuid.Nil.
func (s *OSAgentRevokeStore) Revoke(ctx context.Context, jti string, serverID uuid.UUID, actor, reason string, expiresAt time.Time) error {
	if s == nil || s.pool == nil || jti == "" {
		return nil
	}
	if expiresAt.IsZero() || !expiresAt.After(time.Now().UTC()) {
		expiresAt = time.Now().UTC().Add(24 * time.Hour)
	}
	var sid *uuid.UUID
	if serverID != uuid.Nil {
		sid = &serverID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO optima_os_agent_revoked_tokens (jti, server_id, revoked_by, expires_at, reason)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))
		ON CONFLICT (jti) DO UPDATE SET
			revoked_at = now(),
			revoked_by = EXCLUDED.revoked_by,
			expires_at = GREATEST(optima_os_agent_revoked_tokens.expires_at, EXCLUDED.expires_at),
			reason = COALESCE(EXCLUDED.reason, optima_os_agent_revoked_tokens.reason)
	`, jti, sid, nullIfEmpty(actor), expiresAt.UTC(), reason)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cache[jti] = expiresAt.UTC()
	s.mu.Unlock()
	return nil
}

// PruneExpired deletes revoke rows past expires_at (optional maintenance).
func (s *OSAgentRevokeStore) PruneExpired(ctx context.Context) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, nil
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM optima_os_agent_revoked_tokens WHERE expires_at <= now()`)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	now := time.Now().UTC()
	for jti, exp := range s.cache {
		if !exp.After(now) {
			delete(s.cache, jti)
		}
	}
	s.mu.Unlock()
	return tag.RowsAffected(), nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
