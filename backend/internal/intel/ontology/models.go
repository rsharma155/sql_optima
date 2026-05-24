// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package ontology




import "github.com/rsharma155/sql_optima/internal/intel/utils"

var OntologyClasses = []string{
	"CPU", "Memory", "Disk", "IO", "QueryPerformance", "Blocking",
	"Locks", "Replication", "Capacity", "Backup", "Network", "TempDB",
	"WaitStats", "IndexHealth", "TransactionLog", "HA",
	"AgentJobs", "MemoryGrant", "Scheduler", "PerformanceDebt", "Settings",
}

type MetricMapping struct {
	SourceTable  string `json:"source_table"`
	SourceColumn string `json:"source_column"`
	OntologyClass string `json:"ontology_class"`
	MetricName   string `json:"metric_name"`
	MetricType   string `json:"metric_type"`
	IsValue      bool   `json:"is_value"`
	IsDimension  bool   `json:"is_dimension"`
	IsTimestamp  bool   `json:"is_timestamp"`
}

type OntologyGraph struct {
	Mappings []MetricMapping
}

func NewOntologyGraph() *OntologyGraph {
	return &OntologyGraph{}
}

func (g *OntologyGraph) AddMapping(m MetricMapping) {
	g.Mappings = append(g.Mappings, m)
}

func (g *OntologyGraph) GetByClass(cls string) []MetricMapping {
	var result []MetricMapping
	for _, m := range g.Mappings {
		if m.OntologyClass == cls {
			result = append(result, m)
		}
	}
	return result
}

func (g *OntologyGraph) GetClasses() []string {
	seen := make(map[string]bool)
	var result []string
	for _, m := range g.Mappings {
		if !seen[m.OntologyClass] {
			seen[m.OntologyClass] = true
			result = append(result, m.OntologyClass)
		}
	}
	return result
}

var OntologySignatures = map[string][]string{
	"CPU":              {"cpu", "processor", "scheduler", "runnable", "signal_wait", "load_", "sql_process", "system_idle"},
	"Memory":           {"memory", "ple", "page_life", "buffer", "grant", "allocat", "workspace", "plan_cache", "shared_buffers"},
	"Disk":             {"disk", "storage", "volume", "drive", "free_disk", "data_disk", "log_disk", "database_size", "data_mb", "log_mb", "free_mb"},
	"IO":               {"io_", "i/o", "latency", "throughput", "iops", "read_latency", "write_latency", "blks_read", "blks_hit"},
	"QueryPerformance": {"query", "execution", "plan", "spill", "duration", "cpu_time", "elapsed_time"},
	"Blocking":         {"block", "deadlock", "wait_resource", "blocking_session"},
	"Locks":            {"lock", "latch", "total_locks"},
	"Replication":      {"replicat", "log_shipping", "sync", "lag", "distributor", "secondary_lag"},
	"Capacity":         {"capacity", "growth", "size", "space_used", "delta_", "projected"},
	"Backup":           {"backup", "restore", "recovery", "checkpoint", "archive_mode"},
	"Network":          {"network", "tcp", "connection", "packet", "async_network"},
	"TempDB":           {"tempdb", "templog", "version_store", "temp_files", "temp_bytes"},
	"WaitStats":        {"wait", "waiting", "signal_wait", "wait_event"},
	"IndexHealth":      {"index", "fragmentation", "page_split", "scan"},
	"TransactionLog":   {"log", "vlf", "log_growth", "log_used"},
	"HA":               {"ag_", "availability_group", "failover", "replica_role", "quorum", "synchronization_health", "is_failover_ready"},
	"AgentJobs":        {"job_", "agent", "sqlagent"},
	"MemoryGrant":      {"memory_grant", "resource_semaphore", "granted_query_memory"},
	"Scheduler":        {"scheduler", "worker_thread", "queued_request", "numa_node"},
	"PerformanceDebt":  {"performance_debt", "finding", "debt"},
	"Settings":         {"settings", "configuration", "pg_settings"},
}

var TableToOntology = map[string]string{
	"sqlserver_metrics":                  "CPU",
	"sqlserver_cpu_history":              "CPU",
	"sqlserver_cpu_scheduler_stats":      "Scheduler",
	"sqlserver_memory_metrics":           "Memory",
	"sqlserver_memory_history":           "Memory",
	"sqlserver_risk_health":             "Memory",
	"sqlserver_disk_history":             "Disk",
	"sqlserver_wait_history":             "WaitStats",
	"sqlserver_waiting_tasks":            "WaitStats",
	"sqlserver_latch_waits":              "WaitStats",
	"sqlserver_lock_history":             "Locks",
	"sqlserver_database_throughput":      "IO",
	"sqlserver_long_running_queries":     "QueryPerformance",
	"sqlserver_procedure_stats":          "QueryPerformance",
	"sqlserver_ha_replica_state":         "HA",
	"sqlserver_ha_database_state":        "HA",
	"sqlserver_ha_failover_events":       "HA",
	"sqlserver_replication_latency":      "Replication",
	"sqlserver_replication_topology":     "Replication",
	"sqlserver_replication_articles":     "Replication",
	"sqlserver_job_metrics":              "AgentJobs",
	"sqlserver_job_details":              "AgentJobs",
	"sqlserver_job_failures":             "AgentJobs",
	"sqlserver_buffer_pool_db":           "Memory",
	"sqlserver_performance_debt_findings":"PerformanceDebt",
	"sqlserver_server_properties":        "Settings",
	"sqlserver_connection_history":       "Network",
	"sqlserver_query_dictionary":         "QueryPerformance",
	"sqlserver_scheduler_wg":             "Scheduler",
	"postgres_wait_event_stats":          "WaitStats",
	"postgres_db_io_stats":              "IO",
	"postgres_settings_snapshot":         "Settings",
	"monitor.pg_os_cpu_samples":          "CPU",
	"monitor.pg_os_memory_samples":      "Memory",
	"monitor.pg_os_memory_pressure":     "Memory",
	"monitor.pg_os_process_memory":      "Memory",
}

var ColumnOntologyMap = map[string]string{
	"avg_cpu_load":                      "CPU",
	"sql_process":                       "CPU",
	"system_idle":                       "CPU",
	"total_runnable_tasks_count":        "CPU",
	"avg_runnable_tasks_count":          "CPU",
	"cpu_user_pct":                      "CPU",
	"cpu_system_pct":                    "CPU",
	"cpu_iowait_pct":                    "CPU",
	"memory_usage":                      "Memory",
	"sql_memory_used_mb":                "Memory",
	"ple":                               "Memory",
	"ple_seconds":                       "Memory",
	"memory_grants_pending":             "MemoryGrant",
	"waiting_memory_grants":             "MemoryGrant",
	"buffer_cache_hit_ratio":            "Memory",
	"data_disk_mb":                      "Disk",
	"log_disk_mb":                       "Disk",
	"free_disk_mb":                      "Disk",
	"data_mb":                           "Disk",
	"free_mb":                           "Disk",
	"tempdb_used_percent":               "TempDB",
	"deadlocks":                         "Locks",
	"total_locks":                       "Locks",
	"blocking_sessions":                 "Blocking",
	"blocking_session_id":               "Blocking",
	"blocking_ms_per_sec":               "Blocking",
	"tps":                               "QueryPerformance",
	"batch_requests_per_sec":            "QueryPerformance",
	"read_latency_ms":                   "IO",
	"write_latency_ms":                  "IO",
	"secondary_lag_seconds":             "Replication",
	"latency_seconds":                   "Replication",
	"log_send_queue_kb":                 "HA",
	"redo_queue_kb":                     "HA",
	"failed_jobs_24h":                   "AgentJobs",
	"sort_warnings_per_sec":             "QueryPerformance",
	"hash_warnings_per_sec":             "QueryPerformance",
	"worker_thread_exhaustion_warning":  "Scheduler",
	"physical_memory_pressure_warning":  "Memory",
	"available_physical_memory_kb":      "Memory",
}

func init() {
	_ = utils.ClassifyMetricName
}