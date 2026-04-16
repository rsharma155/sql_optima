// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Feedback collector for index recommendation accuracy.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package feedback

import (
	"sync"
	"time"

	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

type Collector struct {
	mu      sync.RWMutex
	records []types.FeedbackRecord
	maxSize int
}

func New() *Collector {
	return &Collector{
		records: make([]types.FeedbackRecord, 0),
		maxSize: 10000,
	}
}

func (c *Collector) Record(latency float64, rows int64, indexesUsed []string, memoryMB float64, plan string, joinOrder []string, queryID string, queryText string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	record := types.FeedbackRecord{
		QueryID:       queryID,
		QueryText:     queryText,
		ActualLatency: latency,
		RowsProcessed: rows,
		IndexesUsed:   indexesUsed,
		MemoryUsageMB: memoryMB,
		PlanUsed:      plan,
		JoinOrder:     joinOrder,
		Timestamp:     time.Now().Unix(),
	}

	c.records = append(c.records, record)

	if len(c.records) > c.maxSize {
		c.records = c.records[len(c.records)-c.maxSize:]
	}
}

func (c *Collector) GetRecordsForRLTraining() []types.FeedbackRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	records := make([]types.FeedbackRecord, len(c.records))
	copy(records, c.records)
	return records
}

func (c *Collector) GetRecordsForEmbedding() []types.FeedbackRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var recent []types.FeedbackRecord
	if len(c.records) > 100 {
		recent = c.records[len(c.records)-100:]
	} else {
		recent = c.records
	}

	result := make([]types.FeedbackRecord, len(recent))
	copy(result, recent)
	return result
}

func (c *Collector) GetQueryStats(queryID string) *QueryStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalLatency float64
	var totalRows int64
	var count int
	var minLat, maxLat float64

	for _, r := range c.records {
		if r.QueryID == queryID {
			totalLatency += r.ActualLatency
			totalRows += r.RowsProcessed
			if count == 0 || r.ActualLatency < minLat {
				minLat = r.ActualLatency
			}
			if count == 0 || r.ActualLatency > maxLat {
				maxLat = r.ActualLatency
			}
			count++
		}
	}

	if count == 0 {
		return nil
	}

	return &QueryStats{
		QueryID:       queryID,
		AvgLatency:    totalLatency / float64(count),
		TotalRows:     totalRows,
		ExecCount:     int64(count),
		MinLatency:    minLat,
		MaxLatency:    maxLat,
		LastExecution: c.records[len(c.records)-1].Timestamp,
	}
}

func (c *Collector) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = make([]types.FeedbackRecord, 0)
}

func (c *Collector) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.records)
}

type QueryStats struct {
	QueryID       string
	AvgLatency    float64
	TotalRows     int64
	ExecCount     int64
	MinLatency    float64
	MaxLatency    float64
	LastExecution int64
}

type LearningMetrics struct {
	TotalQueries          int
	AvgLatencyImprovement float64
	IndexHitRate          float64
	JoinOrderStability    float64
	SimilarityCacheHits   int
}

func (c *Collector) CalculateMetrics() LearningMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metrics := LearningMetrics{
		TotalQueries: len(c.records),
	}

	if len(c.records) == 0 {
		return metrics
	}

	var totalLatency float64
	var indexHits int

	joinOrders := make(map[string]int)

	for _, r := range c.records {
		totalLatency += r.ActualLatency

		if len(r.IndexesUsed) > 0 {
			indexHits++
		}

		if len(r.JoinOrder) > 0 {
			joinKey := ""
			for _, t := range r.JoinOrder {
				joinKey += t + "-"
			}
			joinOrders[joinKey]++
		}
	}

	metrics.AvgLatencyImprovement = totalLatency / float64(len(c.records))
	metrics.IndexHitRate = float64(indexHits) / float64(len(c.records))

	if len(joinOrders) > 0 {
		maxCount := 0
		for _, count := range joinOrders {
			if count > maxCount {
				maxCount = count
			}
		}
		metrics.JoinOrderStability = float64(maxCount) / float64(len(c.records))
	}

	return metrics
}
