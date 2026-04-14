// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Query similarity detection engine.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package similarity

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

const (
	SimilarityThreshold = 0.85
	EmbeddingDim        = 64
)

type Engine struct {
	store   map[string]*types.QueryEmbedding
	maxSize int
}

func New() *Engine {
	return &Engine{
		store:   make(map[string]*types.QueryEmbedding),
		maxSize: 1000,
	}
}

func (e *Engine) GenerateEmbedding(query string, qa *types.QueryAnalysis) []float64 {
	vec := make([]float64, EmbeddingDim)

	tableFeature := e.extractTableFeature(qa)
	predicateFeature := e.extractPredicateFeature(qa)
	joinFeature := e.extractJoinFeature(qa)
	orderByFeature := e.extractOrderByFeature(qa)

	copy(vec[0:16], tableFeature)
	copy(vec[16:32], predicateFeature)
	copy(vec[32:48], joinFeature)
	copy(vec[48:64], orderByFeature)

	norm := 0.0
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}

	return vec
}

func (e *Engine) extractTableFeature(qa *types.QueryAnalysis) []float64 {
	feature := make([]float64, 16)
	if qa == nil || len(qa.Tables) == 0 {
		return feature
	}

	for i, t := range qa.Tables {
		if i >= 8 {
			break
		}
		hash := hashString(t.Name)
		feature[i*2] = float64(hash%100) / 100.0
		feature[i*2+1] = 1.0 / float64(len(qa.Tables))
	}

	return feature
}

func (e *Engine) extractPredicateFeature(qa *types.QueryAnalysis) []float64 {
	feature := make([]float64, 16)
	if qa == nil || len(qa.Predicates) == 0 {
		return feature
	}

	for i, p := range qa.Predicates {
		if i >= 8 {
			break
		}
		feature[i*2] = float64(hashString(p.Column)%100) / 100.0
		switch p.Type {
		case types.PredicateTypeEquality:
			feature[i*2+1] = 1.0
		case types.PredicateTypeRange:
			feature[i*2+1] = 0.5
		default:
			feature[i*2+1] = 0.25
		}
	}

	return feature
}

func (e *Engine) extractJoinFeature(qa *types.QueryAnalysis) []float64 {
	feature := make([]float64, 16)
	if qa == nil || len(qa.JoinInfo) == 0 {
		return feature
	}

	feature[0] = float64(len(qa.JoinInfo)) / 10.0

	for i, ji := range qa.JoinInfo {
		if i >= 4 {
			break
		}
		h := hashString(ji.LeftTable.Name + "_" + ji.RightTable.Name)
		feature[i+1] = float64(h%100) / 100.0
	}

	return feature
}

func (e *Engine) extractOrderByFeature(qa *types.QueryAnalysis) []float64 {
	feature := make([]float64, 16)
	if qa == nil || len(qa.OrderBy) == 0 {
		return feature
	}

	feature[0] = float64(len(qa.OrderBy)) / 5.0

	for i, o := range qa.OrderBy {
		if i >= 7 {
			break
		}
		h := hashString(o.Column)
		feature[i+1] = float64(h%100) / 100.0
		if o.Descending {
			feature[i+1] += 0.5
		}
	}

	return feature
}

func (e *Engine) FindSimilar(query string, qa *types.QueryAnalysis) *types.SimilarityResult {
	vector := e.GenerateEmbedding(query, qa)

	var bestMatch *types.QueryEmbedding
	bestScore := 0.0

	for _, emb := range e.store {
		score := cosineSimilarity(vector, emb.Vector)
		if score > bestScore {
			bestScore = score
			bestMatch = emb
		}
	}

	if bestMatch == nil || bestScore < SimilarityThreshold {
		return nil
	}

	joinOrderCopy := make([]string, len(bestMatch.JoinOrder))
	copy(joinOrderCopy, bestMatch.JoinOrder)
	indexesCopy := make([]types.IndexCandidate, len(bestMatch.Indexes))
	copy(indexesCopy, bestMatch.Indexes)

	return &types.SimilarityResult{
		QueryID:          bestMatch.ID,
		Similarity:       bestScore,
		ReusedJoinOrder:  joinOrderCopy,
		ReusedIndexes:    indexesCopy,
		JoinOrderChanged: false,
		IndexChangeCount: 0,
	}
}

func (e *Engine) Store(query string, qa *types.QueryAnalysis, joinOrder []string, indexes []types.IndexCandidate, avgLatency float64) {
	id := generateQueryID(query)
	vector := e.GenerateEmbedding(query, qa)

	joinOrderCopy := make([]string, len(joinOrder))
	copy(joinOrderCopy, joinOrder)
	indexesCopy := make([]types.IndexCandidate, len(indexes))
	copy(indexesCopy, indexes)

	emb := &types.QueryEmbedding{
		ID:         id,
		QueryHash:  hex.EncodeToString([]byte{byte(hashString(query) & 0xFF)}),
		Vector:     vector,
		JoinOrder:  joinOrderCopy,
		Indexes:    indexesCopy,
		AvgLatency: avgLatency,
		ExecCount:  1,
		LastSeen:   time.Now().Unix(),
	}

	e.store[id] = emb

	if len(e.store) > e.maxSize {
		e.evictLeastUsed()
	}
}

func (e *Engine) UpdateFeedback(queryID string, latency float64, rows int64, indexesUsed []string, joinOrder []string) {
	emb, ok := e.store[queryID]
	if !ok {
		return
	}

	emb.ExecCount++
	emb.AvgLatency = (emb.AvgLatency*float64(emb.ExecCount-1) + latency) / float64(emb.ExecCount)
	emb.LastSeen = time.Now().Unix()

	if len(joinOrder) > 0 {
		joinOrderCopy := make([]string, len(joinOrder))
		copy(joinOrderCopy, joinOrder)
		emb.JoinOrder = joinOrderCopy
	}
}

func (e *Engine) evictLeastUsed() {
	var oldestID string
	oldestTime := int64(math.MaxInt64)

	for id, emb := range e.store {
		if emb.LastSeen < oldestTime {
			oldestTime = emb.LastSeen
			oldestID = id
		}
	}

	if oldestID != "" {
		delete(e.store, oldestID)
	}
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	dot := 0.0
	normA := 0.0
	normB := 0.0

	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func hashString(s string) int {
	h := sha256.Sum256([]byte(s))
	return int(h[0])<<24 | int(h[1])<<16 | int(h[2])<<8 | int(h[3])
}

func generateQueryID(query string) string {
	h := sha256.Sum256([]byte(query))
	return hex.EncodeToString(h[:8])
}

type SimilaritySearchResult struct {
	Results    []types.SimilarityResult
	TotalCount int
}

func (e *Engine) SearchByEmbedding(vector []float64, limit int) SimilaritySearchResult {
	type scoredResult struct {
		id    string
		score float64
	}

	var scored []scoredResult

	for id, emb := range e.store {
		score := cosineSimilarity(vector, emb.Vector)
		scored = append(scored, scoredResult{id: id, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	results := make([]types.SimilarityResult, 0, minInt(len(scored), limit))
	for i := 0; i < len(scored) && i < limit; i++ {
		emb := e.store[scored[i].id]
		joinOrderCopy := make([]string, len(emb.JoinOrder))
		copy(joinOrderCopy, emb.JoinOrder)
		indexesCopy := make([]types.IndexCandidate, len(emb.Indexes))
		copy(indexesCopy, emb.Indexes)

		results = append(results, types.SimilarityResult{
			QueryID:         emb.ID,
			Similarity:      scored[i].score,
			ReusedJoinOrder: joinOrderCopy,
			ReusedIndexes:   indexesCopy,
		})
	}

	return SimilaritySearchResult{
		Results:    results,
		TotalCount: len(scored),
	}
}

func ExtractQuerySignature(query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	query = strings.ReplaceAll(query, " ", "")

	var signature []string

	tables := extractTablesSimple(query)
	signature = append(signature, tables...)

	predicates := extractPredicatesSimple(query)
	signature = append(signature, predicates...)

	joins := extractJoinsSimple(query)
	signature = append(signature, joins...)

	return strings.Join(signature, "|")
}

func extractTablesSimple(query string) []string {
	var tables []string
	parts := strings.Split(query, "from")
	if len(parts) > 1 {
		rest := strings.Split(parts[1], "where")[0]
		rest = strings.Split(rest, "join")[0]
		words := strings.Fields(rest)
		for _, w := range words {
			if w != "as" && w != "on" {
				tables = append(tables, w)
			}
		}
	}
	return tables
}

func extractPredicatesSimple(query string) []string {
	var preds []string
	parts := strings.Split(query, "where")
	if len(parts) > 1 {
		rest := parts[1]
		rest = strings.Split(rest, "order")[0]
		rest = strings.Split(rest, "group")[0]
		rest = strings.Split(rest, "limit")[0]
		preds = append(preds, strings.Fields(rest)...)
	}
	return preds[:minInt(len(preds), 5)]
}

func extractJoinsSimple(query string) []string {
	var joins []string
	if strings.Contains(query, "join") {
		parts := strings.Split(query, "join")
		for i := 1; i < len(parts); i++ {
			word := strings.Fields(parts[i])[0]
			joins = append(joins, word)
		}
	}
	return joins
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
