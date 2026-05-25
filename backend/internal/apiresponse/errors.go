// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
//
// Purpose: Safe JSON error responses for API handlers — log full errors server-side,
// return generic messages to clients (P2-4 error leakage mitigation).

package apiresponse

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// WriteJSONError logs err with attrs and writes {"error": publicMsg} to the client.
func WriteJSONError(w http.ResponseWriter, status int, publicMsg string, err error, attrs ...any) {
	if err != nil {
		attrs = append(attrs, "err", err)
	}
	slog.Error(publicMsg, attrs...)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": publicMsg})
}

// WritePlainError logs err and writes a plain-text body (for legacy handlers using http.Error).
func WritePlainError(w http.ResponseWriter, status int, publicMsg string, err error, attrs ...any) {
	if err != nil {
		attrs = append(attrs, "err", err)
	}
	slog.Error(publicMsg, attrs...)
	http.Error(w, publicMsg, status)
}
