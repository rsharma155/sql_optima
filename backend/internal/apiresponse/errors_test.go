// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Tests for client-safe API error responses (no internal err.Error leakage).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package apiresponse_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rsharma155/sql_optima/internal/apiresponse"
)

func TestWriteJSONError_DoesNotLeakInternalError(t *testing.T) {
	rr := httptest.NewRecorder()
	internal := errors.New("pq: password authentication failed for user sql_optima")
	apiresponse.WriteJSONError(rr, http.StatusInternalServerError, "failed to load data", internal, "handler", "Test")

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "password") || strings.Contains(body, "sql_optima") || strings.Contains(body, "pq:") {
		t.Fatalf("client body leaked internal error: %s", body)
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got["error"] != "failed to load data" {
		t.Fatalf("error = %q, want public message", got["error"])
	}
}

func TestWritePlainError_DoesNotLeakInternalError(t *testing.T) {
	rr := httptest.NewRecorder()
	internal := errors.New("connection to host=db.internal failed: dial tcp 10.0.0.1:5432")
	apiresponse.WritePlainError(rr, http.StatusBadGateway, "upstream unavailable", internal)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "10.0.0.1") || strings.Contains(body, "db.internal") {
		t.Fatalf("client body leaked internal error: %s", body)
	}
	if !strings.Contains(body, "upstream unavailable") {
		t.Fatalf("missing public message: %s", body)
	}
}

func TestWriteJSONError_NilErrStillWritesPublicMessage(t *testing.T) {
	rr := httptest.NewRecorder()
	apiresponse.WriteJSONError(rr, http.StatusBadRequest, "invalid instance name", nil)
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["error"] != "invalid instance name" {
		t.Fatalf("got %q", got["error"])
	}
}
