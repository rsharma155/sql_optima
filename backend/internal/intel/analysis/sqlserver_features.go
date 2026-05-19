// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package analysis


type FeatureThreshold struct {
	Warning  interface{} `json:"warning"`
	Critical interface{} `json:"critical"`
}

type SQLServerFeature struct {
	Name             string                      `json:"name"`
	Checks           []string                    `json:"checks"`
	Thresholds       map[string]FeatureThreshold `json:"thresholds"`
	RecommendedValues map[string]string          `json:"recommended_values"`
	CommonIssues     []string                    `json:"common_issues"`
}

var SQLServerFeatures = map[string]SQLServerFeature{
	"backup_restore": {
		Name:   "Backup & Restore",
		Checks: []string{"full_backup_frequency", "log_backup_frequency", "differential_backup_frequency", "backup_destination_space", "backup_compression_enabled", "backup_checksum_enabled", "backup_verification_enabled", "recovery_model_check", "backup_retention_days", "restore_test_last_successful"},
		Thresholds: map[string]FeatureThreshold{
			"full_backup_max_age_hours":      {Warning: 24, Critical: 72},
			"log_backup_max_age_minutes":     {Warning: 15, Critical: 60},
			"backup_destination_free_pct":    {Warning: 20, Critical: 10},
			"backup_retention_min_days":      {Warning: 30, Critical: 7},
			"differential_backup_max_age_hours": {Warning: 24, Critical: 48},
		},
		RecommendedValues: map[string]string{
			"full_backup":        "Daily (weekly for very large DBs with differentials)",
			"log_backup":         "Every 15-30 minutes for FULL recovery model",
			"differential_backup": "Every 6-8 hours between full backups",
			"backup_compression": "Enabled (saves 60-80% space)",
			"backup_checksum":    "Enabled",
			"recovery_model":     "FULL for production, SIMPLE for dev/test",
			"retention":          "30 days on disk, 90 days to archive",
		},
		CommonIssues: []string{
			"Log file grows unexpectedly when log backups are missed in FULL recovery model",
			"Backup failures due to insufficient disk space",
			"Corrupt backups without checksum validation",
			"Slow restore times due to no backup compression",
		},
	},
	"rto_rpo": {
		Name:   "RTO & RPO",
		Checks: []string{"rto_target_seconds", "rpo_target_seconds", "actual_rto_seconds", "actual_rpo_seconds", "rto_compliance_pct", "rpo_compliance_pct", "failover_duration_seconds", "estimated_data_loss_bytes"},
		Thresholds: map[string]FeatureThreshold{
			"rto_compliance_pct":  {Warning: 90, Critical: 80},
			"rpo_compliance_pct":  {Warning: 90, Critical: 80},
			"actual_rpo_seconds":  {Warning: 300, Critical: 900},
			"actual_rto_seconds":  {Warning: 600, Critical: 1800},
		},
		RecommendedValues: map[string]string{
			"rto":              "5 minutes for mission-critical, 1 hour for business-critical, 4 hours for standard",
			"rpo":              "1 second for mission-critical (AG sync), 5 minutes for critical (async), 1 hour for standard",
			"failover_duration": "Under 30 seconds for automatic failover",
		},
		CommonIssues: []string{
			"RPO breach during peak hours when log generation exceeds replication throughput",
			"RTO breach when failover requires manual intervention",
			"Inconsistent RPO expectations with async replication",
		},
	},
	"hadr_always_on": {
		Name:   "Always On Availability Groups",
		Checks: []string{"synchronization_health", "replica_connected", "log_send_queue_size", "redo_queue_size", "secondary_lag_seconds", "failover_readiness", "quorum_state", "automatic_failover_enabled", "primary_role_only_config", "backup_preference", "readable_secondary", "long_running_transactions"},
		Thresholds: map[string]FeatureThreshold{
			"log_send_queue_kb":    {Warning: 50000, Critical: 200000},
			"redo_queue_kb":        {Warning: 50000, Critical: 200000},
			"secondary_lag_seconds": {Warning: 30, Critical: 120},
			"long_running_tx_count": {Warning: 5, Critical: 15},
			"quorum_state":         {Warning: "WEAK_QUORUM", Critical: "NO_QUORUM"},
		},
		RecommendedValues: map[string]string{
			"replicas":          "At least 2 (primary + 1 secondary) for HA",
			"failover_mode":     "AUTOMATIC for primary to first synchronous secondary",
			"backup_preference": "Secondary preferred",
			"readable_secondary": "Read-intent only for reporting",
			"compression":       "Enable log stream compression for async replicas",
		},
		CommonIssues: []string{
			"Log send queue grows when network bandwidth insufficient",
			"Redo queue blocks when secondary has I/O pressure",
			"Quorum loss when witness fails at same time as replica",
			"Long-running transactions on primary block log truncation and increase log send queue",
		},
	},
	"query_store": {
		Name:   "Query Store",
		Checks: []string{"query_store_enabled", "query_store_capture_mode", "query_store_max_size_mb", "query_store_cleanup_policy", "query_store_data_flush_interval", "total_query_regressions", "plan_changes_count", "wait_stats_capture_enabled"},
		Thresholds: map[string]FeatureThreshold{
			"query_store_max_size_mb": {Warning: 500, Critical: 200},
			"total_regressions":       {Warning: 5, Critical: 20},
			"plan_changes_per_hour":   {Warning: 10, Critical: 50},
			"query_store_enabled":     {Warning: false, Critical: false},
		},
		RecommendedValues: map[string]string{
			"capture_mode":       "AUTO (filter infrequent queries)",
			"max_size_mb":        "1000-2000 MB for OLTP",
			"cleanup_policy":     "30 days retention",
			"data_flush_interval": "900 seconds (15 minutes)",
			"interval_length_minutes": "60",
			"wait_stats_capture": "Enabled (2017+)",
		},
		CommonIssues: []string{
			"Query Store fills up and switches to READ_ONLY mode",
			"Performance overhead from capturing too many plans (use AUTO mode)",
			"Plan forcing with FORCED plan can cause regressions if data distribution changes",
		},
	},
	"resource_governor": {
		Name:   "Resource Governor",
		Checks: []string{"resource_governor_enabled", "classifier_function_exists", "pool_min_memory_pct", "pool_max_memory_pct", "pool_cap_cpu_pct", "pool_min_cpu_pct", "group_max_dop", "group_max_request_grant_memory_pct", "group_importance", "group_request_max_memory_grant_pct"},
		Thresholds: map[string]FeatureThreshold{
			"pool_cap_cpu_pct":                {Warning: 100, Critical: 100},
			"pool_min_memory_pct_sum":         {Warning: 80, Critical: 100},
			"group_request_max_memory_grant_pct": {Warning: 25, Critical: 50},
		},
		RecommendedValues: map[string]string{
			"classifier_function": "Use to route workloads to specific pools",
			"admin_pool":          "Reserve 5-10% CPU for admin queries",
			"reporting_pool":      "Cap at 50% CPU to protect OLTP",
			"oltp_pool":           "High importance, min 30% CPU",
		},
		CommonIssues: []string{
			"Misconfigured classifier function routes wrong workloads",
			"Pool CPU caps cause unrestible performance degradation during peak",
			"Memory grant percents too high allow single query to exhaust pool",
		},
	},
	"log_shipping": {
		Name:   "Log Shipping",
		Checks: []string{"log_shipping_enabled", "backup_job_running", "copy_job_running", "restore_job_running", "log_shipping_lag_minutes", "last_backup_time", "last_copy_time", "last_restore_time", "backup_directory_free_space", "alert_threshold"},
		Thresholds: map[string]FeatureThreshold{
			"log_shipping_lag_minutes":  {Warning: 30, Critical: 120},
			"backup_frequency_minutes":  {Warning: 5, Critical: 15},
			"backup_directory_free_pct": {Warning: 20, Critical: 10},
			"restore_job_failures":      {Warning: 1, Critical: 3},
		},
		RecommendedValues: map[string]string{
			"backup_interval":  "Every 5-15 minutes",
			"restore_delay":    "30-60 minutes (to allow recovery) or 0 for standby",
			"alert_threshold":  "Alert if lag exceeds backup_interval * 3",
			"disaster_recovery": "Combine with AG for HA + DR",
		},
		CommonIssues: []string{
			"Restore job falls behind during heavy write activity",
			"Backup directory fills up and jobs fail",
			"Restore with NORECOVERY blocks application connection attempts",
			"Secondary database enters SUSPECT state after failed restore",
		},
	},
	"replication": {
		Name:   "Replication (Transactional)",
		Checks: []string{"distributor_configured", "publisher_working", "subscriber_connected", "distribution_agent_running", "log_reader_agent_running", "undistributed_commands", "latency_seconds", "delivery_rate_cmds_sec", "replication_type"},
		Thresholds: map[string]FeatureThreshold{
			"undistributed_commands":          {Warning: 1000, Critical: 10000},
			"latency_seconds":                 {Warning: 30, Critical: 120},
			"log_reader_agent_lag_seconds":    {Warning: 30, Critical: 120},
			"distribution_cleanup_retention_hours": {Warning: 72, Critical: 168},
		},
		RecommendedValues: map[string]string{
			"distribution_retention": "48-72 hours",
			"agent_profile":          "High throughput for large publications",
			"conflict_resolution":    "Publisher wins for most scenarios",
			"subscription_type":      "Pull subscriptions for many subscribers",
		},
		CommonIssues: []string{
			"Undistributed commands backlog grows when subscriber is slow",
			"Log reader agent blocks log truncation on publisher",
			"Schema changes require re-initialization",
			"Distribution database grows without proper cleanup",
		},
	},
	"index_management": {
		Name:   "Index Management",
		Checks: []string{"avg_fragmentation_pct", "page_count", "fill_factor", "index_usage_stats", "missing_index_impact", "duplicate_indexes", "unused_indexes", "index_columns_count", "included_columns"},
		Thresholds: map[string]FeatureThreshold{
			"avg_fragmentation_pct":  {Warning: 30, Critical: 50},
			"page_count_for_rebuild": {Warning: 1000, Critical: 10000},
			"fill_factor_low":        {Warning: 75, Critical: 50},
			"unused_index_percent":   {Warning: 20, Critical: 30},
		},
		RecommendedValues: map[string]string{
			"rebuild_threshold":            ">30% fragmentation, rebuild offline if possible",
			"reorganize_threshold":         "5-30% fragmentation, always online",
			"fill_factor":                  "90-100 for OLTP with moderate updates, 80-90 for high-update tables",
			"max_degree_of_parallelism_rebuild": "Use MAXDOP=2 for index rebuilds (less log)",
			"online_rebuild":               "SQL Server Enterprise Edition required",
		},
		CommonIssues: []string{
			"High fragmentation causes page splitting and I/O amplification",
			"Low fill factor wastes space and increases I/O",
			"Rebuilding indexes during peak hours causes blocking",
			"Missing indexes cause table scans (visible in dm_db_missing_index_details)",
		},
	},
	"statistics": {
		Name:   "Statistics Management",
		Checks: []string{"stats_update_date", "stats_modification_counter", "auto_update_stats_enabled", "auto_create_stats_enabled", "stats_sampling_rate", "stats_columns_count", "stats_no_recompute_count", "stats_update_on_primary_only"},
		Thresholds: map[string]FeatureThreshold{
			"stats_age_days":           {Warning: 7, Critical: 30},
			"stats_modification_pct":   {Warning: 20, Critical: 50},
			"stats_sampling_rate_below": {Warning: 5, Critical: 1},
			"stats_no_recompute_count": {Warning: 10, Critical: 50},
		},
		RecommendedValues: map[string]string{
			"auto_update_stats":  "Enabled (default)",
			"auto_create_stats":  "Enabled (default)",
			"sampling_rate":      "FULLSCAN or at least 30% for large tables",
			"update_frequency":   "Daily for high-change tables, weekly for moderate",
			"async_stats_update": "Auto-update async for large tables",
		},
		CommonIssues: []string{
			"Stale statistics cause poor cardinality estimation and bad query plans",
			"100% sampling on very large tables takes too long - use RESAMPLE",
			"Incremental statistics not used for partitioned tables",
			"Auto-update triggers during peak hours causing compilation storms",
		},
	},
	"server_maintenance": {
		Name:   "Server Maintenance",
		Checks: []string{"integrity_check_last_run", "integrity_check_frequency", "index_maintenance_last_run", "stats_update_last_run", "sp_updatestats_last_run", "database_consistency_history", "maintenance_jobs_enabled", "maintenance_cleanup_policy"},
		Thresholds: map[string]FeatureThreshold{
			"integrity_check_max_age_days":  {Warning: 7, Critical: 30},
			"index_maintenance_max_age_days": {Warning: 7, Critical: 30},
			"stats_update_max_age_days":     {Warning: 7, Critical: 30},
			"maintenance_job_failures":      {Warning: 1, Critical: 3},
		},
		RecommendedValues: map[string]string{
			"integrity_check":  "Weekly DBCC CHECKDB during low activity",
			"index_maintenance": "Weekly rebuild/reorganize during maintenance window",
			"stats_update":     "Daily for high-change databases",
			"cleanup":          "Purge old backup history and maintenance logs monthly",
		},
		CommonIssues: []string{
			"Integrity checks skipped during busy periods accumulate corruption risk",
			"Index maintenance jobs overlap with production workload",
			"Maintenance jobs fail silently without alerting",
		},
	},
	"sql_agent_jobs": {
		Name:   "SQL Agent Jobs",
		Checks: []string{"job_failures_24h", "job_duration_trend", "job_schedule_adherence", "disabled_critical_jobs", "job_step_without_retry", "long_running_jobs", "job_output_log_cleaned"},
		Thresholds: map[string]FeatureThreshold{
			"job_failures_24h":          {Warning: 1, Critical: 3},
			"job_duration_deviation_pct": {Warning: 50, Critical: 100},
			"critical_jobs_disabled":    {Warning: 1, Critical: 3},
		},
		RecommendedValues: map[string]string{
			"notification":  "Configure notifications for job failure",
			"retry_attempts": "At least 2-3 for critical jobs",
			"logging":       "Write to log table, enable job history",
			"max_history_rows": "Clean up job history every 30 days",
		},
		CommonIssues: []string{
			"Jobs disabled without notification during maintenance forgotten after",
			"Job duration increasing over time indicates performance degradation",
			"No retry configured means transient failure = missed backup",
		},
	},
	"tempdb_configuration": {
		Name:   "TempDB Configuration",
		Checks: []string{"tempdb_data_files_count", "tempdb_equal_file_sizes", "tempdb_initial_size_mb", "tempdb_autogrowth_mb", "tempdb_file_location", "tempdb_growth_rate", "tempdb_contention_wait_type", "version_store_usage_mb"},
		Thresholds: map[string]FeatureThreshold{
			"tempdb_data_files_below":  {Warning: 4, Critical: 2},
			"tempdb_unequal_file_sizes": {Warning: true, Critical: true},
			"tempdb_file_on_c_drive":   {Warning: true, Critical: true},
			"tempdb_growth_pct":        {Warning: 100, Critical: 200},
			"tempdb_used_pct":          {Warning: 70, Critical: 85},
		},
		RecommendedValues: map[string]string{
			"data_files":    "One file per CPU core (up to 8), equal size",
			"initial_size":  "At least 1024 MB per file",
			"autogrowth":    "512 MB for data files, 256 MB for log (not percent!)",
			"file_location": "On dedicated fast storage, NOT on C: drive",
			"log_file":      "One log file, size = 25% of sum of data files",
		},
		CommonIssues: []string{
			"PAGELATCH contention from insufficient data files (classic 2 file guidance is outdated)",
			"Unequal file sizes cause proportional fill algorithm issues",
			"Percent autogrowth leads to uneven file sizes over time",
			"TempDB on slow storage cripples all queries with spills",
		},
	},
	"database_files": {
		Name:   "Database File Configuration",
		Checks: []string{"data_file_autogrowth_type", "data_file_initial_size", "log_file_autogrowth_type", "log_file_size_ratio", "multiple_files_per_filegroup", "file_on_c_drive", "filegroup_configuration", "filestream_config"},
		Thresholds: map[string]FeatureThreshold{
			"autogrowth_by_percent":  {Warning: true, Critical: true},
			"file_on_c_drive":        {Warning: true, Critical: true},
			"log_to_data_ratio_below": {Warning: 10, Critical: 5},
			"log_to_data_ratio_above": {Warning: 50, Critical: 75},
		},
		RecommendedValues: map[string]string{
			"autogrowth":    "Fixed MB (not percent) - 256 MB for data, 128-256 MB for log",
			"log_file_size": "Typically 10-25% of data file size for OLTP",
			"file_placement": "Data on RAID 10/5, Log on dedicated RAID 1/10 SSD",
			"max_file_size": "Set to reasonable max to prevent runaway growth",
		},
		CommonIssues: []string{
			"Percent autogrowth causes frequent small growth events and fragmentation",
			"Data and log files on same volume cause contention during checkpoint",
			"Log file pre-sized too small causes frequent VLF creation",
		},
	},
	"log_file_growth": {
		Name:   "Transaction Log Management",
		Checks: []string{"log_reuse_wait_desc", "vlf_count", "log_file_size_gb", "log_used_pct", "log_growth_rate_mb_per_day", "log_backup_frequency_minutes", "log_autogrowth_events"},
		Thresholds: map[string]FeatureThreshold{
			"vlf_count":                   {Warning: 500, Critical: 1000},
			"log_used_pct":                {Warning: 60, Critical: 80},
			"log_reuse_wait_active_transaction": {Warning: true, Critical: true},
			"log_growth_rate_mb_per_day":   {Warning: 500, Critical: 2000},
			"log_autogrowth_pct":          {Warning: 10, Critical: 25},
		},
		RecommendedValues: map[string]string{
			"vlf_ideal":       "< 100 VLFs for good performance",
			"log_backup":      "Every 15-30 minutes for FULL recovery",
			"log_file_count":  "One log file per database (multiple are NOT used)",
			"log_initial_size": "Pre-size to expected peak to avoid growth events",
		},
		CommonIssues: []string{
			"Too many VLFs from repeated small autogrowth events - shrink and regrow in large chunks",
			"Log file can't truncate because of ACTIVE_TRANSACTION log_reuse_wait",
			"Log disk fills up causing transaction log full errors (9002)",
			"Very large VLFs (from single large growth) slow down crash recovery",
		},
	},
	"partitioning": {
		Name:   "Table Partitioning",
		Checks: []string{"partitioned_tables", "partition_range_alignment", "partition_function_type", "sliding_window_maintenance", "partition_key_data_skew", "partition_index_alignment", "partition_compression", "stale_partition_count"},
		Thresholds: map[string]FeatureThreshold{
			"partition_range_gap":    {Warning: true, Critical: true},
			"data_skew_pct":         {Warning: 30, Critical: 50},
			"stale_partitions_count": {Warning: 5, Critical: 10},
			"uncompressed_partitions": {Warning: true, Critical: false},
		},
		RecommendedValues: map[string]string{
			"partition_count": "50-200 partitions maximum for manageability",
			"sliding_window":  "Implement SWITCH out old partitions, SWITCH in new",
			"partition_key":   "Date/time column for time-series data",
			"compression":     "Enable PAGE compression on historical partitions",
			"alignment":       "Keep indexes aligned with partition scheme",
		},
		CommonIssues: []string{
			"Non-aligned indexes prevent partition SWITCH operations",
			"Outdated statistics on partitioned tables (use INCREMENTAL stats)",
			"Too many partitions (1000+) degrade query performance",
			"Range gaps prevent query optimizer from using partition elimination",
		},
	},
	"columnstore": {
		Name:   "Columnstore Indexes",
		Checks: []string{"columnstore_version", "compression_ratio", "segment_quality", "deleted_rows_pct", "rowgroup_compression", "open_rowgroups_count", "columnstore_segment_skew"},
		Thresholds: map[string]FeatureThreshold{
			"deleted_rows_pct":            {Warning: 10, Critical: 20},
			"open_rowgroups_below_1m_rows": {Warning: true, Critical: false},
			"segment_compression_below_10x": {Warning: 10, Critical: 5},
			"rowgroup_size_below_100k":    {Warning: true, Critical: false},
		},
		RecommendedValues: map[string]string{
			"use_case":     "Data warehouse / analytics workloads, NOT OLTP",
			"rowgroup_size": "Aim for ~1M rows per rowgroup (102400-1048576)",
			"compression":  "COLUMNSTORE_ARCHIVE for historical partitions",
			"batch_mode":   "Use batch mode execution for best performance",
		},
		CommonIssues: []string{
			"Too many deletes cause fragmentation (columnstore is append-optimized)",
			"Small rowgroups (<100K rows) reduce compression and scan performance",
			"Nonclustered columnstore on OLTP tables adds overhead without benefit",
			"Tuple mover may not keep up with high insert rates",
		},
	},
	"cdc": {
		Name:   "Change Data Capture",
		Checks: []string{"cdc_enabled", "cdc_capture_job_running", "cdc_cleanup_job_running", "cdc_latency_seconds", "cdc_log_space_usage", "cdc_schema_version", "cdc_retention_days"},
		Thresholds: map[string]FeatureThreshold{
			"cdc_latency_seconds":   {Warning: 30, Critical: 120},
			"cdc_log_space_pct":     {Warning: 30, Critical: 50},
			"cdc_cleanup_failures":  {Warning: 1, Critical: 3},
			"cdc_retention_below_hours": {Warning: 24, Critical: 6},
		},
		RecommendedValues: map[string]string{
			"retention":           "72 hours for operational, 7 days for audit",
			"capture_job_interval": "Run every 5-15 seconds for low latency",
			"cleanup_job_interval": "Run every 30 minutes",
			"max_changes_to_scan": "10000 per cycle for high volume",
		},
		CommonIssues: []string{
			"Transaction log can't truncate when capture job is not running",
			"CDC cleanup job falls behind, cdc table grows unbounded",
			"Schema changes require CDC to be disabled and re-enabled",
			"High latency from capture job not keeping up with changes",
		},
	},
	"memory_configuration": {
		Name:   "Memory Configuration",
		Checks: []string{"max_server_memory_mb", "min_server_memory_mb", "lock_pages_in_memory_enabled", "ple_seconds", "buffer_pool_extension_enabled", "memory_grants_pending", "total_physical_memory_mb", "available_physical_memory_mb"},
		Thresholds: map[string]FeatureThreshold{
			"max_server_memory_pct_of_total": {Warning: 90, Critical: 95},
			"available_physical_memory_mb_below": {Warning: 1024, Critical: 512},
			"ple_seconds_below":              {Warning: 600, Critical: 300},
			"lock_pages_in_memory_missing":   {Warning: true, Critical: false},
		},
		RecommendedValues: map[string]string{
			"max_server_memory": "Leave 1-2 GB for OS + 1 GB per 16 GB for SQL up to 128 GB",
			"headroom_formula":  "OS Reserve = 4 GB for < 64 GB, 8 GB for 64-256 GB, 16 GB for > 256 GB",
			"lock_pages_in_memory": "Enabled (prevents paging of SQL buffer pool)",
		},
		CommonIssues: []string{
			"Max server memory too high causes OS memory pressure and paging",
			"PLE below 300 seconds indicates insufficient buffer pool",
			"Memory grants pending means queries waiting for memory workspace",
			"Lock pages in memory not configured causes buffer pool paging",
		},
	},
	"parallelism": {
		Name:   "Parallelism Configuration",
		Checks: []string{"maxdop_setting", "cost_threshold_for_parallelism", "cxpacket_wait_pct", "parallel_query_pct", "max_dop_for_primary", "max_dop_for_secondary_readonly"},
		Thresholds: map[string]FeatureThreshold{
			"maxdop_too_high":            {Warning: 8, Critical: 0},
			"cost_threshold_too_low":     {Warning: 5, Critical: 1},
			"cxpacket_wait_pct_above":    {Warning: 15, Critical: 30},
			"maxdop_set_to_default_0":    {Warning: true, Critical: false},
		},
		RecommendedValues: map[string]string{
			"maxdop_formula":             "For NUMA: cores_per_NUMA_node. For 8+ cores: 8. For <=8: number of cores",
			"cost_threshold_for_parallelism": "25-50 (default 5 is too low for most workloads)",
			"maxdop_for_secondary":       "= number of cores on secondary for readable replica",
		},
		CommonIssues: []string{
			"MAXDOP=0 on >8 cores causes CXPACKET waits and thread oversubscription",
			"Cost threshold for parallelism at 5 causes parallel plans for trivial queries",
			"CXPACKET waits indicate unbalanced parallel execution (data skew)",
		},
	},
	"tempdb_contention": {
		Name:   "TempDB Contention",
		Checks: []string{"pagelatch_contention_detected", "page_latch_wait_pct", "tempdb_data_files_count", "global_allocation_map_contention", "shared_global_allocation_map_contention", "page_free_space_contention"},
		Thresholds: map[string]FeatureThreshold{
			"tempdb_pagelatch_pct_above": {Warning: 20, Critical: 40},
			"files_below_cpu_cores":      {Warning: true, Critical: false},
			"gma_contention":             {Warning: true, Critical: false},
		},
		RecommendedValues: map[string]string{
			"file_count":    "1 per CPU core, up to 8. More than 8 gives diminishing returns",
			"trace_flags":   "TF 1117 (equal autogrowth), TF 1118 (uniform extent allocation)",
			"all_same_size": "ALL data files must be IDENTICAL initial size and autogrowth",
		},
		CommonIssues: []string{
			"PAGELATCH_UP contention on global allocation pages (GAM, SGAM, PFS)",
			"PAGELATCH_EX on page free space (PFS) pages from concurrent temp table creation",
			"Unequal file sizes cause hot files - proportional fill favors largest file",
		},
	},
	"security": {
		Name:   "Security Configuration",
		Checks: []string{"contained_database_authentication", "tde_enabled", "always_encrypted_enabled", "audit_specification", "server_audit_enabled", "login_audit_level"},
		Thresholds: map[string]FeatureThreshold{
			"failed_logins_5m":   {Warning: 10, Critical: 50},
			"contained_dbs_disabled": {Warning: false, Critical: false},
			"audit_disabled":     {Warning: true, Critical: false},
		},
		RecommendedValues: map[string]string{
			"tde":              "Enable for databases with sensitive data",
			"contained_database": "Enable contained database authentication for Azure migration readiness",
			"audit":            "Configure SQL Server Audit for login failures and permissions changes",
			"always_encrypted": "Use for column-level encryption of PII data",
		},
		CommonIssues: []string{
			"TDE enabled without backing up certificates causes unrecoverable database",
			"Contained databases allow password authentication without SQL Server login",
			"Missing audit configurations violate compliance requirements",
		},
	},
	"filestream": {
		Name:   "Filestream / Filetable",
		Checks: []string{"filestream_enabled", "filestream_access_level", "filestream_disk_space", "filestream_backup_strategy", "filetable_enabled", "filestream_garbage_collection"},
		Thresholds: map[string]FeatureThreshold{
			"filestream_disk_free_pct":  {Warning: 20, Critical: 10},
			"filestream_missing_backup": {Warning: true, Critical: false},
			"garbage_collection_lag_days": {Warning: 7, Critical: 30},
		},
		RecommendedValues: map[string]string{
			"access_level": "FILESTREAM access at TSQL level only unless file system access needed",
			"backup":       "FILESTREAM data is included in database backup - no separate backup needed",
			"filetable":    "Enable FILESTREAM for FileTable namespace with Windows compatibility",
		},
		CommonIssues: []string{
			"Transaction log grows large during FILESTREAM garbage collection",
			"FILESTREAM container fills up and cannot be easily grown",
			"Backup/restore of large FILESTREAM databases takes very long",
		},
	},
	"full_text_search": {
		Name:   "Full-Text Search",
		Checks: []string{"full_text_catalogs", "full_text_indexes", "crawl_status", "full_text_index_fragmentation", "thesaurus_config", "stoplist_config", "full_text_change_tracking"},
		Thresholds: map[string]FeatureThreshold{
			"crawl_not_completed":      {Warning: true, Critical: false},
			"index_fragmentation_above": {Warning: 30, Critical: 50},
			"crawl_queue_backlog":      {Warning: 1000, Critical: 10000},
		},
		RecommendedValues: map[string]string{
			"change_tracking": "AUTO for moderate update rate, MANUAL for bulk updates",
			"crawl_frequency": "Schedule incremental crawls during low activity",
			"thesaurus":       "Configure for domain-specific synonym matching",
		},
		CommonIssues: []string{
			"Full-text index crawl degrades during high DML activity",
			"Master merge not occurring causes fragmentation and poor performance",
			"Full-text queries using CONTAINSTABLE can be slower than LIKE for small datasets",
		},
	},
	"sharding": {
		Name:   "Sharding / Elastic Query",
		Checks: []string{"elastic_query_enabled", "shard_map_manager_exists", "external_data_source_configured", "distributed_query_config", "shard_key_distribution", "cross_shard_query_pattern"},
		Thresholds: map[string]FeatureThreshold{
			"shard_skew_pct":        {Warning: 20, Critical: 40},
			"cross_shard_queries_pct": {Warning: 30, Critical: 50},
		},
		RecommendedValues: map[string]string{
			"shard_key":     "Use tenant_id or region for natural distribution",
			"shard_map":     "Use shard map manager on Azure SQL DB or custom routing",
			"query_pattern": "Design queries to use shard key in WHERE clause",
		},
		CommonIssues: []string{
			"Cross-shard queries significantly increase latency",
			"Uneven data distribution causes hot shards",
			"Schema changes across shards require coordinated deployment",
		},
	},
	"in_memory_oltp": {
		Name:   "In-Memory OLTP / Hekaton",
		Checks: []string{"in_memory_tables_count", "memory_optimized_filegroup_exists", "checkpoint_files_disk_space", "durability_checkpoint", "in_memory_procedures_count", "memory_optimized_size_mb"},
		Thresholds: map[string]FeatureThreshold{
			"checkpoint_file_disk_pct":    {Warning: 60, Critical: 80},
			"in_memory_table_size_pct_of_max": {Warning: 70, Critical: 85},
			"checkpoint_pair_file_lag":    {Warning: 1000, Critical: 5000},
		},
		RecommendedValues: map[string]string{
			"max_memory_pct":  "Limit to 20-40% of buffer pool for in-memory tables",
			"durability":       "SCHEMA_AND_DATA for transactional data, SCHEMA_ONLY for staging",
			"checkpoint_files": "Pre-allocate checkpoint file pair on fast SSD",
		},
		CommonIssues: []string{
			"Checkpoint file pair consumption grows and can fill disk",
			"Memory-optimized tables exceed SCHEMA_ONLY durability loses data on restart",
			"Transact-SQL restrictions prevent some operations on in-memory tables",
		},
	},
	"buffer_pool_extension": {
		Name:   "Buffer Pool Extension",
		Checks: []string{"bpe_enabled", "bpe_size_mb", "bpe_file_path", "bpe_eviction_count", "bpe_read_ratio"},
		Thresholds: map[string]FeatureThreshold{
			"bpe_eviction_rate_high": {Warning: true, Critical: false},
			"bpe_read_ratio_below":   {Warning: 50, Critical: 30},
			"bpe_path_on_system_drive": {Warning: true, Critical: false},
		},
		RecommendedValues: map[string]string{
			"bpe_size":      "Up to 4x the buffer pool size, or up to 32 GB",
			"bpe_placement": "Fast SSD/NVMe, NOT system drive",
			"bpe_use_case":  "Servers with limited RAM but fast SSD",
		},
		CommonIssues: []string{
			"BPE on slow storage degrades performance (adds latency)",
			"BPE file too large causes disk space issues",
			"BPE not well-suited for workloads with large memory grants",
		},
	},
}

func GetFeatureByName(name string) *SQLServerFeature {
	f, ok := SQLServerFeatures[name]
	if !ok {
		return nil
	}
	return &f
}

func GetAllFeatureNames() []string {
	names := make([]string, 0, len(SQLServerFeatures))
	for k := range SQLServerFeatures {
		names = append(names, k)
	}
	return names
}

func GetFeatureChecks(featureName string) []string {
	f, ok := SQLServerFeatures[featureName]
	if !ok {
		return nil
	}
	return f.Checks
}

func GetFeatureThreshold(featureName, checkName string) *FeatureThreshold {
	f, ok := SQLServerFeatures[featureName]
	if !ok {
		return nil
	}
	t, ok := f.Thresholds[checkName]
	if !ok {
		return nil
	}
	return &t
}
