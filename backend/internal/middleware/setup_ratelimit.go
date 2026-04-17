package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SetupRateLimiter limits anonymous /api/setup/* POST abuse per client IP.
type SetupRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

func NewSetupRateLimiter(maxPerWindow int, window time.Duration) *SetupRateLimiter {
	if maxPerWindow <= 0 {
		maxPerWindow = 15
	}
	if window <= 0 {
		window = time.Minute
	}
	return &SetupRateLimiter{
		attempts: make(map[string][]time.Time),
		max:      maxPerWindow,
		window:   window,
	}
}

// TrustProxy controls whether clientIPKey honours X-Forwarded-For.
// Set to true only when the app sits behind a trusted reverse proxy
// that overwrites the header (e.g. nginx, AWS ALB).
var TrustProxy bool

func clientIPKey(r *http.Request) string {
	if TrustProxy {
		if xf := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xf != "" {
			parts := strings.Split(xf, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (l *SetupRateLimiter) Allow(ip string) bool {
	if l == nil {
		return true
	}
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	ts := l.attempts[ip]
	var kept []time.Time
	for _, t := range ts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.attempts, ip)
	}
	if len(kept) >= l.max {
		l.attempts[ip] = kept
		return false
	}
	kept = append(kept, now)
	l.attempts[ip] = kept
	return true
}

// SetupRateLimitMiddleware applies POST rate limiting for setup endpoints.
func SetupRateLimitMiddleware(l *SetupRateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l == nil || r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIPKey(r)
		if !l.Allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "too many setup attempts; try again later"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
