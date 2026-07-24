// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Optional revoke-list hook for OS-agent JWTs (checked on metric ingest).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package middleware

import (
	"context"
	"sync"
)

// OSAgentRevokeChecker reports whether a JWT id (jti) has been revoked.
type OSAgentRevokeChecker interface {
	IsRevoked(ctx context.Context, jti string) (bool, error)
}

var (
	osAgentRevokeMu      sync.RWMutex
	osAgentRevokeChecker OSAgentRevokeChecker
)

// SetOSAgentRevokeChecker registers the backend used by RequireOSMetricsAuth.
// Pass nil to disable revoke checks (e.g. tests without a DB).
func SetOSAgentRevokeChecker(c OSAgentRevokeChecker) {
	osAgentRevokeMu.Lock()
	osAgentRevokeChecker = c
	osAgentRevokeMu.Unlock()
}

func getOSAgentRevokeChecker() OSAgentRevokeChecker {
	osAgentRevokeMu.RLock()
	defer osAgentRevokeMu.RUnlock()
	return osAgentRevokeChecker
}
