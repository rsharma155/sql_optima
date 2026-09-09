// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/iceberg_test.go
// Purpose: Unit tests for the Iceberg registrar, mocking the REST catalog API.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package cold

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIcebergRegistrar_AppendDataFile(t *testing.T) {
	calls := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.Method+" "+r.URL.Path]++

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/namespaces":
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/namespaces/default/tables/"):
			// Table does not exist yet → 404 triggers create
			w.WriteHeader(http.StatusNotFound)

		case r.Method == http.MethodPost && r.URL.Path == "/v1/namespaces/default/tables":
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodPost && r.URL.Path == "/v1/namespaces/default/tables/test_table":
			// CommitTableRequest — append snapshot
			w.WriteHeader(http.StatusOK)

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	registrar := &IcebergRegistrar{
		catalogURL: server.URL,
		namespace:  "default",
		httpClient: server.Client(),
	}

	err := registrar.AppendDataFile(context.Background(), "test_table", "s3://bucket/path.parquet", 100)
	assert.NoError(t, err)

	assert.Equal(t, 1, calls["POST /v1/namespaces"], "should create namespace")
	assert.Equal(t, 1, calls["GET /v1/namespaces/default/tables/test_table"], "should check table existence")
	assert.Equal(t, 1, calls["POST /v1/namespaces/default/tables"], "should create table")
	assert.Equal(t, 1, calls["POST /v1/namespaces/default/tables/test_table"], "should commit snapshot")
}

func TestIcebergRegistrar_AppendDataFile_TableAlreadyExists(t *testing.T) {
	calls := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.Method+" "+r.URL.Path]++
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/namespaces":
			w.WriteHeader(http.StatusConflict) // namespace already exists
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/namespaces/default/tables/"):
			w.WriteHeader(http.StatusOK) // table already exists
		case r.Method == http.MethodPost && r.URL.Path == "/v1/namespaces/default/tables/metrics":
			w.WriteHeader(http.StatusOK) // commit snapshot
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	registrar := &IcebergRegistrar{
		catalogURL: server.URL,
		namespace:  "default",
		httpClient: server.Client(),
	}

	err := registrar.AppendDataFile(context.Background(), "metrics", "s3://bucket/metrics.parquet", 500)
	assert.NoError(t, err)

	assert.Equal(t, 0, calls["POST /v1/namespaces/default/tables"], "should skip table create when table exists")
	assert.Equal(t, 1, calls["POST /v1/namespaces/default/tables/metrics"], "should still commit snapshot")
}
