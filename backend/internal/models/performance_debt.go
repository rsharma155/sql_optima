package models

import (
	"github.com/google/uuid"
	"time"
)

type PerformanceDebtFinding struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	DatabaseName     string    `json:"database_name"`
	FindingType      string    `json:"finding_type"`
	ObjectName       string    `json:"object_name"`
	ImpactScore      float64   `json:"impact_score"`
	Details          string    `json:"details"`
}
