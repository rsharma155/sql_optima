package service

import "context"

// RunLiveCollectorOnce runs one live diagnostics scrape (used by async workers).
func (s *MetricsService) RunLiveCollectorOnce(ctx context.Context) {
	s.runLiveDiagnosticsWithContext(ctx)
}

// RunHistoricalCollectorOnce runs one historical / Timescale persistence tick (used by async workers).
func (s *MetricsService) RunHistoricalCollectorOnce(ctx context.Context) {
	s.runHistoricalStorageWithContext(ctx)
}
