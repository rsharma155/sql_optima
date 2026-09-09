// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/iceberg.go
// Purpose: Lightweight client for registering exported Parquet files with an Apache Iceberg
//          REST catalog (e.g. Project Nessie or tabular/iceberg-rest-catalog).
//
//          Phase 2 implementation notes:
//          - EnsureNamespace and EnsureTable use the standard Iceberg REST spec endpoints.
//          - AppendDataFile uses CommitTableRequest (POST /v1/{ns}/tables/{table}) which is
//            the correct endpoint per the Apache Iceberg REST catalog spec.
//          - Full snapshot commits require Avro manifest files in S3. Until
//            github.com/apache/iceberg-go reaches stable, AppendDataFile registers a
//            snapshot summary without manifest file references. This is accepted by some
//            catalog implementations (Nessie ≥0.67 in "relaxed" mode) but will fail on
//            strict Iceberg validators. When a full Go Iceberg client is available, replace
//            the snapshot body with a proper manifest-list-backed commit.
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
	"io"
	"math/rand"
	"net/http"
	"time"
)

// IcebergRegistrar registers newly uploaded Parquet files with an Iceberg REST catalog.
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
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// EnsureNamespace creates the catalog namespace if it does not already exist.
// A 409 Conflict response is treated as success (namespace already exists).
func (r *IcebergRegistrar) EnsureNamespace(ctx context.Context) error {
	url := fmt.Sprintf("%s/v1/namespaces", r.catalogURL)

	payload := map[string]any{
		"namespace":  []string{r.namespace},
		"properties": map[string]string{},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("iceberg/namespace: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("iceberg/namespace: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("iceberg/namespace: %w", err)
	}
	defer resp.Body.Close()

	// 200 = created, 409 = already exists — both are acceptable.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("iceberg/namespace: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// EnsureTable creates the Iceberg table in the catalog if it does not yet exist.
// A 409 Conflict is treated as success. The schema is minimal — Iceberg will
// evolve it as additional Parquet files are registered.
func (r *IcebergRegistrar) EnsureTable(ctx context.Context, tableName string) error {
	// Check existence first to avoid sending a create on every export.
	checkURL := fmt.Sprintf("%s/v1/namespaces/%s/tables/%s", r.catalogURL, r.namespace, tableName)
	checkReq, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
	if err != nil {
		return fmt.Errorf("iceberg/table: check: %w", err)
	}
	checkResp, err := r.httpClient.Do(checkReq)
	if err != nil {
		return fmt.Errorf("iceberg/table: check: %w", err)
	}
	_, _ = io.Copy(io.Discard, checkResp.Body)
	checkResp.Body.Close()
	if checkResp.StatusCode == http.StatusOK {
		return nil // table already exists
	}

	// Table not found — create it with a minimal schema.
	createURL := fmt.Sprintf("%s/v1/namespaces/%s/tables", r.catalogURL, r.namespace)

	payload := map[string]any{
		"name": tableName,
		"schema": map[string]any{
			"type":   "struct",
			"fields": []any{}, // schema-on-read: actual columns come from Parquet files
		},
		"partition-spec": map[string]any{"fields": []any{}},
		"write-order":    map[string]any{"fields": []any{}},
		"stage-create":   false,
		"properties": map[string]string{
			"write.format.default": "parquet",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("iceberg/table: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("iceberg/table: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("iceberg/table: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("iceberg/table: create status %d", resp.StatusCode)
	}
	return nil
}

// AppendDataFile registers a Parquet file as a new snapshot in the Iceberg catalog.
//
// This uses the correct Iceberg REST endpoint:
//
//	POST /v1/namespaces/{ns}/tables/{table}     (CommitTableRequest)
//
// Full Iceberg compliance requires Avro manifest files. Until the Go Iceberg library
// (apache/iceberg-go) is stable, the snapshot is committed without manifest-list
// references. Some catalog implementations (Nessie ≥0.67) accept this; strict validators
// will reject it. The exporter logs a warning rather than failing the export on error.
func (r *IcebergRegistrar) AppendDataFile(ctx context.Context, tableName, s3Path string, rowCount int64) error {
	if err := r.EnsureNamespace(ctx); err != nil {
		return fmt.Errorf("iceberg: ensure namespace: %w", err)
	}
	if err := r.EnsureTable(ctx, tableName); err != nil {
		return fmt.Errorf("iceberg: ensure table: %w", err)
	}

	url := fmt.Sprintf("%s/v1/namespaces/%s/tables/%s", r.catalogURL, r.namespace, tableName)

	snapshotID := rand.Int63() //nolint:gosec // snapshot IDs do not need cryptographic randomness
	payload := map[string]any{
		"identifier": map[string]any{
			"namespace": []string{r.namespace},
			"name":      tableName,
		},
		"requirements": []map[string]any{},
		"updates": []map[string]any{
			{
				"action": "add-snapshot",
				"snapshot": map[string]any{
					"snapshot-id":   snapshotID,
					"timestamp-ms":  time.Now().UnixMilli(),
					"manifest-list": "", // placeholder — full manifests require apache/iceberg-go
					"summary": map[string]string{
						"operation":        "append",
						"added-data-files": "1",
						"added-records":    fmt.Sprintf("%d", rowCount),
						"added-files-size": "0",
						"total-data-files": "1",
						"total-records":    fmt.Sprintf("%d", rowCount),
					},
				},
			},
			{
				"action":      "set-snapshot-ref",
				"ref-name":    "main",
				"type":        "branch",
				"snapshot-id": snapshotID,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("iceberg/append: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("iceberg/append: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("iceberg/append: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("iceberg/append: catalog returned %d for %s", resp.StatusCode, s3Path)
	}
	return nil
}
