// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Structured logging singleton built on Go's log/slog with JSON output,
//
//	configurable level (via LOG_LEVEL env), and automatic credential redaction.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/rsharma155/sql_optima/internal/security/redact"
)

var (
	once     sync.Once
	instance *slog.Logger
)

// Init initialises the global slog.Logger with a JSON handler writing to the
// given writer. Pass nil to default to os.Stdout. The level is read from the
// LOG_LEVEL environment variable (debug, info, warn, error); defaults to info.
// Credential redaction is applied transparently to all string attribute values.
func Init(w io.Writer) *slog.Logger {
	once.Do(func() {
		if w == nil {
			w = os.Stdout
		}
		level := parseLevel(os.Getenv("LOG_LEVEL"))
		handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
			Level: level,
		})
		instance = slog.New(&redactHandler{inner: handler})
		slog.SetDefault(instance)
	})
	return instance
}

// Get returns the global logger, initialising it with defaults if necessary.
func Get() *slog.Logger {
	if instance == nil {
		Init(nil)
	}
	return instance
}

// With returns a child logger with the given key-value attributes.
func With(args ...any) *slog.Logger {
	return Get().With(args...)
}

// parseLevel converts a human-readable level string to slog.Level.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// redactHandler wraps a slog.Handler and scrubs credential patterns from string values.
type redactHandler struct {
	inner slog.Handler
}

func (h *redactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *redactHandler) Handle(ctx context.Context, r slog.Record) error {
	r.Message = redact.String(r.Message)
	cleaned := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		cleaned.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, cleaned)
}

func (h *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = redactAttr(a)
	}
	return &redactHandler{inner: h.inner.WithAttrs(redacted)}
}

func (h *redactHandler) WithGroup(name string) slog.Handler {
	return &redactHandler{inner: h.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindString {
		a.Value = slog.StringValue(redact.String(a.Value.String()))
	}
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		out := make([]slog.Attr, len(attrs))
		for i, ga := range attrs {
			out[i] = redactAttr(ga)
		}
		a.Value = slog.GroupValue(out...)
	}
	return a
}

// Reset clears the singleton — intended for testing only.
func Reset() {
	once = sync.Once{}
	instance = nil
}
