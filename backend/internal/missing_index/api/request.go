// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Request models for missing index API.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import (
	"time"

	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

// IndexAdvisorRequest represents the incoming API request
type IndexAdvisorRequest struct {
	DatabaseDSN       string          `json:"database_dsn"`
	QueryText         string          `json:"query_text"`
	ExecutionPlanJSON map[string]any  `json:"execution_plan_json"`
	QueryParams       []any           `json:"query_params"`
	Options           *RequestOptions `json:"options"`
}

// RequestOptions contains optional parameters for the request
type RequestOptions struct {
	MaxCandidates      int     `json:"max_candidates"`
	MinImprovementPct  float64 `json:"min_improvement_pct"`
	StatementTimeoutMs int     `json:"statement_timeout_ms"`
	IncludeColumns     bool    `json:"include_columns"`
}

// DefaultRequestOptions returns the default options
func DefaultRequestOptions() *RequestOptions {
	return &RequestOptions{
		MaxCandidates:      5,
		MinImprovementPct:  15.0,
		StatementTimeoutMs: 5000,
		IncludeColumns:     true,
	}
}

// IndexAdvisorResponse represents the API response
type IndexAdvisorResponse struct {
	RecommendationStatus types.RecommendationStatus `json:"recommendation_status"`
	TopRecommendation    *Recommendation            `json:"top_recommendation,omitempty"`
	Alternatives         []Recommendation           `json:"alternatives,omitempty"`
	Rejections           []RejectedCandidate        `json:"rejections,omitempty"`
	Diagnostics          types.DiagnosticInfo       `json:"diagnostics"`
	DebugInfo            *types.DebugInfo           `json:"debug_info,omitempty"`
	QueryRewrites        []QueryRewrite             `json:"query_rewrites,omitempty"`
}

// QueryRewrite represents a query rewrite suggestion
type QueryRewrite struct {
	OriginalQuery  string   `json:"original_query"`
	RewrittenQuery string   `json:"rewritten_query"`
	AppliedRules   []string `json:"applied_rules"`
}

// Recommendation contains a single index recommendation
type Recommendation struct {
	Table          string   `json:"table"`
	IndexMethod    string   `json:"index_method"`
	IndexStatement string   `json:"index_statement"`
	Confidence     float64  `json:"confidence"`
	Reasoning      []string `json:"reasoning"`
	Evidence       Evidence `json:"evidence"`
}

// Evidence contains supporting evidence for the recommendation
type Evidence struct {
	OriginalTotalCost     float64 `json:"original_total_cost"`
	HypotheticalTotalCost float64 `json:"hypothetical_total_cost"`
	ImprovementPct        float64 `json:"improvement_pct"`
	PlanChangeDetected    bool    `json:"plan_change_detected"`
	IndexUsedInPlan       bool    `json:"index_used_in_plan"`
}

// RejectedCandidate represents a candidate that was rejected
type RejectedCandidate struct {
	Candidate       types.IndexCandidate `json:"candidate"`
	RejectionReason string               `json:"rejection_reason"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}
