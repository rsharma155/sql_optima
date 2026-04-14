// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Exported collector functions for external service integration.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import "context"

// RunLiveCollectorOnce runs one live diagnostics scrape (used by async workers).
func (s *MetricsService) RunLiveCollectorOnce(ctx context.Context) {
	s.runLiveDiagnosticsWithContext(ctx)
}

// RunLiveCollectorForInstance runs one live scrape for a single instance (used by on-demand dashboard endpoints).
func (s *MetricsService) RunLiveCollectorForInstance(ctx context.Context, instanceName string) {
	s.runLiveDiagnosticsForInstance(ctx, instanceName)
}

// RunHistoricalCollectorOnce runs one historical / Timescale persistence tick (used by async workers).
func (s *MetricsService) RunHistoricalCollectorOnce(ctx context.Context) {
	s.runHistoricalStorageWithContext(ctx)
}
