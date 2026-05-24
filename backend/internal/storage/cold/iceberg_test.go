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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIcebergRegistrar_AppendDataFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/namespaces/default/tables/test_table/datafiles", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	registrar := &IcebergRegistrar{
		catalogURL: server.URL,
		namespace:  "default",
		httpClient: server.Client(),
	}

	err := registrar.AppendDataFile(context.Background(), "test_table", "s3://bucket/path.parquet", 100)
	assert.NoError(t, err)
}
