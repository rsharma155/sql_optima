// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : Lightweight 15-minute feature detection collector.
//           Determines if HA or Replication is active on the instance.
//           If neither is active, HA/Replication collectors for this server
//           are stopped to save resources and avoid "empty dashboard" noise.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package collectors

import (
	"log/slog"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// FeatureDetector orchestrates the 15-minute HA/Replication discovery cycle.
type FeatureDetector struct {
	querier  *SourceQuerier
	tsLogger *hot.TimescaleLogger
}

// NewFeatureDetector constructs the detector.
func NewFeatureDetector(q *SourceQuerier, ts *hot.TimescaleLogger) *FeatureDetector {
	return &FeatureDetector{querier: q, tsLogger: ts}
}

// Run executes the detection query and persists the result to TimescaleDB.
func (c *FeatureDetector) Run(ctx context.Context, serverID uuid.UUID, instanceName string) (bool, bool, error) {
	feat, err := c.querier.FetchFeatureDetection(ctx, instanceName)
	if err != nil {
		return false, false, err
	}

	feat.ServerID = serverID.String()
	feat.CaptureTimestamp = time.Now().UTC()

	err = c.tsLogger.WriteHAFeatureDetection(ctx, hot.HAFeatureDetectionRow{
		ServerID:           serverID,
		CaptureTimestamp:   feat.CaptureTimestamp,
		HAEnabled:          feat.HAEnabled,
		ReplicationEnabled: feat.ReplicationEnabled,
		AGEnabled:          feat.AGEnabled,
		FCIEnabled:         feat.FCIEnabled,
		LogShippingEnabled: feat.LogShippingEnabled,
		MirroringEnabled:   feat.MirroringEnabled,
		ReplicationTypes:   feat.ReplicationTypes,
	})

	if err != nil {
		slog.Error("[HA-Collector] failed to write feature detection", "target", instanceName, "err", err)
	}

	return feat.HAEnabled, feat.ReplicationEnabled, nil
}
