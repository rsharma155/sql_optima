package rules

import "strings"

type AdvisoryMeta struct {
	Area         string   `json:"area"`
	Problem      string   `json:"problem"`
	Resource     string   `json:"resource"`
	Symptoms     []string `json:"symptoms"`
	Causes       []string `json:"causes"`
	Impacts      []string `json:"impacts"`
	SeverityHint string   `json:"severity_hint"`
}

var RuleIDMapping = map[string]string{
	"INST_MEM_MAX_001":      "max_server_memory",
	"INST_CPU_MAXDOP_003":   "maxdop",
	"INST_CPU_CTFP_004":     "cost_threshold",
	"INST_PLAN_CACHE_005":   "optimize_adhoc",
	"INST_IFI_006":          "instant_file_init",
	"PG_SHARED_BUFFERS_001": "pg_shared_buffers",
	"PG_AUTOVACUUM_017":     "pg_autovacuum",
	"PG_DEAD_TUPLES_001":    "pg_vacuum_health",
	"HA_AG_REPLICA_017":     "pg_table_bloat",
	"FUNC_INDEX_001":        "pg_missing_index",
}

func GetAdvisoryKey(ruleID string) string {
	if mapped, ok := RuleIDMapping[ruleID]; ok {
		return mapped
	}
	return ruleID
}

var AdvisoryMetadata = map[string]AdvisoryMeta{
	"max_server_memory": {
		Area:         "memory",
		Problem:      "uncontrolled_memory_allocation",
		Resource:     "RAM",
		Symptoms:     []string{"OS paging", "random query slowdowns", "memory pressure"},
		Causes:       []string{"unlimited max_server_memory", "missing memory cap"},
		Impacts:      []string{"OS paging (major performance degradation)", "Random query slowdowns due to memory pressure", "Backup and maintenance jobs failing or slowing down", "Instance instability or crashes in extreme cases"},
		SeverityHint: "critical",
	},
	"maxdop": {
		Area:         "parallelism",
		Problem:      "excessive_parallelism",
		Resource:     "CPU",
		Symptoms:     []string{"high CPU usage", "parallel waits", "query slowdown"},
		Causes:       []string{"high MAXDOP value", "inappropriate parallelism setting"},
		Impacts:      []string{"High CPU usage without performance gain", "Increased context switching and thread contention", "Query latency spikes under load", "Poor scalability as workload increases"},
		SeverityHint: "workload_sensitive",
	},
	"cost_threshold": {
		Area:         "parallelism",
		Problem:      "low_cost_threshold",
		Resource:     "CPU",
		Symptoms:     []string{"excessive parallel queries", "high CPU overhead"},
		Causes:       []string{"cost_threshold set too low"},
		Impacts:      []string{"Excessive parallel queries", "Increased CPU overhead", "Reduced throughput for OLTP workloads", "Unstable performance under concurrency"},
		SeverityHint: "medium",
	},
	"optimize_adhoc": {
		Area:         "query_optimization",
		Problem:      "plan_cache_bloat",
		Resource:     "memory",
		Symptoms:     []string{"plan cache bloat", "memory pressure"},
		Causes:       []string{"optimize_for_adhoc disabled"},
		Impacts:      []string{"Plan cache bloat", "Increased memory pressure", "Reduced efficiency of frequently executed queries", "Potential CPU overhead due to recompilations"},
		SeverityHint: "low",
	},
	"instant_file_init": {
		Area:         "storage",
		Problem:      "delayed_file_initialization",
		Resource:     "disk",
		Symptoms:     []string{"slow file growth", "blocking during autogrowth"},
		Causes:       []string{"instant_file_initialization disabled"},
		Impacts:      []string{"Slow database file growth", "Blocking during autogrowth events", "Slower restores and data loads", "Potential production outages during sudden growth"},
		SeverityHint: "high_in_write_heavy",
	},
	"pg_shared_buffers": {
		Area:         "memory",
		Problem:      "insufficient_buffer_cache",
		Resource:     "memory",
		Symptoms:     []string{"low cache hit ratio", "high disk I/O"},
		Causes:       []string{"shared_buffers too low"},
		Impacts:      []string{"Increased disk reads", "Slower query performance", "Reduced throughput", "Inefficient memory utilization"},
		SeverityHint: "medium",
	},
	"pg_autovacuum": {
		Area:         "storage_maintenance",
		Problem:      "insufficient_vacuum",
		Resource:     "storage",
		Symptoms:     []string{"dead tuples", "table bloat", "transaction wraparound risk"},
		Causes:       []string{"autovacuum disabled or misconfigured"},
		Impacts:      []string{"Table and index bloat", "Slower queries due to scanning dead rows", "Increased storage usage", "Risk of transaction wraparound (can halt writes)"},
		SeverityHint: "critical_if_persistent",
	},
	"pg_vacuum_health": {
		Area:         "storage_maintenance",
		Problem:      "insufficient_vacuum",
		Resource:     "storage",
		Symptoms:     []string{"dead tuples", "table bloat", "stale vacuum"},
		Causes:       []string{"autovacuum not running", "high dead tuple percentage"},
		Impacts:      []string{"Table and index bloat", "Slower queries due to scanning dead rows", "Increased storage usage", "Risk of transaction wraparound (can halt writes)"},
		SeverityHint: "critical",
	},
	"pg_table_bloat": {
		Area:         "storage",
		Problem:      "table_bloat",
		Resource:     "disk",
		Symptoms:     []string{"uncontrolled table growth"},
		Causes:       []string{"insufficient vacuuming", "deleted rows not reclaimed"},
		Impacts:      []string{"Larger disk usage", "Slower sequential scans", "Inefficient index usage", "Increased backup and restore times"},
		SeverityHint: "medium",
	},
	"pg_unused_index": {
		Area:         "storage",
		Problem:      "unused_indexes",
		Resource:     "disk",
		Symptoms:     []string{"unused indexes", "write overhead"},
		Causes:       []string{"indexes not being scanned"},
		Impacts:      []string{"Slower INSERT/UPDATE/DELETE operations", "Increased storage usage", "Longer maintenance windows (vacuum, reindex)"},
		SeverityHint: "medium",
	},
	"pg_cache_hit": {
		Area:         "performance",
		Problem:      "low_cache_efficiency",
		Resource:     "memory",
		Symptoms:     []string{"high disk I/O", "low cache ratio"},
		Causes:       []string{"shared_buffers too low"},
		Impacts:      []string{"Increased disk reads", "Higher latency", "Reduced throughput", "Poor scaling under load"},
		SeverityHint: "high_if_low",
	},
	"pg_missing_index": {
		Area:         "performance",
		Problem:      "missing_indexes",
		Resource:     "CPU",
		Symptoms:     []string{"high sequential scans"},
		Causes:       []string{"queries lacking index coverage"},
		Impacts:      []string{"Full table scans", "Higher CPU usage", "Slower query execution"},
		SeverityHint: "medium",
	},
}

func GenerateWhyThisMatters(ruleID string) string {
	advisoryKey := GetAdvisoryKey(ruleID)
	meta, ok := AdvisoryMetadata[advisoryKey]
	if !ok {
		return ""
	}

	var parts []string

	if meta.Problem != "" {
		parts = append(parts, humanize(meta.Problem)+" affects "+meta.Resource+" utilization.")
	}

	if len(meta.Symptoms) > 0 {
		parts = append(parts, "It is typically observed as "+joinWithAnd(meta.Symptoms)+".")
	}

	if len(meta.Causes) > 0 {
		parts = append(parts, "Common causes include "+joinWithAnd(meta.Causes)+".")
	}

	return strings.Join(parts, " ")
}

func GenerateImpact(ruleID string) string {
	advisoryKey := GetAdvisoryKey(ruleID)
	meta, ok := AdvisoryMetadata[advisoryKey]
	if !ok {
		return ""
	}

	var parts []string

	if len(meta.Impacts) > 0 {
		parts = append(parts, "This can lead to "+joinWithAnd(meta.Impacts)+".")
	}

	return strings.Join(parts, " ")
}

func GetRiskLevel(ruleID string) string {
	advisoryKey := GetAdvisoryKey(ruleID)
	meta, ok := AdvisoryMetadata[advisoryKey]
	if !ok {
		return "MEDIUM"
	}

	switch meta.SeverityHint {
	case "critical", "critical_if_persistent":
		return "HIGH"
	case "high_in_write_heavy":
		return "MEDIUM"
	case "low", "medium", "workload_sensitive":
		return "MEDIUM"
	case "high_if_low":
		return "MEDIUM"
	default:
		return "MEDIUM"
	}
}

func GetConfidenceNote(ruleID string) string {
	advisoryKey := GetAdvisoryKey(ruleID)
	meta, ok := AdvisoryMetadata[advisoryKey]
	if !ok {
		return "Standard confidence based on rule evaluation logic"
	}

	switch meta.SeverityHint {
	case "critical", "critical_if_persistent":
		return "High confidence when dead tuples and vacuum lag are significant"
	case "high_in_write_heavy":
		return "High (binary setting, deterministic behavior)"
	case "workload_sensitive":
		return "Stronger when combined with parallel wait stats (e.g., CXPACKET)"
	case "high_if_low":
		return "High (strong performance indicator)"
	case "medium":
		if meta.Area == "performance" {
			return "Medium unless correlated with actual parallel query usage"
		}
		return "Medium (estimation-based unless using precise tools)"
	default:
		return "Confidence based on rule evaluation logic"
	}
}

func joinWithAnd(items []string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

func humanize(s string) string {
	return strings.ReplaceAll(s, "_", " ")
}
