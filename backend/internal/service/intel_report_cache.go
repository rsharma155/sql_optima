// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: In-memory cache for intelligence report analysis and HTML between Analyze and GetReport.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package service

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/models"
)

const intelReportCacheTTL = 15 * time.Minute

type intelReportCacheEntry struct {
	result  *models.IntelligenceReportResponse
	rawData map[string]interface{}
	html    string
	at      time.Time
}

type intelReportCache struct {
	mu    sync.RWMutex
	items map[string]intelReportCacheEntry
}

func newIntelReportCache() *intelReportCache {
	return &intelReportCache{items: make(map[string]intelReportCacheEntry)}
}

func (c *intelReportCache) set(serverID uuid.UUID, result *models.IntelligenceReportResponse, raw map[string]interface{}, html string) {
	if result == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[serverID.String()] = intelReportCacheEntry{
		result:  result,
		rawData: raw,
		html:    html,
		at:      time.Now(),
	}
}

func (c *intelReportCache) get(serverID uuid.UUID) *intelReportCacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.items[serverID.String()]
	if !ok {
		return nil
	}
	if time.Since(e.at) > intelReportCacheTTL {
		return nil
	}
	return &e
}
