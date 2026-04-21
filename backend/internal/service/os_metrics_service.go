package service

import (
	"context"
	"fmt"
)

// SaveOSMetrics saves OS level metrics collected from the host agent.
func (s *MetricsService) SaveOSMetrics(ctx context.Context, hostname, instanceName string, metrics map[string]interface{}) error {
	if s.tsLogger == nil {
		return fmt.Errorf("tsLogger not available")
	}
	return s.tsLogger.SaveOSMetrics(ctx, hostname, instanceName, metrics)
}

// GetOSCollectorStatus checks if we have received metrics from the OS collector recently.
func (s *MetricsService) GetOSCollectorStatus(ctx context.Context, hostname string) (bool, error) {
	if s.tsLogger == nil {
		return false, nil
	}
	return s.tsLogger.CheckOSCollectorStatus(ctx, hostname)
}
