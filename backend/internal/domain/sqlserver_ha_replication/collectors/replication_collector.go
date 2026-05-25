// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : 1-minute replication latency and 15-minute topology collector.
//           Captures replication backlog (undelivered commands) and
//           publisher→subscriber relationship details.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package collectors

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// ReplicationCollector handles replication performance and topology.
type ReplicationCollector struct {
	querier  *SourceQuerier
	tsLogger *hot.TimescaleLogger
}

// NewReplicationCollector constructs the collector.
func NewReplicationCollector(q *SourceQuerier, ts *hot.TimescaleLogger) *ReplicationCollector {
	return &ReplicationCollector{querier: q, tsLogger: ts}
}

// RunLatency fetches undelivered commands and latency.
func (c *ReplicationCollector) RunLatency(ctx context.Context, serverID uuid.UUID, instanceName string) error {
	now := time.Now().UTC()
	points, err := c.querier.FetchReplicationLatency(ctx, instanceName)
	if err != nil {
		return err
	}

	var rows []hot.ReplicationLatencyRow
	for _, p := range points {
		status := p.Status
		if status == "" {
			status = "Unknown"
		}

		// Optimization: skip storing data if everything is zero (idle replication)
		// BUT always store if there is an error/warning status
		isHealthy := status == "Idle" || status == "Running" || status == "Unknown"
		if isHealthy && p.MaxLatencySeconds == 0 && p.PeakBacklog == 0 && p.DeliveryRateCmdsSec == 0 {
			continue
		}

		rows = append(rows, hot.ReplicationLatencyRow{
			CaptureTimestamp:      now,
			ServerID:              serverID,
			Publisher:             p.Publisher,
			Subscriber:            p.Subscriber,
			Publication:           p.Publication,
			LatencySeconds:        p.MaxLatencySeconds,
			UndistributedCommands: p.PeakBacklog,
			DeliveryRateCmdsSec:   p.DeliveryRateCmdsSec,
			Status:                status,
		})
	}

	if len(rows) == 0 {
		return nil
	}
	return c.tsLogger.WriteReplicationLatency(ctx, rows)
}

// RunTopology fetches pub/sub relationships.
func (c *ReplicationCollector) RunTopology(ctx context.Context, serverID uuid.UUID, instanceName string) error {
	now := time.Now().UTC()
	topology, err := c.querier.FetchReplicationTopology(ctx, instanceName)
	if err != nil {
		return err
	}

	rows := make([]hot.ReplicationTopologyRow, len(topology))
	for i, t := range topology {
		var lastSync *time.Time
		if !t.LastSyncTime.IsZero() && t.LastSyncTime.Year() > 1970 {
			tmp := t.LastSyncTime
			lastSync = &tmp
		}
		rows[i] = hot.ReplicationTopologyRow{
			CaptureTimestamp: now,
			ServerID:         serverID,
			Publisher:        t.Publisher,
			Subscriber:       t.Subscriber,
			Publication:      t.Publication,
			PublicationDB:    t.PublicationDB,
			SubscriberDB:     t.SubscriberDB,
			ReplicationType:  t.ReplicationType,
			SyncType:         t.SyncType,
			AgentStatus:      t.AgentStatus,
			LastSyncTime:     lastSync,
		}
	}

	return c.tsLogger.WriteReplicationTopology(ctx, rows)
}

// RunArticles fetches table-level replication details.
func (c *ReplicationCollector) RunArticles(ctx context.Context, serverID uuid.UUID, instanceName string) error {
	now := time.Now().UTC()
	articles, err := c.querier.FetchReplicationArticles(ctx, instanceName)
	if err != nil {
		return err
	}

	var rows []hot.ReplicationArticleRow
	for _, a := range articles {
		// Optimization: skip storing table-level data if idle
		// BUT always store if there is an error/warning status
		isHealthy := a.Status == "Idle" || a.Status == "Active" || a.Status == "Succeeded" || a.Status == "Starting" || a.Status == "Unknown"
		if isHealthy && a.RowsPerSec == 0 && a.LatencySeconds == 0 && a.ConflictsDetected == 0 {
			continue
		}

		rows = append(rows, hot.ReplicationArticleRow{
			CaptureTimestamp:  now,
			ServerID:          serverID,
			Publication:       a.Publication,
			DatabaseName:      a.DatabaseName,
			SchemaName:        a.SchemaName,
			TableName:         a.TableName,
			Subscriber:        a.Subscriber,
			RowsPerSec:        a.RowsPerSec,
			LatencySeconds:    a.LatencySeconds,
			ConflictsDetected: a.ConflictsDetected,
			Status:            a.Status,
		})
	}

	if len(rows) == 0 {
		return nil
	}
	return c.tsLogger.WriteReplicationArticles(ctx, rows)
}
