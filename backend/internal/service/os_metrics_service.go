// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Service implementation for OS-level metrics processing.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"fmt"
	"github.com/google/uuid"
)

func (s *MetricsService) SaveOSMetrics(ctx context.Context, serverID, collectorID uuid.UUID, metrics map[string]interface{}) error {
	if s.tsLogger == nil {
		return fmt.Errorf("timescale logger not initialized")
	}
	hostname := ""
	for _, inst := range s.Config.Instances {
		if inst.ServerID == serverID {
			hostname = inst.Name
			break
		}
	}
	return s.tsLogger.SaveOSMetrics(ctx, hostname, serverID, metrics)
}
