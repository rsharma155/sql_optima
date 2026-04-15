// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Optional Linux host probes for PostgreSQL CPU dashboard (host vs. postgres process CPU, load, cores).
//          Intended for co-located collectors; set SQL_OPTIMA_PG_COLLECT_HOST_CPU=1 to enable.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package pghostcpu

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Snapshot holds OS-level samples from the machine running this process.
type Snapshot struct {
	HostCpuPercent     float64 `json:"host_cpu_percent"`
	PostgresCpuPercent float64 `json:"postgres_cpu_percent"`
	Load1m             float64 `json:"load_1m"`
	Load5m             float64 `json:"load_5m"`
	Load15m            float64 `json:"load_15m"`
	CpuCores           int     `json:"cpu_cores"`
}

func runBash(script string) string {
	out, err := exec.Command("bash", "-c", script).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Collect runs Linux probes when SQL_OPTIMA_PG_COLLECT_HOST_CPU=1 and GOOS=linux.
func Collect() Snapshot {
	if runtime.GOOS != "linux" {
		return Snapshot{}
	}
	if strings.TrimSpace(os.Getenv("SQL_OPTIMA_PG_COLLECT_HOST_CPU")) != "1" {
		return Snapshot{}
	}
	return collectLinux()
}

func collectLinux() Snapshot {
	var s Snapshot
	s.CpuCores = parseInt(runBash("nproc"))
	if s.CpuCores <= 0 {
		s.CpuCores = parseInt(runBash("getconf _NPROCESSORS_ONLN 2>/dev/null || echo 0"))
	}
	s.Load1m, s.Load5m, s.Load15m = parseLoadAvg(runBash("cat /proc/loadavg"))

	hostRaw := runBash("top -bn1 | grep 'Cpu(s)' | awk '{print $2 + $4}'")
	s.HostCpuPercent, _ = strconv.ParseFloat(hostRaw, 64)

	pgRaw := runBash("ps -eo pcpu,comm 2>/dev/null | grep postgres | awk '{sum+=$1} END {print sum}'")
	if pgRaw == "" {
		pgRaw = "0"
	}
	s.PostgresCpuPercent, _ = strconv.ParseFloat(pgRaw, 64)
	return s
}

// parseLoadAvg parses the first three fields of /proc/loadavg output.
func parseLoadAvg(line string) (float64, float64, float64) {
	line = strings.TrimSpace(line)
	if line == "" {
		return 0, 0, 0
	}
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return 0, 0, 0
	}
	a, _ := strconv.ParseFloat(parts[0], 64)
	b, _ := strconv.ParseFloat(parts[1], 64)
	c, _ := strconv.ParseFloat(parts[2], 64)
	return a, b, c
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// CpuSaturationPct returns (load_1m / cpu_cores) * 100 when cores > 0.
func CpuSaturationPct(load1m float64, cpuCores int) float64 {
	if cpuCores <= 0 || load1m < 0 {
		return 0
	}
	return (load1m / float64(cpuCores)) * 100
}

// CpuPerConnection returns postgres CPU % divided by active connections (0 if none).
func CpuPerConnection(postgresCpuPct float64, activeConns int) float64 {
	if activeConns <= 0 || postgresCpuPct < 0 {
		return 0
	}
	return postgresCpuPct / float64(activeConns)
}
