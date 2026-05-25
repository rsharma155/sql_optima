// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Package domain defines the ubiquitous language types for the SQL Server
// HA & Replication monitoring bounded context.  No external project imports
// are allowed here — this package contains only pure value objects and
// domain entity types so that business rules stay database-agnostic and
// fully unit-testable.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package domain

import "time"

// ---------------------------------------------------------------------------
// Feature Detection
// ---------------------------------------------------------------------------

// FeatureDetection records which high-availability and replication features
// are active on a monitored SQL Server instance.  The dashboard page gate
// reads the latest row: if both HAEnabled and ReplicationEnabled are false
// the page is hidden and all collectors stop.
type FeatureDetection struct {
	ServerID           string    `json:"server_id"`
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	HAEnabled          bool      `json:"ha_enabled"`
	ReplicationEnabled bool      `json:"replication_enabled"`
	AGEnabled          bool      `json:"ag_enabled"`
	FCIEnabled         bool      `json:"fci_enabled"`
	LogShippingEnabled bool      `json:"log_shipping_enabled"`
	MirroringEnabled   bool      `json:"mirroring_enabled"`
	ReplicationTypes   []string  `json:"replication_types"`
}

// ShouldShowPage returns true when at least one HA or replication feature is
// active — i.e. the dashboard page should be rendered.
func (f FeatureDetection) ShouldShowPage() bool {
	return f.HAEnabled || f.ReplicationEnabled
}

// ---------------------------------------------------------------------------
// HA — Replica Health
// ---------------------------------------------------------------------------

// ReplicaHealthRow is a point-in-time snapshot of one availability group
// replica captured from sys.dm_hadr_availability_replica_states.
type ReplicaHealthRow struct {
	AGName                   string    `json:"ag_name"`
	ReplicaServerName        string    `json:"replica_server_name"`
	RoleDesc                 string    `json:"role_desc"`
	SynchronizationStateDesc string    `json:"synchronization_state_desc"`
	SynchronizationHealth    string    `json:"synchronization_health_desc"`
	AvailabilityModeDesc     string    `json:"availability_mode_desc"`
	LogSendQueueKB           int64     `json:"log_send_queue_kb"`
	RedoQueueKB              int64     `json:"redo_queue_kb"`
	LogSendRateKBPS          int64     `json:"log_send_rate_kbps"`
	RedoRateKBPS             int64     `json:"redo_rate_kbps"`
	LastCommitTime           time.Time `json:"last_commit_time"`
	SecondaryLagSeconds      int64     `json:"secondary_lag_seconds"`
	ConnectedStateDesc       string    `json:"connected_state_desc"`
	IsFailoverReady          bool      `json:"is_failover_ready"`
	LongRunningTxCount       int       `json:"long_running_tx_count"`
	QuorumStateDesc          string    `json:"quorum_state_desc"`
}

// IsPrimary returns true when this row represents the PRIMARY replica.
func (r ReplicaHealthRow) IsPrimary() bool { return r.RoleDesc == "PRIMARY" }

// IsConnected returns true when the replica is currently connected.
func (r ReplicaHealthRow) IsConnected() bool { return r.ConnectedStateDesc == "CONNECTED" }

// IsHealthy returns true when synchronization health is HEALTHY.
func (r ReplicaHealthRow) IsHealthy() bool { return r.SynchronizationHealth == "HEALTHY" }

// ---------------------------------------------------------------------------
// HA — Database Sync State
// ---------------------------------------------------------------------------

// DatabaseSyncState captures per-database synchronization state within an AG.
type DatabaseSyncState struct {
	AGName                   string    `json:"ag_name"`
	DatabaseName             string    `json:"database_name"`
	ReplicaServerName        string    `json:"replica_server_name"`
	SynchronizationStateDesc string    `json:"synchronization_state_desc"`
	IsSuspended              bool      `json:"is_suspended"`
	LogSendQueueKB           int64     `json:"log_send_queue_kb"`
	RedoQueueKB              int64     `json:"redo_queue_kb"`
	LastCommitTime           time.Time `json:"last_commit_time"`
}

// ---------------------------------------------------------------------------
// HA — RPO / RTO derived metrics
// ---------------------------------------------------------------------------

// RPOThreshold classifies the current RPO into a traffic-light colour.
type RPOThreshold string

const (
	RPOGreen  RPOThreshold = "green"  // < 5 s
	RPOYellow RPOThreshold = "yellow" // 5–60 s
	RPORed    RPOThreshold = "red"    // > 60 s
)

// RTOThreshold classifies the estimated RTO.
type RTOThreshold string

const (
	RTOGreen  RTOThreshold = "green"  // < 30 s
	RTOYellow RTOThreshold = "yellow" // 30–120 s
	RTORed    RTOThreshold = "red"    // > 120 s
)

// RPOResult is the calculated Recovery Point Objective for a server.
type RPOResult struct {
	// Seconds is the worst-case data loss across all secondaries (MAX lag).
	Seconds      int64        `json:"rpo_seconds"`
	AvgSeconds   int64        `json:"avg_rpo_seconds"`
	MaxLagMB     float64      `json:"max_lag_mb"`
	Threshold    RPOThreshold `json:"threshold"`
	WorstReplica string       `json:"worst_replica"`
	// Timestamp is the time the measurement was taken (from continuous aggregate).
	Timestamp    time.Time    `json:"timestamp"`
}

// RTOResult is the estimated Recovery Time Objective for a server.
type RTOResult struct {
	// EstimatedSeconds = redo_time + cluster_failover_overhead (30 s constant).
	EstimatedSeconds   int64        `json:"estimated_rto_seconds"`
	RedoSeconds        int64        `json:"redo_seconds"`
	MaxRedoQueueKB     int64        `json:"max_redo_queue_kb"`
	ClusterOverheadSec int64        `json:"cluster_overhead_sec"`
	Threshold          RTOThreshold `json:"threshold"`
	Timestamp          time.Time    `json:"timestamp"`
}
// ---------------------------------------------------------------------------
// HA — Failover Readiness
// ---------------------------------------------------------------------------

// ReadinessStatus is the pass/fail/unknown state of a single readiness check.
type ReadinessStatus string

const (
	ReadinessPass    ReadinessStatus = "pass"
	ReadinessFail    ReadinessStatus = "fail"
	ReadinessUnknown ReadinessStatus = "unknown"
)

// FailoverReadinessCheck is a single item in the failover readiness checklist.
type FailoverReadinessCheck struct {
	Name    string          `json:"name"`
	Status  ReadinessStatus `json:"status"`
	Detail  string          `json:"detail,omitempty"`
}

// FailoverReadiness is the overall failover readiness assessment.
type FailoverReadiness struct {
	// Ready is true only when every check passes.
	Ready                       bool                     `json:"ready"`
	Checks                      []FailoverReadinessCheck `json:"checks"`
	LongRunningTransactionsCount int                      `json:"long_running_transactions_count"`
	QuorumStateDesc              string                   `json:"quorum_state_desc"`
}

// ---------------------------------------------------------------------------
// HA — Failover Events
// ---------------------------------------------------------------------------

// FailoverEvent records a detected primary change for an availability group.
type FailoverEvent struct {
	CaptureTimestamp        time.Time `json:"capture_timestamp"`
	AGName                  string    `json:"ag_name"`
	PreviousPrimary         string    `json:"previous_primary"`
	NewPrimary              string    `json:"new_primary"`
	FailoverType            string    `json:"failover_type"`
	FailoverDurationSeconds int       `json:"failover_duration_seconds"`
}

// ---------------------------------------------------------------------------
// Replication — Topology
// ---------------------------------------------------------------------------

// ReplicationTopologyRow is one publisher→subscriber relationship snapshot.
type ReplicationTopologyRow struct {
	Publisher           string    `json:"publisher"`
	Subscriber          string    `json:"subscriber"`
	Publication         string    `json:"publication"`
	PublicationDB       string    `json:"publication_db"`
	SubscriberDB        string    `json:"subscriber_db"`
	ReplicationType     string    `json:"replication_type"`
	SyncType            string    `json:"sync_type"`
	AgentStatus         string    `json:"agent_status"`
	LastSyncTime        time.Time `json:"last_sync_time"`
	LatencySeconds      int64     `json:"latency_seconds"`
	DeliveryRateCmdsSec int64     `json:"delivery_rate_cmds_sec"`
	HasError            bool      `json:"has_error"`
}

// ---------------------------------------------------------------------------
// Replication — Latency & Backlog
// ---------------------------------------------------------------------------

// ReplicationLatencyPoint is a single time-series data point for replication
// latency and undelivered command backlog.
type ReplicationLatencyPoint struct {
	Bucket                time.Time `json:"bucket"`
	Publisher             string    `json:"publisher"`
	Subscriber            string    `json:"subscriber"`
	Publication           string    `json:"publication"`
	PeakBacklog           int64     `json:"peak_backlog"`
	AvgBacklog            float64   `json:"avg_backlog"`
	MaxLatencySeconds     int64     `json:"max_latency_seconds"`
	DeliveryRateCmdsSec   int64     `json:"delivery_rate_cmds_sec"`
	Status                string    `json:"status"`
}

// ---------------------------------------------------------------------------
// Replication — Articles
// ---------------------------------------------------------------------------

// ReplicationArticle is the state of one replicated table (article).
type ReplicationArticle struct {
	Publication       string    `json:"publication"`
	DatabaseName      string    `json:"database_name"`
	SchemaName        string    `json:"schema_name"`
	TableName         string    `json:"table_name"`
	Subscriber        string    `json:"subscriber"`
	RowsPerSec        int64     `json:"rows_per_sec"`
	LatencySeconds    int64     `json:"latency_seconds"`
	ConflictsDetected int64     `json:"conflicts_detected"`
	Status            string    `json:"status"`
	CaptureTimestamp  time.Time `json:"capture_timestamp"`
}

// ---------------------------------------------------------------------------
// Coverage Panel
// ---------------------------------------------------------------------------

// ProtectionLevel classifies how well a database is protected.
type ProtectionLevel string

const (
	ProtectionGold   ProtectionLevel = "Gold"   // HA + Replication
	ProtectionSilver ProtectionLevel = "Silver" // HA only
	ProtectionBronze ProtectionLevel = "Bronze" // Replication only
	ProtectionNone   ProtectionLevel = "Red"    // Neither
)

// DatabaseCoverage shows the HA and replication protection of one database.
type DatabaseCoverage struct {
	DatabaseName    string          `json:"database_name"`
	InHA            bool            `json:"in_ha"`
	InReplication   bool            `json:"in_replication"`
	BackupFreshOK   bool            `json:"backup_fresh_ok"`
	ProtectionLevel ProtectionLevel `json:"protection_level"`
}

// ---------------------------------------------------------------------------
// Alerts Timeline
// ---------------------------------------------------------------------------

// AlertEvent is a single event in the HA/Replication alerts timeline.
type AlertEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Severity  string    `json:"severity"` // "info" | "warning" | "critical"
	Category  string    `json:"category"` // "HA" | "Replication"
	Title     string    `json:"title"`
	Detail    string    `json:"detail"`
}

// ---------------------------------------------------------------------------
// Replication KPIs (summary counts)
// ---------------------------------------------------------------------------

// ReplicationKPIs is the aggregate summary of replication configuration.
type ReplicationKPIs struct {
	Publishers       int            `json:"publishers"`
	Subscribers      int            `json:"subscribers"`
	Publications     int            `json:"publications"`
	ReplicatedDBs    int            `json:"replicated_dbs"`
	Articles         int            `json:"articles"`
	IssueCount       int            `json:"issue_count"`
	TypeBreakdown    map[string]int `json:"type_breakdown"`
}

// ---------------------------------------------------------------------------
// Dashboard Summary — single aggregated response for page load
// ---------------------------------------------------------------------------

// BannerInfo is the sticky status banner at the top of the page.
type BannerInfo struct {
	Text  string `json:"text"`
	Color string `json:"color"` // "green" | "yellow" | "red"
}

// KPICards contains the six executive KPI cards (Row 1 of the dashboard).
type KPICards struct {
	PrimaryServer      string       `json:"primary_server"`
	HealthyReplicas    int          `json:"healthy_replicas"`
	TotalReplicas      int          `json:"total_replicas"`
	SyncMode           string       `json:"sync_mode"`
	RPOSeconds         int64        `json:"rpo_seconds"`
	RPOWorst24h        int64        `json:"rpo_worst_24h"`
	RPOThreshold       RPOThreshold `json:"rpo_threshold"`
	EstRTOSeconds      int64        `json:"estimated_rto_seconds"`
	RTOThreshold       RTOThreshold `json:"rto_threshold"`
	FailoverReady      bool         `json:"failover_ready"`
	UptimeHuman        string       `json:"uptime_human"`
	LastFailoverAt     *time.Time   `json:"last_failover_at"`
	LastFailoverSecs   int          `json:"last_failover_secs"`
	ReplicationSummary string       `json:"replication_summary,omitempty"`
}

// DashboardSummary is the single response payload returned by
// GET /api/sqlserver/ha/dashboard-summary.  It drives the initial page render
// in one round-trip.
type DashboardSummary struct {
	Feature          FeatureDetection  `json:"feature"`
	Banner           BannerInfo        `json:"banner"`
	KPIs             KPICards          `json:"kpis"`
	FailoverReadiness FailoverReadiness `json:"failover_readiness"`
}
