// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/iceberg.go
// Purpose: Lightweight client for registering exported Parquet files with an Apache Iceberg REST catalog.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package cold

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// IcebergRegistrar registers newly uploaded Parquet files with an Iceberg
// REST catalog (e.g. Project Nessie or iceberg-rest-catalog).
type IcebergRegistrar struct {
	catalogURL string
	namespace  string
	httpClient *http.Client
}

// NewIcebergRegistrar creates a new Iceberg catalog client.
func NewIcebergRegistrar(url, namespace string) *IcebergRegistrar {
	return &IcebergRegistrar{
		catalogURL: url,
		namespace:  namespace,
		httpClient: &http.Client{},
	}
}

// AppendDataFile registers a Parquet file as a new data file in the Iceberg table.
// This is a simplified implementation of the Iceberg REST API call.
func (r *IcebergRegistrar) AppendDataFile(ctx context.Context, tableName, s3Path string, rowCount int64) error {
	url := fmt.Sprintf("%s/v1/namespaces/%s/tables/%s/datafiles", r.catalogURL, r.namespace, tableName)

	payload := map[string]any{
		"content":      "DATA",
		"file-path":    s3Path,
		"file-format":  "PARQUET",
		"record-count": rowCount,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("iceberg/register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("iceberg/register: unexpected status %d", resp.StatusCode)
	}

	return nil
}
