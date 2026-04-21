// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Generic retry wrapper with exponential backoff for transient failures.
//
//	Uses cenkalti/backoff for jittered exponential back-off and supports
//	context cancellation, max retries, and structured logging via slog.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package reliability

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// RetryConfig controls the retry behaviour.
type RetryConfig struct {
	MaxRetries      int           // 0 means no retries (run once)
	InitialInterval time.Duration // first back-off delay
	MaxElapsed      time.Duration // hard wall-clock cap
}

// DefaultRetryConfig returns a sensible default: 3 retries, 1 s initial, 30 s max elapsed.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      3,
		InitialInterval: 1 * time.Second,
		MaxElapsed:      30 * time.Second,
	}
}

// Do executes fn with exponential back-off retries. The operation string is
// used for structured log messages. Returns the last error if all attempts fail.
func Do(ctx context.Context, cfg RetryConfig, operation string, fn func() error) error {
	if cfg.MaxRetries <= 0 {
		return fn()
	}

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = cfg.InitialInterval
	if cfg.MaxElapsed > 0 {
		bo.MaxElapsedTime = cfg.MaxElapsed
	}

	bCtx := backoff.WithContext(bo, ctx)
	wrapped := backoff.WithMaxRetries(bCtx, uint64(cfg.MaxRetries))

	attempt := 0
	err := backoff.Retry(func() error {
		attempt++
		if err := fn(); err != nil {
			slog.Warn("retry_attempt",
				"operation", operation,
				"attempt", attempt,
				"error", err,
			)
			return err
		}
		return nil
	}, wrapped)

	if err != nil {
		return fmt.Errorf("%s failed after %d attempt(s): %w", operation, attempt, err)
	}
	return nil
}

// DoWithResult executes fn and returns both the result and error with retry logic.
func DoWithResult[T any](ctx context.Context, cfg RetryConfig, operation string, fn func() (T, error)) (T, error) {
	var result T
	err := Do(ctx, cfg, operation, func() error {
		var fnErr error
		result, fnErr = fn()
		return fnErr
	})
	return result, err
}
