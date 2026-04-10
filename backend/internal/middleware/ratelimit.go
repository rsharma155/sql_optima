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
	windows   map[string][]time.Time
	maxPerMin int
}

func NewLoginRateLimiter(maxPerMin int) *LoginRateLimiter {
	if maxPerMin <= 0 {
		maxPerMin = 20
	}
	return &LoginRateLimiter{
		windows:   make(map[string][]time.Time),
		maxPerMin: maxPerMin,
	}
}

// Allow returns true if the request may proceed.
func (l *LoginRateLimiter) Allow(ip string) bool {
	now := time.Now()
	cutoff := now.Add(-time.Minute)

	l.mu.Lock()
	defer l.mu.Unlock()

	hits := l.windows[ip]
	var kept []time.Time
	for _, t := range hits {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.maxPerMin {
		l.windows[ip] = kept
		return false
	}
	kept = append(kept, now)
	l.windows[ip] = kept
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
