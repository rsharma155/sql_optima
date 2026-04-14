// Package advisor exposes index recommendations and query rewrites for embedding in other Go modules.
// It wraps the internal engine without leaking internal packages across module boundaries.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Missing index advisor core logic for index recommendation generation.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package advisor

import (
	"context"

	intapi "github.com/rsharma155/sql_optima/internal/missing_index/api"
	"github.com/rsharma155/sql_optima/internal/missing_index/service"
	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

// Client runs the missing-index advisor against SQL text and a JSON execution plan.
type Client struct {
	svc *service.Service
}

// New creates a Client. Safe for concurrent use via the underlying service.
func New() *Client {
	return &Client{svc: service.New()}
}

// Options tune candidate generation and verification (nil = service defaults).
type Options struct {
	MaxCandidates      int
	MinImprovementPct  float64
	StatementTimeoutMs int
	IncludeColumns     *bool
}

// Analyze runs the full pipeline and returns the same JSON shape as POST /v1/recommend-index.
func (c *Client) Analyze(ctx context.Context, databaseDSN, queryText string, executionPlan map[string]any, queryParams []any, opts *Options) (map[string]any, error) {
	var ro *types.RequestOptions
	if opts != nil {
		ro = &types.RequestOptions{
			MaxCandidates:      opts.MaxCandidates,
			MinImprovementPct:  opts.MinImprovementPct,
			StatementTimeoutMs: opts.StatementTimeoutMs,
			IncludeColumns:     true,
		}
		if opts.IncludeColumns != nil {
			ro.IncludeColumns = *opts.IncludeColumns
		}
	}
	result, err := c.svc.Analyze(ctx, &types.AnalysisRequest{
		DatabaseDSN:       databaseDSN,
		QueryText:         queryText,
		ExecutionPlanJSON: executionPlan,
		QueryParams:       queryParams,
		Options:           ro,
	})
	if err != nil {
		return nil, err
	}
	return intapi.EncodeAnalysisResultJSON(result)
}
