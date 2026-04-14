// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL best practices configuration checker against pg_settings.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

// PgConfigRow represents a single row from pg_settings.
type PgConfigRow struct {
	Name         string
	Setting      string
	Unit         string
	DefaultValue string
}

// FetchPgBestPractices queries pg_settings and evaluates DBA health rules.
func (c *PgRepository) FetchPgBestPractices(instanceName string) models.BestPracticesResult {
	configs, err := c.QueryPgBestPracticesConfigRows(instanceName)
	if err != nil {
		return models.BestPracticesResult{InstanceName: instanceName}
	}
	return c.FetchPgBestPracticesFromConfigs(instanceName, configs)
}

// QueryPgBestPracticesConfigRows loads the curated pg_settings rows used for the DBA audit (live from PostgreSQL).
func (c *PgRepository) QueryPgBestPracticesConfigRows(instanceName string) ([]PgConfigRow, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		log.Printf("[POSTGRES] QueryPgBestPracticesConfigRows: connection not found for %s", instanceName)
		return nil, fmt.Errorf("connection not found")
	}
	return c.queryPgSettings(db)
}

// FetchPgBestPracticesFromConfigs evaluates DBA rules for pre-built config rows (e.g. after overlaying a Timescale snapshot).
func (c *PgRepository) FetchPgBestPracticesFromConfigs(instanceName string, configs []PgConfigRow) models.BestPracticesResult {
	var result models.BestPracticesResult
	result.InstanceName = instanceName
	result.Timestamp = time.Now().UTC().Format(time.RFC3339)
	result.ServerConfig = c.evaluatePgRules(configs)
	return result
}

// queryPgSettings fetches critical PostgreSQL parameters from pg_settings.
func (c *PgRepository) queryPgSettings(db *sql.DB) ([]PgConfigRow, error) {
	// default_value: reset_val is what RESET / postgresql.conf would use; boot_val is bootstrap default.
	// Using reset_val avoids showing the same text as "setting" when the instance is still at file defaults.
	query := `
		SELECT 
			name, 
			setting, 
			unit, 
			COALESCE(NULLIF(BTRIM(reset_val), ''), BTRIM(boot_val)) AS default_value 
		FROM pg_settings 
		WHERE name IN (
			'shared_buffers', 
			'work_mem', 
			'maintenance_work_mem', 
			'max_connections', 
			'random_page_cost', 
			'effective_cache_size',
			'checkpoint_completion_target',
			'checkpoint_timeout',
			'max_wal_size',
			'min_wal_size',
			'wal_keep_size',
			'max_worker_processes',
			'max_parallel_workers',
			'max_parallel_workers_per_gather',
			'autovacuum',
			'autovacuum_max_workers',
			'autovacuum_naptime',
			'autovacuum_vacuum_scale_factor',
			'autovacuum_analyze_scale_factor',
			'wal_buffers',
			'default_statistics_target',
			'log_min_duration_statement'
		)
		ORDER BY name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []PgConfigRow
	for rows.Next() {
		var r PgConfigRow
		var unit sql.NullString
		if err := rows.Scan(&r.Name, &r.Setting, &unit, &r.DefaultValue); err != nil {
			continue
		}
		if unit.Valid {
			r.Unit = unit.String
		}
		configs = append(configs, r)
	}

	return configs, rows.Err()
}

// pgRemediationByParam offers copy-paste-style hints for the pg_settings audit (fallback when Rule Engine is unused).
var pgRemediationByParam = map[string]string{
	"shared_buffers":       "ALTER SYSTEM SET shared_buffers = '256MB';  -- size for your RAM; often requires restart\n-- Then: SELECT pg_reload_conf();  (restart if parameter is not SIGHUP)",
	"work_mem":             "ALTER SYSTEM SET work_mem = '16MB';  -- scale with RAM / max_connections\nSELECT pg_reload_conf();",
	"maintenance_work_mem": "ALTER SYSTEM SET maintenance_work_mem = '512MB';\nSELECT pg_reload_conf();",
	"max_connections":      "ALTER SYSTEM SET max_connections = '200';  -- lower if possible; prefer pooler\nSELECT pg_reload_conf();  -- restart if required",
	"random_page_cost":     "ALTER SYSTEM SET random_page_cost = '1.1';  -- typical for SSD / cloud storage\nSELECT pg_reload_conf();",
	"effective_cache_size": "ALTER SYSTEM SET effective_cache_size = '8GB';  -- ~50–75% of RAM for planner\nSELECT pg_reload_conf();",
	"checkpoint_completion_target": "ALTER SYSTEM SET checkpoint_completion_target = '0.9';\nSELECT pg_reload_conf();",
	"wal_buffers":          "ALTER SYSTEM SET wal_buffers = '16MB';  -- or -1 for auto\nSELECT pg_reload_conf();",
	"autovacuum":           "ALTER SYSTEM SET autovacuum = on;\nSELECT pg_reload_conf();",
	"default_statistics_target": "ALTER SYSTEM SET default_statistics_target = 100;  -- increase for important tables\nSELECT pg_reload_conf();",
	"log_min_duration_statement": "ALTER SYSTEM SET log_min_duration_statement = 1000;  -- ms; tune for workload\nSELECT pg_reload_conf();",
}

func canonicalPgParamName(configurationName string) string {
	name := strings.TrimSpace(configurationName)
	if i := strings.IndexByte(name, ' '); i > 0 {
		return name[:i]
	}
	return name
}

func attachPgRemediationHints(checks []models.ServerConfigCheck) {
	for i := range checks {
		if checks[i].Status == "GREEN" {
			continue
		}
		key := canonicalPgParamName(checks[i].ConfigurationName)
		if hint, ok := pgRemediationByParam[key]; ok {
			checks[i].RemediationSQL = hint
		}
	}
}

// Built-in PostgreSQL defaults for health checks (do not use reset_val for these comparisons:
// when the cluster is tuned, setting and reset_val are often equal, and "setting <= reset" would false-positive).
const (
	pgBuiltinDefaultSharedBuffers = 128 * 1024 * 1024       // 16384 × 8kB pages
	pgBuiltinDefaultWorkMemKB     = 4096                    // 4 MB
	pgBuiltinDefaultMaintWorkMemKB = 65536                  // 64 MB in kB (pg_settings)
	pgBuiltinDefaultEffCache      = 4 * 1024 * 1024 * 1024 // typical shipped default ~4GB
)

// evaluatePgRules applies DBA health checks to PostgreSQL configurations.
func (c *PgRepository) evaluatePgRules(configs []PgConfigRow) []models.ServerConfigCheck {
	var checks []models.ServerConfigCheck

	configMap := make(map[string]PgConfigRow)
	for _, cfg := range configs {
		configMap[cfg.Name] = cfg
	}

	// shared_buffers check
	if cfg, exists := configMap["shared_buffers"]; exists {
		settingBytes := c.parsePgSize(cfg.Setting, cfg.Unit)
		check := models.ServerConfigCheck{
			ConfigurationName: "shared_buffers",
			Category:          "Memory",
			CurrentValue:      c.formatPgSizeHuman(cfg.Setting, cfg.Unit),
			DefaultValue:      c.formatPgSizeHuman(cfg.DefaultValue, cfg.Unit),
			Status:            "GREEN",
			Message:           "shared_buffers is above the small built-in default (128MB). Consider sizing to ~25% of RAM for dedicated DB servers.",
		}

		if settingBytes <= pgBuiltinDefaultSharedBuffers {
			check.Status = "RED"
			check.Message = "shared_buffers is at or below the typical built-in default (~128MB). Raise it (often ~25% of RAM on dedicated hosts; requires restart on many installs)."
		} else if settingBytes < 256*1024*1024 {
			check.Status = "YELLOW"
			check.Message = "shared_buffers is better than default but still modest (<256MB). Many production systems use hundreds of MB to several GB depending on RAM."
		}

		checks = append(checks, check)
	}

	// max_connections check
	if cfg, exists := configMap["max_connections"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "max_connections",
			Category:          "Connections",
			CurrentValue:      cfg.Setting,
			DefaultValue:      cfg.DefaultValue,
			Status:            "GREEN",
			Message:           "Connection limit is within a reasonable range.",
		}

		maxConns, err := strconv.Atoi(cfg.Setting)
		if err == nil && maxConns > 500 {
			check.Status = "YELLOW"
			check.Message = "High connection limit. Postgres uses a heavy process-per-connection model. Consider lowering this and using a connection pooler like PgBouncer."
		}

		checks = append(checks, check)
	}

	// random_page_cost check
	if cfg, exists := configMap["random_page_cost"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "random_page_cost",
			Category:          "Query planner",
			CurrentValue:      cfg.Setting,
			DefaultValue:      cfg.DefaultValue,
			Status:            "GREEN",
			Message:           "Random page cost is optimized for your storage type.",
		}

		rpc, err := strconv.ParseFloat(cfg.Setting, 64)
		if err == nil && rpc >= 4.0 {
			check.Status = "YELLOW"
			check.Message = "Optimized for HDD. If you are using SSDs or Cloud Storage, change this to 1.1 to encourage index usage."
		}

		checks = append(checks, check)
	}

	// work_mem check
	if cfg, exists := configMap["work_mem"]; exists {
		settingKB := c.parsePgSizeKB(cfg.Setting, cfg.Unit)
		check := models.ServerConfigCheck{
			ConfigurationName: "work_mem",
			Category:          "Memory",
			CurrentValue:      c.formatPgSizeHuman(cfg.Setting, cfg.Unit),
			DefaultValue:      c.formatPgSizeHuman(cfg.DefaultValue, cfg.Unit),
			Status:            "GREEN",
			Message:           "work_mem is above the built-in default (4MB). Size per operation; multiply by max parallel workers × connections when reasoning about RAM.",
		}

		if settingKB <= pgBuiltinDefaultWorkMemKB {
			check.Status = "YELLOW"
			check.Message = "work_mem is at or below the built-in default (4MB). Sorts/hashes may spill to disk; increase based on RAM and expected concurrency (careful: it is per operation)."
		}

		checks = append(checks, check)
	}

	// maintenance_work_mem check
	if cfg, exists := configMap["maintenance_work_mem"]; exists {
		settingKB := c.parsePgSizeKB(cfg.Setting, cfg.Unit)
		check := models.ServerConfigCheck{
			ConfigurationName: "maintenance_work_mem",
			Category:          "Memory",
			CurrentValue:      c.formatPgSizeHuman(cfg.Setting, cfg.Unit),
			DefaultValue:      c.formatPgSizeHuman(cfg.DefaultValue, cfg.Unit),
			Status:            "GREEN",
			Message:           "maintenance_work_mem is above the built-in default (64MB).",
		}

		if settingKB <= pgBuiltinDefaultMaintWorkMemKB {
			check.Status = "YELLOW"
			check.Message = "maintenance_work_mem is at or below the built-in default (64MB). VACUUM, CREATE INDEX, and similar operations may be slow; consider hundreds of MB to ~1GB depending on RAM."
		}

		checks = append(checks, check)
	}

	// effective_cache_size check
	if cfg, exists := configMap["effective_cache_size"]; exists {
		settingBytes := c.parsePgSize(cfg.Setting, cfg.Unit)
		check := models.ServerConfigCheck{
			ConfigurationName: "effective_cache_size",
			Category:          "Query planner",
			CurrentValue:      c.formatPgSizeHuman(cfg.Setting, cfg.Unit),
			DefaultValue:      c.formatPgSizeHuman(cfg.DefaultValue, cfg.Unit),
			Status:            "GREEN",
			Message:           "effective_cache_size is above the small built-in default (~4GB). Planner assumes this much OS cache for disk reads.",
		}

		if settingBytes <= pgBuiltinDefaultEffCache {
			check.Status = "YELLOW"
			check.Message = "effective_cache_size is at or below the typical built-in default (~4GB). Raise toward ~50–75% of RAM so the planner weighs index vs sequential scans realistically."
		}

		checks = append(checks, check)
	}

	// effective_cache_size sanity vs shared_buffers (planner cache assumptions)
	if ecs, ok := configMap["effective_cache_size"]; ok {
		if sb, ok2 := configMap["shared_buffers"]; ok2 {
			check := models.ServerConfigCheck{
				ConfigurationName: "effective_cache_size (sanity)",
				Category:          "Query planner",
				CurrentValue:      c.formatPgSizeHuman(ecs.Setting, ecs.Unit),
				DefaultValue:      c.formatPgSizeHuman(ecs.DefaultValue, ecs.Unit),
				Status:            "GREEN",
				Message:           "Effective cache size is consistent with shared_buffers.",
			}
			ecsBytes := c.parsePgSize(ecs.Setting, ecs.Unit)
			sbBytes := c.parsePgSize(sb.Setting, sb.Unit)
			if ecsBytes > 0 && sbBytes > 0 && ecsBytes < (sbBytes*2) {
				check.Status = "YELLOW"
				check.Message = "effective_cache_size is low relative to shared_buffers. Set effective_cache_size to roughly shared_buffers + OS cache (often ~75% RAM) so planner picks better index vs seq-scan plans."
			}
			checks = append(checks, check)
		}
	}

	// checkpoint_completion_target check
	if cfg, exists := configMap["checkpoint_completion_target"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "checkpoint_completion_target",
			Category:          "Checkpoints & WAL",
			CurrentValue:      cfg.Setting,
			DefaultValue:      cfg.DefaultValue,
			Status:            "GREEN",
			Message:           "Checkpoint completion target is well configured.",
		}

		cct, err := strconv.ParseFloat(cfg.Setting, 64)
		if err == nil && cct < 0.9 {
			check.Status = "YELLOW"
			check.Message = "Low checkpoint completion target. Increase to 0.9 to spread checkpoint I/O over a longer period and reduce I/O spikes."
		}

		checks = append(checks, check)
	}

	// checkpoint_timeout check
	if cfg, exists := configMap["checkpoint_timeout"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "checkpoint_timeout",
			Category:          "Checkpoints & WAL",
			CurrentValue:      cfg.Setting + " " + cfg.Unit,
			DefaultValue:      cfg.DefaultValue + " " + cfg.Unit,
			Status:            "GREEN",
			Message:           "Checkpoint timeout is within a reasonable range.",
		}
		// unit is typically "s"
		sec, err := strconv.Atoi(cfg.Setting)
		if err == nil && sec < 300 {
			check.Status = "YELLOW"
			check.Message = "Very frequent checkpoints (<5m). This can increase I/O churn. Consider 5m-15m along with max_wal_size tuning."
		}
		checks = append(checks, check)
	}

	// max_wal_size check
	if cfg, exists := configMap["max_wal_size"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "max_wal_size",
			Category:          "Checkpoints & WAL",
			CurrentValue:      c.formatPgSizeHuman(cfg.Setting, cfg.Unit),
			DefaultValue:      c.formatPgSizeHuman(cfg.DefaultValue, cfg.Unit),
			Status:            "GREEN",
			Message:           "Max WAL size is not dangerously small.",
		}
		b := c.parsePgSize(cfg.Setting, cfg.Unit)
		if b > 0 && b < (1024*1024*1024) { // < 1GB
			check.Status = "YELLOW"
			check.Message = "max_wal_size is low (<1GB). This can force frequent checkpoints and cause write spikes. Consider 2GB-16GB depending on workload."
		}
		checks = append(checks, check)
	}

	// min_wal_size check
	if cfg, exists := configMap["min_wal_size"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "min_wal_size",
			Category:          "Checkpoints & WAL",
			CurrentValue:      c.formatPgSizeHuman(cfg.Setting, cfg.Unit),
			DefaultValue:      c.formatPgSizeHuman(cfg.DefaultValue, cfg.Unit),
			Status:            "GREEN",
			Message:           "Min WAL size looks reasonable.",
		}
		b := c.parsePgSize(cfg.Setting, cfg.Unit)
		if b > 0 && b < (256*1024*1024) { // < 256MB
			check.Status = "YELLOW"
			check.Message = "min_wal_size is very small. This can increase WAL recycling churn. Consider >= 1GB for steady workloads."
		}
		checks = append(checks, check)
	}

	// wal_keep_size check
	if cfg, exists := configMap["wal_keep_size"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "wal_keep_size",
			Category:          "Replication & WAL",
			CurrentValue:      c.formatPgSizeHuman(cfg.Setting, cfg.Unit),
			DefaultValue:      c.formatPgSizeHuman(cfg.DefaultValue, cfg.Unit),
			Status:            "GREEN",
			Message:           "wal_keep_size is set.",
		}
		b := c.parsePgSize(cfg.Setting, cfg.Unit)
		if b <= 0 {
			check.Status = "YELLOW"
			check.Message = "wal_keep_size is 0. If replicas disconnect/lag, they may need re-initialization. Consider setting a retention floor appropriate for your RPO/RTO."
		}
		checks = append(checks, check)
	}

	// max_worker_processes check
	if cfg, exists := configMap["max_worker_processes"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "max_worker_processes",
			Category:          "Parallelism",
			CurrentValue:      cfg.Setting,
			DefaultValue:      cfg.DefaultValue,
			Status:            "GREEN",
			Message:           "Worker processes limit looks ok.",
		}
		n, err := strconv.Atoi(cfg.Setting)
		if err == nil && n < 8 {
			check.Status = "YELLOW"
			check.Message = "max_worker_processes is low. Background workers (autovacuum, parallel query, logical replication) can be constrained. Consider increasing based on CPU cores."
		}
		checks = append(checks, check)
	}

	// max_parallel_workers sanity
	if cfg, exists := configMap["max_parallel_workers"]; exists {
		checks = append(checks, models.ServerConfigCheck{
			ConfigurationName: "max_parallel_workers",
			Category:          "Parallelism",
			CurrentValue:      cfg.Setting,
			DefaultValue:      cfg.DefaultValue,
			Status:            "GREEN",
			Message:           "Parallel workers setting captured for review.",
		})
	}
	if cfg, exists := configMap["max_parallel_workers_per_gather"]; exists {
		checks = append(checks, models.ServerConfigCheck{
			ConfigurationName: "max_parallel_workers_per_gather",
			Category:          "Parallelism",
			CurrentValue:      cfg.Setting,
			DefaultValue:      cfg.DefaultValue,
			Status:            "GREEN",
			Message:           "Parallel per-gather setting captured for review.",
		})
	}

	// autovacuum enabled check
	if cfg, exists := configMap["autovacuum"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "autovacuum",
			Category:          "Autovacuum",
			CurrentValue:      cfg.Setting,
			DefaultValue:      cfg.DefaultValue,
			Status:            "GREEN",
			Message:           "Autovacuum is enabled.",
		}
		if strings.TrimSpace(cfg.Setting) == "off" {
			check.Status = "RED"
			check.Message = "Autovacuum is disabled. This risks table bloat and transaction ID wraparound. Enable autovacuum immediately."
		}
		checks = append(checks, check)
	}

	// autovacuum_max_workers check
	if cfg, exists := configMap["autovacuum_max_workers"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "autovacuum_max_workers",
			Category:          "Autovacuum",
			CurrentValue:      cfg.Setting,
			DefaultValue:      cfg.DefaultValue,
			Status:            "GREEN",
			Message:           "Autovacuum worker count looks ok.",
		}
		n, err := strconv.Atoi(cfg.Setting)
		if err == nil && n < 3 {
			check.Status = "YELLOW"
			check.Message = "autovacuum_max_workers is low. Consider 3-10+ depending on table count and write volume."
		}
		checks = append(checks, check)
	}

	// autovacuum_naptime check (seconds)
	if cfg, exists := configMap["autovacuum_naptime"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "autovacuum_naptime",
			Category:          "Autovacuum",
			CurrentValue:      cfg.Setting + " " + cfg.Unit,
			DefaultValue:      cfg.DefaultValue + " " + cfg.Unit,
			Status:            "GREEN",
			Message:           "Autovacuum naptime looks ok.",
		}
		sec, err := strconv.Atoi(cfg.Setting)
		if err == nil && sec > 60 {
			check.Status = "YELLOW"
			check.Message = "autovacuum_naptime is high (>60s). This can delay vacuum/analyze responsiveness on busy systems."
		}
		checks = append(checks, check)
	}

	// autovacuum scale factor checks
	if cfg, exists := configMap["autovacuum_vacuum_scale_factor"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "autovacuum_vacuum_scale_factor",
			Category:          "Autovacuum",
			CurrentValue:      cfg.Setting,
			DefaultValue:      cfg.DefaultValue,
			Status:            "GREEN",
			Message:           "Vacuum scale factor captured for review.",
		}
		f, err := strconv.ParseFloat(cfg.Setting, 64)
		if err == nil && f >= 0.2 {
			check.Status = "YELLOW"
			check.Message = "High vacuum scale factor (>=0.2). Large tables may bloat before vacuum triggers. Consider per-table tuning or lower scale factors for hot tables."
		}
		checks = append(checks, check)
	}
	if cfg, exists := configMap["autovacuum_analyze_scale_factor"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "autovacuum_analyze_scale_factor",
			Category:          "Autovacuum",
			CurrentValue:      cfg.Setting,
			DefaultValue:      cfg.DefaultValue,
			Status:            "GREEN",
			Message:           "Analyze scale factor captured for review.",
		}
		f, err := strconv.ParseFloat(cfg.Setting, 64)
		if err == nil && f >= 0.2 {
			check.Status = "YELLOW"
			check.Message = "High analyze scale factor (>=0.2). Statistics may become stale on large tables. Consider lowering for frequently updated tables."
		}
		checks = append(checks, check)
	}

	// wal_buffers check
	if cfg, exists := configMap["wal_buffers"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "wal_buffers",
			Category:          "Checkpoints & WAL",
			CurrentValue:      c.formatPgSizeHuman(cfg.Setting, cfg.Unit),
			DefaultValue:      c.formatPgSizeHuman(cfg.DefaultValue, cfg.Unit),
			Status:            "GREEN",
			Message:           "WAL buffers is adequately configured.",
		}

		settingKB := c.parsePgSizeKB(cfg.Setting, cfg.Unit)
		if settingKB < 16384 { // Less than 16MB
			check.Status = "YELLOW"
			check.Message = "WAL buffers is below 16MB. For write-heavy workloads, increase to 16MB-64MB to reduce WAL flush frequency."
		}

		checks = append(checks, check)
	}

	// default_statistics_target check
	if cfg, exists := configMap["default_statistics_target"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "default_statistics_target",
			Category:          "Query planner",
			CurrentValue:      cfg.Setting,
			DefaultValue:      cfg.DefaultValue,
			Status:            "GREEN",
			Message:           "Statistics target is at the recommended default.",
		}

		target, err := strconv.Atoi(cfg.Setting)
		if err == nil && target < 100 {
			check.Status = "YELLOW"
			check.Message = "Low statistics target. Increase to 100-500 for tables with skewed data distributions to improve query plans."
		}

		checks = append(checks, check)
	}

	// log_min_duration_statement check
	if cfg, exists := configMap["log_min_duration_statement"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "log_min_duration_statement",
			Category:          "Logging",
			CurrentValue:      cfg.Setting + "ms",
			DefaultValue:      cfg.DefaultValue + "ms",
			Status:            "GREEN",
			Message:           "Slow query logging is enabled.",
		}

		duration, err := strconv.Atoi(cfg.Setting)
		if err == nil && duration < 0 {
			check.Status = "YELLOW"
			check.Message = "Slow query logging is disabled (-1). Enable it by setting to 250ms or 500ms to identify slow queries."
		} else if err == nil && duration > 5000 {
			check.Status = "YELLOW"
			check.Message = "Slow query threshold is very high. Consider lowering to 500ms-1000ms to catch more problematic queries."
		}

		checks = append(checks, check)
	}

	attachPgRemediationHints(checks)

	// Sort: RED first, then YELLOW, then GREEN
	// (handled by frontend, but we can also sort here for API consistency)
	var red, yellow, green []models.ServerConfigCheck
	for _, c := range checks {
		switch c.Status {
		case "RED":
			red = append(red, c)
		case "YELLOW":
			yellow = append(yellow, c)
		default:
			green = append(green, c)
		}
	}
	checks = append(red, append(yellow, green...)...)

	return checks
}

// formatPgSizeHuman prints pg_settings size values consistently (MB/GB), while preserving the raw value+unit.
func (c *PgRepository) formatPgSizeHuman(value, unit string) string {
	raw := c.formatPgValue(value, unit)
	bytes := c.parsePgSize(value, unit)
	if bytes <= 0 {
		return raw
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	if bytes >= gb {
		return fmt.Sprintf("%.1f GB (%s)", float64(bytes)/float64(gb), raw)
	}
	return fmt.Sprintf("%.0f MB (%s)", float64(bytes)/float64(mb), raw)
}

// formatPgValue formats a pg_settings value with its unit for display.
func (c *PgRepository) formatPgValue(value, unit string) string {
	if unit == "" {
		return value
	}
	return value + " " + unit
}

// parsePgSize converts a pg_settings size value to bytes.
func (c *PgRepository) parsePgSize(value, unit string) int64 {
	num, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}

	switch strings.ToLower(unit) {
	case "kb":
		return int64(num * 1024)
	case "mb":
		return int64(num * 1024 * 1024)
	case "gb":
		return int64(num * 1024 * 1024 * 1024)
	case "tb":
		return int64(num * 1024 * 1024 * 1024 * 1024)
	default:
		// For unitless values (like shared_buffers in 8kB pages)
		if strings.ToLower(unit) == "8kb" {
			return int64(num * 8 * 1024)
		}
		return int64(num)
	}
}

// parsePgSizeKB converts a pg_settings size value to kilobytes.
func (c *PgRepository) parsePgSizeKB(value, unit string) int64 {
	num, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}

	switch strings.ToLower(unit) {
	case "kb":
		return int64(num)
	case "mb":
		return int64(num * 1024)
	case "gb":
		return int64(num * 1024 * 1024)
	default:
		// For unitless values (like work_mem in kB)
		if unit == "" {
			return int64(num) // work_mem is stored in kB internally
		}
		return int64(num)
	}
}
