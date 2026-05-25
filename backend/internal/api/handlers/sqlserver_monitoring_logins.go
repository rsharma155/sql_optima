// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Resolve SQL Server monitoring login names to exclude from user workload views.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package handlers

import (
	"sort"
	"strings"

	"github.com/rsharma155/sql_optima/internal/config"
)

var sqlServerDefaultExcludedLogins = []string{
	"dbmonitor_user", "dbmonitor", "sql-optima", "sql_optima", "sqloptima",
}

// sqlServerExcludeLoginsForInstance returns login names to omit (optima_servers.username + defaults).
func sqlServerExcludeLoginsForInstance(cfg *config.Config, instanceName string) []string {
	seen := make(map[string]struct{})
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		seen[strings.ToLower(name)] = struct{}{}
	}
	for _, u := range sqlServerDefaultExcludedLogins {
		add(u)
	}
	if cfg != nil {
		for _, inst := range cfg.Instances {
			if strings.EqualFold(inst.Name, instanceName) {
				add(inst.MonitoringUser)
				add(inst.User)
				break
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
