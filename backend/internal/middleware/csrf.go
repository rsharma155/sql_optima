// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: CSRF protection middleware using the Double Submit Cookie pattern.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
)

const (
	// CSRFCookieName is the name of the cookie that holds the CSRF token.
	// This cookie is NOT HttpOnly so that JavaScript can read it and send it
	// back as a header on mutating requests.
	CSRFCookieName = "csrf_token"

	// CSRFHeaderName is the header the client must send with the cookie value.
	CSRFHeaderName = "X-CSRF-Token"
)

// GenerateCSRFToken returns a cryptographically random URL-safe string (32 bytes / 43 chars).
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SetCSRFCookie writes the CSRF cookie. It is readable by JS (not HttpOnly).
func SetCSRFCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // JS must be able to read this
		Secure:   isTLS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
}

// CSRFProtect is middleware that enforces CSRF validation on mutating HTTP methods
// (POST, PUT, PATCH, DELETE) when the request is authenticated via cookie
// (i.e. no Authorization header present). Requests that carry a Bearer token
// in the Authorization header are exempt because those cannot be triggered by
// a cross-origin form submission or link.
func CSRFProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Safe methods are exempt.
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}

		// If the request carries a Bearer token, it is an API client (curl,
		// SDK, Postman). CSRF is a browser-only attack vector, so skip.
		if r.Header.Get("Authorization") != "" {
			next.ServeHTTP(w, r)
			return
		}

		// Cookie-based auth → enforce CSRF double-submit check.
		cookie, err := r.Cookie(CSRFCookieName)
		if err != nil || cookie.Value == "" {
			csrfError(w, "missing CSRF cookie")
			return
		}

		header := r.Header.Get(CSRFHeaderName)
		if header == "" {
			csrfError(w, "missing "+CSRFHeaderName+" header")
			return
		}

		if subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) != 1 {
			csrfError(w, "CSRF token mismatch")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func csrfError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// isTLS returns true when the request was served over TLS or is behind a
// TLS-terminating reverse proxy that sets X-Forwarded-Proto.
func isTLS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}
