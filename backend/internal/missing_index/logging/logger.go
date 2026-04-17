// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Logging for missing index advisor operations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package logging

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var levelNames = map[Level]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
}

type Logger struct {
	output io.Writer
	level  Level
}

var defaultLogger *Logger

func init() {
	defaultLogger = New(os.Stdout, LevelInfo)
}

func New(output io.Writer, level Level) *Logger {
	return &Logger{
		output: output,
		level:  level,
	}
}

func (l *Logger) log(ctx context.Context, level Level, msg string, fields map[string]any) {
	if level < l.level {
		return
	}

	entry := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"level":     levelNames[level],
		"message":   msg,
	}

	if requestID := getRequestID(ctx); requestID != "" {
		entry["request_id"] = requestID
	}

	for k, v := range fields {
		entry[k] = v
	}

	data, _ := json.Marshal(entry)
	_, _ = l.output.Write(append(data, '\n'))
}

func (l *Logger) Debug(ctx context.Context, msg string, fields map[string]any) {
	l.log(ctx, LevelDebug, msg, fields)
}

func (l *Logger) Info(ctx context.Context, msg string, fields map[string]any) {
	l.log(ctx, LevelInfo, msg, fields)
}

func (l *Logger) Warn(ctx context.Context, msg string, fields map[string]any) {
	l.log(ctx, LevelWarn, msg, fields)
}

func (l *Logger) Error(ctx context.Context, msg string, fields map[string]any) {
	l.log(ctx, LevelError, msg, fields)
}

func getRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

type requestIDKeyType struct{}

var RequestIDKey = requestIDKeyType{}

func WithRequestID(ctx context.Context) context.Context {
	return context.WithValue(ctx, RequestIDKey, uuid.New().String())
}

func Debug(ctx context.Context, msg string, fields map[string]any) {
	defaultLogger.Debug(ctx, msg, fields)
}

func Info(ctx context.Context, msg string, fields map[string]any) {
	defaultLogger.Info(ctx, msg, fields)
}

func Warn(ctx context.Context, msg string, fields map[string]any) {
	defaultLogger.Warn(ctx, msg, fields)
}

func Error(ctx context.Context, msg string, fields map[string]any) {
	defaultLogger.Error(ctx, msg, fields)
}

func SetLevel(level Level) {
	defaultLogger.level = level
}

func SetOutput(output io.Writer) {
	defaultLogger.output = output
}

func SanitizeDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	parts := strings.Split(dsn, "@")
	if len(parts) > 1 {
		creds := strings.Split(parts[0], "://")
		if len(creds) > 1 {
			return strings.Replace(dsn, creds[1], "***", 1)
		}
	}
	return "***"
}

func SanitizeQuery(query string) string {
	if len(query) > 500 {
		return query[:500] + "..."
	}
	return query
}
