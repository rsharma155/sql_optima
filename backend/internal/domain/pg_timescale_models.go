package domain

import (
	"github.com/google/uuid"
	"time"
)

// PGTimescaleLock represents a historical lock event for PostgreSQL,
// prefixed with pg_ as per Phase 8 requirements.
type PGTimescaleLock struct {
	CaptureTimestamp time.Time `json:"capture_timestamp" db:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id" db:"server_id"`
	DatabaseName     string    `json:"database_name" db:"database_name"`
	PID              int       `json:"pid" db:"pid"`
	WaitEventType    string    `json:"wait_event_type" db:"wait_event_type"`
	WaitEvent        string    `json:"wait_event" db:"wait_event"`
	LockType         string    `json:"lock_type" db:"lock_type"`
	Mode             string    `json:"mode" db:"mode"`
	Granted          bool      `json:"granted" db:"granted"`
	QueryText        string    `json:"query_text" db:"query_text"`
	BlockedBy        int       `json:"blocked_by" db:"blocked_by"`
	WaitDurationMs   int64     `json:"wait_duration_ms" db:"wait_duration_ms"`
}

// PGStatStatementsDelta represents differential query performance metrics.
type PGStatStatementsDelta struct {
	CaptureTimestamp  time.Time `json:"capture_timestamp" db:"capture_timestamp"`
	ServerID          uuid.UUID `json:"server_id" db:"server_id"`
	QueryID           int64     `json:"query_id" db:"query_id"`
	DatabaseName      string    `json:"database_name" db:"database_name"`
	UserName          string    `json:"user_name" db:"user_name"`
	CallsDelta        int64     `json:"calls_delta" db:"calls_delta"`
	TotalTimeDeltaMs  float64   `json:"total_time_delta_ms" db:"total_time_delta_ms"`
	RowsDelta         int64     `json:"rows_delta" db:"rows_delta"`
	SharedBlksHit     int64     `json:"shared_blks_hit_delta" db:"shared_blks_hit_delta"`
	SharedBlksRead    int64     `json:"shared_blks_read_delta" db:"shared_blks_read_delta"`
	SharedBlksDirtied int64     `json:"shared_blks_dirtied_delta" db:"shared_blks_dirtied_delta"`
	SharedBlksWritten int64     `json:"shared_blks_written_delta" db:"shared_blks_written_delta"`
	TempBlksRead      int64     `json:"temp_blks_read_delta" db:"temp_blks_read_delta"`
	TempBlksWritten   int64     `json:"temp_blks_written_delta" db:"temp_blks_written_delta"`
	BlkReadTimeMs     float64   `json:"blk_read_time_delta_ms" db:"blk_read_time_delta_ms"`
	BlkWriteTimeMs    float64   `json:"blk_write_time_delta_ms" db:"blk_write_time_delta_ms"`
	WalBytes          int64     `json:"wal_bytes_delta" db:"wal_bytes_delta"`
}

// PGInstanceSnapshot represents a unified health snapshot of a PostgreSQL engine.
type PGInstanceSnapshot struct {
	CaptureTimestamp time.Time `json:"capture_timestamp" db:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id" db:"server_id"`

	HealthScore      int     `json:"health_score" db:"health_score"`
	TotalConnections int     `json:"total_connections" db:"total_connections"`
	ActiveSessions   int     `json:"active_sessions" db:"active_sessions"`
	IdleSessions     int     `json:"idle_sessions" db:"idle_sessions"`
	WaitingSessions  int     `json:"waiting_sessions" db:"waiting_sessions"`
	BlockedSessions  int     `json:"blocked_sessions" db:"blocked_sessions"`
	LongestActiveMs  float64 `json:"longest_active_ms" db:"longest_active_ms"`

	TPS               float64 `json:"tps" db:"tps"`
	CacheHitRatio     float64 `json:"cache_hit_ratio" db:"cache_hit_ratio"`
	RWRatio           float64 `json:"rw_ratio" db:"rw_ratio"`
	AvgQueryLatencyMs float64 `json:"avg_query_latency_ms" db:"avg_query_latency_ms"`

	WalGenRateMbps     float64 `json:"wal_generation_rate_mbps" db:"wal_generation_rate_mbps"`
	ReplicaLagSec      float64 `json:"replica_lag_sec" db:"replica_lag_sec"`
	MaxXidAge          int64   `json:"max_xid_age" db:"max_xid_age"`
	CheckpointReqRatio float64 `json:"checkpoint_req_ratio" db:"checkpoint_req_ratio"`
}
