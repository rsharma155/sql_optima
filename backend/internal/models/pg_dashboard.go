package models

import "database/sql"

const MaxPgThroughputHistoryMinutes = 30

// PgSession is a detailed row from pg_stat_activity (used for long-running and active query lists).
type PgSession struct {
	PID           int            `json:"pid"`
	UserName      string         `json:"user"`
	Database      string         `json:"database"`
	ClientAddr    string         `json:"client_addr,omitempty"`
	ClientPort    sql.NullInt64  `json:"client_port,omitempty"`
	BackendStart  sql.NullTime   `json:"backend_start,omitempty"`
	QueryStart    sql.NullTime   `json:"query_start,omitempty"`
	StateChange   sql.NullTime   `json:"state_change,omitempty"`
	WaitEventType sql.NullString `json:"wait_event_type,omitempty"`
	WaitEvent     sql.NullString `json:"wait_event,omitempty"`
	State         string         `json:"state"`
	Query         string         `json:"query"`
}

type PgDbCounters struct {
	XactCommit   int64
	XactRollback int64
	BlksRead     int64
	BlksHit      int64
}

type PgThroughputMinuteAgg struct {
	TxnDelta      int64
	BlksReadDelta int64
	BlksHitDelta  int64
}

// PgThroughputPoint stores both chart-ready values and raw deltas.
// Raw deltas are used for aggregating multiple DBs (database=all).
type PgThroughputPoint struct {
	Tps         float64 `json:"tps"`
	CacheHitPct float64 `json:"cache_hit_pct"`

	// For server-side aggregation.
	TxnDelta      int64 `json:"-"`
	BlksReadDelta int64 `json:"-"`
	BlksHitDelta  int64 `json:"-"`
}

// PgCoreDashboardCache is the in-memory, scrape-to-scrape cache backing the Postgres dashboard.
// It keeps previous pg_stat_database counters to compute deltas between polls.
type PgCoreDashboardCache struct {
	InstanceName string `json:"instance_name"`
	Timestamp    string `json:"timestamp"`

	// For delta computation.
	LastPollUnixMs int64                   `json:"-"`
	PrevDbCounters map[string]PgDbCounters `json:"-"`

	// Minute bucketing (we downsample into 1-minute points).
	AggMinuteKey string                           `json:"-"`
	AggByDB      map[string]PgThroughputMinuteAgg `json:"-"`

	// History buffers are stored per DB.
	MinuteKeys     []string                       `json:"-"`
	HistoryByDB    map[string][]PgThroughputPoint `json:"-"`
	KnownDatabases map[string]struct{}            `json:"-"`
}

type PgReplicationStat struct {
	ReplicaPodName string  `json:"replica_pod_name"`
	PodIP          string  `json:"pod_ip"`
	State          string  `json:"state"`
	SyncState      string  `json:"sync_state"`
	ReplayLagMB    float64 `json:"replay_lag_mb"`
}

type PgReplicationStats struct {
	IsPrimary      bool                `json:"is_primary"`
	LocalLagMB     float64             `json:"local_lag_mb"`
	ClusterState   string              `json:"cluster_state"`
	MaxLagMB       float64             `json:"max_lag_mb"`
	WalGenRateMBps float64             `json:"wal_gen_rate_mbps"`
	BgWriterEffPct float64             `json:"bg_writer_eff_pct"`
	Standbys       []PgReplicationStat `json:"standbys"`
}
type PgConfigSetting struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Unit     string `json:"unit"`
	Category string `json:"category"`
	Source   string `json:"source"`
	BootVal  string `json:"boot_val"`
	ResetVal string `json:"reset_val"`
}

type PgAlert struct {
	Severity   string `json:"severity"`
	Metric     string `json:"metric"`
	Threshold  string `json:"threshold"`
	CurrentVal string `json:"current_val"`
	Timestamp  string `json:"timestamp"`
	Status     string `json:"status"`
}
type PgThroughputDashboardResponse struct {
	InstanceName string    `json:"instance_name"`
	DatabaseName string    `json:"database_name"`
	Timestamp    string    `json:"timestamp,omitempty"`
	Labels       []string  `json:"labels"`
	Tps          []float64 `json:"tps"`
	CacheHitPct  []float64 `json:"cache_hit_pct"`
}
