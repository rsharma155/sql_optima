package models

type GlobalInstanceMetric struct {
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	CPUUsage  float64 `json:"cpu"`
	MemoryPct float64 `json:"mem"`
	Status    int     `json:"alertLvl"`        // 0 = Healthy, 1 = Warning, 2 = Critical (Offline/Down)
	Error     string  `json:"error,omitempty"` // Trace if offline
}

// BestPracticesResult contains the complete best practices audit results
type BestPracticesResult struct {
	InstanceName         string                `json:"instance_name"`
	ServerConfig         []ServerConfigCheck   `json:"server_config"`
	DatabaseConfig       []DatabaseConfigCheck `json:"database_config"`
	Timestamp            string                `json:"timestamp"`
	DataSource           string                `json:"data_source,omitempty"`            // "live" | "timescale" (pg_settings values overlaid from latest postgres_settings_snapshot)
	SnapshotCapturedAt   string                `json:"snapshot_captured_at,omitempty"` // RFC3339 when DataSource is timescale
}

// ServerConfigCheck represents a server-level configuration check
type ServerConfigCheck struct {
	ConfigurationName string `json:"configuration_name"`
	Category          string `json:"category,omitempty"` // UI grouping (Memory, Connections, …)
	CurrentValue      string `json:"current_value"`
	DefaultValue      string `json:"default_value,omitempty"` // pg_settings reset_val (or boot_val); drift vs live setting
	Status            string `json:"status"`                  // "GREEN", "YELLOW", "RED"
	Message           string `json:"message"`
	RemediationSQL    string `json:"remediation_sql,omitempty"` // optional ALTER SYSTEM-style hint when not GREEN
}

// DatabaseConfigCheck represents a database-level configuration check
type DatabaseConfigCheck struct {
	DatabaseName       string `json:"database_name"`
	PageVerify         string `json:"page_verify"`
	AutoShrink         bool   `json:"auto_shrink"`
	AutoClose          bool   `json:"auto_close"`
	TargetRecoveryTime int    `json:"target_recovery_time"`
	Status             string `json:"status"` // "GREEN", "YELLOW", "RED"
	Message            string `json:"message"`
}

// ============================================
// SQL SERVER WORKLOAD GUARDRAILS MODELS
// ============================================

// GuardrailsResult contains the complete guardrails audit results
type GuardrailsResult struct {
	InstanceName string           `json:"instance_name"`
	Timestamp    string           `json:"timestamp"`
	HealthScore  int              `json:"health_score"`
	HealthStatus string           `json:"health_status"`
	StorageRisks []StorageRisk    `json:"storage_risks"`
	DiskSpace    []DiskSpaceInfo  `json:"disk_space"`
	LogHealth    []LogHealthInfo  `json:"log_health"`
	LogBackups   []LogBackupInfo  `json:"log_backups"`
	LongTxns     []LongTxnInfo    `json:"long_transactions"`
	Autogrowth   []AutogrowthInfo `json:"autogrowth_risks"`
	TempDBConfig TempDBInfo       `json:"tempdb_config"`
	ResourceGov  ResourceGovInfo  `json:"resource_governor"`
	Summary      []RiskSummary    `json:"summary"`
}

// StorageRisk represents database files on risky paths
type StorageRisk struct {
	DatabaseName  string `json:"database_name"`
	FileType      string `json:"file_type"`
	LogicalName   string `json:"logical_name"`
	PhysicalName  string `json:"physical_name"`
	SizeMB        int    `json:"size_mb"`
	Path          string `json:"path"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	DrillDown     string `json:"drill_down"`
	MitigationSQL string `json:"mitigation_sql"`
}

// DiskSpaceInfo represents drive space information
type DiskSpaceInfo struct {
	DriveLetter              string  `json:"drive_letter"`
	FreeSpaceMB              int     `json:"free_space_mb"`
	TotalSizeMB              int     `json:"total_size_mb"`
	FreePercent              float64 `json:"free_percent"`
	Severity                 string  `json:"severity"`
	LogSizeMB                int     `json:"log_size_mb"`
	LogGrowthGreaterThanFree bool    `json:"log_growth_greater_than_free"`
	Message                  string  `json:"message"`
	DrillDown                string  `json:"drill_down"`
	MitigationSQL            string  `json:"mitigation_sql"`
}

// LogHealthInfo represents transaction log health
type LogHealthInfo struct {
	DatabaseName    string  `json:"database_name"`
	RecoveryModel   string  `json:"recovery_model"`
	LogReuseWait    string  `json:"log_reuse_wait_desc"`
	LogSizeMB       float64 `json:"log_size_mb"`
	LogSpaceUsedPct float64 `json:"log_space_used_pct"`
	VLFCount        int     `json:"vlf_count"`
	Severity        string  `json:"severity"`
	Message         string  `json:"message"`
	DrillDown       string  `json:"drill_down"`
	MitigationSQL   string  `json:"mitigation_sql"`
}

// LogBackupInfo represents log backup compliance
type LogBackupInfo struct {
	DatabaseName  string `json:"database_name"`
	LastBackup    string `json:"last_log_backup"`
	MinutesAgo    int    `json:"minutes_ago"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	DrillDown     string `json:"drill_down"`
	MitigationSQL string `json:"mitigation_sql"`
}

// LongTxnInfo represents long running transactions
type LongTxnInfo struct {
	SessionID         int    `json:"session_id"`
	LoginName         string `json:"login_name"`
	DatabaseName      string `json:"database_name"`
	Status            string `json:"status"`
	CPUTime           int    `json:"cpu_time"`
	ElapsedSeconds    int    `json:"elapsed_seconds"`
	LogicalReads      int64  `json:"logical_reads"`
	Writes            int    `json:"writes"`
	BlockingSessionID int    `json:"blocking_session_id"`
	IsOrphaned        bool   `json:"is_orphaned"`
	Severity          string `json:"severity"`
	Message           string `json:"message"`
	DrillDown         string `json:"drill_down"`
	MitigationSQL     string `json:"mitigation_sql"`
}

// AutogrowthInfo represents autogrowth configuration risks
type AutogrowthInfo struct {
	DatabaseName    string `json:"database_name"`
	FileType        string `json:"file_type"`
	LogicalName     string `json:"logical_name"`
	IsPercentGrowth bool   `json:"is_percent_growth"`
	Growth          int    `json:"growth"`
	Severity        string `json:"severity"`
	Message         string `json:"message"`
	DrillDown       string `json:"drill_down"`
	MitigationSQL   string `json:"mitigation_sql"`
}

// TempDBInfo represents TempDB configuration
type TempDBInfo struct {
	FileCount     int          `json:"file_count"`
	TotalSizeMB   int          `json:"total_size_mb"`
	Files         []TempDBFile `json:"files"`
	Severity      string       `json:"severity"`
	Message       string       `json:"message"`
	DrillDown     string       `json:"drill_down"`
	MitigationSQL string       `json:"mitigation_sql"`
}

// TempDBFile represents a TempDB data file
type TempDBFile struct {
	LogicalName string `json:"logical_name"`
	SizeMB      int    `json:"size_mb"`
}

// ResourceGovInfo represents Resource Governor status
type ResourceGovInfo struct {
	IsEnabled     bool   `json:"is_enabled"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	DrillDown     string `json:"drill_down"`
	MitigationSQL string `json:"mitigation_sql"`
}

// RiskSummary represents a summary of risks
type RiskSummary struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
	Critical int    `json:"critical"`
	Warning  int    `json:"warning"`
	Severity string `json:"severity"`
}
