// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Rate limiting middleware for login endpoint protection against brute force attacks.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

var trustedProxyIPs = []string{
	"127.0.0.1",
	"::1",
	"::ffff:127.0.0.1",
}

func isTrustedProxy(ip string) bool {
	for _, trusted := range trustedProxyIPs {
		if ip == trusted {
			return true
		}
	}
	return false
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			remoteIP = r.RemoteAddr
		}
		if isTrustedProxy(remoteIP) {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type LoginRateLimiter struct {
	mu        sync.Mutex
	windows   map[string][]time.Time // keyed by IP or "user:<username>"
	maxPerMin int
	// maxPerMinUser is a tighter per-username limit (default: maxPerMin/4, min 5).
	// Even if the attacker rotates IPs, each target account gets at most
	// maxPerMinUser attempts per minute.
	maxPerMinUser int
}

func NewLoginRateLimiter(maxPerMin int) *LoginRateLimiter {
	if maxPerMin <= 0 {
		maxPerMin = 20
	}
	perUser := maxPerMin / 4
	if perUser < 5 {
		perUser = 5
	}
	return &LoginRateLimiter{
		windows:       make(map[string][]time.Time),
		maxPerMin:     maxPerMin,
		maxPerMinUser: perUser,
	}
}

// Allow returns true if the IP has not exceeded the per-IP limit.
func (l *LoginRateLimiter) Allow(ip string) bool {
	return l.allow(ip, l.maxPerMin)
}

// AllowUsername returns true if the given (lowercased) username has not
// exceeded the tighter per-user limit. Call this after decoding the request
// body so the username is available, but before calling AuthenticateUser.
func (l *LoginRateLimiter) AllowUsername(username string) bool {
	if username == "" {
		return true // blank username checked elsewhere
	}
	return l.allow("user:"+strings.ToLower(username), l.maxPerMinUser)
}

func (l *LoginRateLimiter) allow(key string, limit int) bool {
	now := time.Now()
	cutoff := now.Add(-time.Minute)

	l.mu.Lock()
	defer l.mu.Unlock()

	hits := l.windows[key]
	var kept []time.Time
	for _, t := range hits {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		l.windows[key] = kept
		return false
	}
	kept = append(kept, now)
	l.windows[key] = kept
	return true
}

// LoginRateLimitMiddleware wraps a handler for POST-only rate limiting.
func LoginRateLimitMiddleware(l *LoginRateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !l.Allow(clientIP(r)) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}
		next(w, r)
	}
}
