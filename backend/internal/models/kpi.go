package models

import "time"

type KpiMetrics struct {
	InstanceName    string  `json:"instance_name"`
	CpuPct          float64 `json:"cpu_pct"`
	MemoryPct       float64 `json:"memory_pct"`
	DataDiskGB      float64 `json:"data_disk_gb"`
	LogDiskGB       float64 `json:"log_disk_gb"`
	Tps             float64 `json:"tps"`
	ActiveSessions  int     `json:"active_sessions"`
	BlockedSessions int     `json:"blocked_sessions"`
	CaptureTime     string  `json:"capture_time"`
}

type KpiCache struct {
	PrevTpsValue    float64   `json:"prev_tps_value"`
	PrevCaptureTime time.Time `json:"prev_capture_time"`
}
