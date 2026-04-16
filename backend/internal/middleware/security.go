// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Security headers middleware adding X-Frame-Options, X-Content-Type-Options, CSP, HSTS, and Referrer-Policy.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package middleware

import (
	"net/http"
	"os"
	"strings"
)

// SecurityHeadersMiddleware add security headers to all responses
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	// HSTS is only meaningful when the server is running behind TLS.
	// Honour an opt-out for local HTTP development via HSTS_DISABLED=1.
	hstsDisabled := strings.TrimSpace(os.Getenv("HSTS_DISABLED")) == "1"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Enable XSS protection (legacy browsers)
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Prevent referrer information leakage
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// HTTP Strict Transport Security — 1 year, include subdomains.
		// Requires a TLS reverse proxy (nginx/Caddy) in front of this server.
		if !hstsDisabled {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Content Security Policy.
		// 'unsafe-inline' is removed from script-src: all JS is loaded via <script src> or ES modules.
		// style-src retains 'unsafe-inline' because page modules use inline style attributes.
		// connect-src allows cdn.jsdelivr.net only for Chart.js CDN fetch (auto-upgrade check).
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' https://cdn.jsdelivr.net https://cdnjs.cloudflare.com; "+
				"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdnjs.cloudflare.com; "+
				"font-src 'self' https://fonts.gstatic.com https://cdnjs.cloudflare.com; "+
				"img-src 'self' data: https:; "+
				"connect-src 'self' https://cdn.jsdelivr.net")

		next.ServeHTTP(w, r)
	})
}
