package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"runtime/debug"
	"time"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

// RequestIDMiddleware assigns X-Request-ID (or propagates incoming) and attaches it to request context.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-ID")
		if rid == "" {
			rid = newRequestID()
		}
		w.Header().Set("X-Request-ID", rid)
		ctx := context.WithValue(r.Context(), requestIDKey, rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext returns the request id if present.
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// AccessLogMiddleware logs one line per request with duration (structured via slog).
func AccessLogMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	// Set ACCESS_LOG=0 to disable request logs (useful in dev with polling UIs).
	accessLogEnabled := true
	if v := strings.TrimSpace(os.Getenv("ACCESS_LOG")); v != "" {
		if v == "0" || strings.EqualFold(v, "false") {
			accessLogEnabled = false
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			if err := recover(); err != nil {
				log.Error("panic recovered in request", "error", err, "stack", string(debug.Stack()))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(sw, r)
		if accessLogEnabled {
			rid := RequestIDFromContext(r.Context())
			log.Info("http_request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", rid),
			)
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	return s.ResponseWriter.Write(b)
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
