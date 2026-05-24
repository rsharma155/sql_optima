// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background worker for "Performance Debt" analysis (Unused indexes, missing indexes, fragmentation).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"log/slog"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

func (s *MetricsService) StartPerformanceDebtCollector(ctx context.Context) {
	// Performance debt is heavy; run every 6 hours by default
	interval := s.GetCollectorInterval(ctx, "Performance Debt Collection", 6*time.Hour)
	slog.Info("[PerformanceDebt] Starting background collector (interval: %v)", "val", interval)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		// Run once at start
		if interval > 0 {
			s.collectPerformanceDebtOnce()
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Dynamic interval check
				newInterval := s.GetCollectorInterval(ctx, "Performance Debt Collection", 6*time.Hour)
				if newInterval != interval && newInterval > 0 {
					slog.Info("[PerformanceDebt] Frequency changed from", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}

				if interval > 0 {
					s.collectPerformanceDebtOnce()
				}
			}
		}
	}()
}

func (s *MetricsService) TriggerPerformanceDebtCollector() {
	slog.Info("[PerformanceDebt] Manual trigger received")
	go s.collectPerformanceDebtOnce()
}

func (s *MetricsService) collectPerformanceDebtOnce() {
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}

		// Skip if instance is not online in the repository
		if s.MsRepo.GetInstanceStatus(inst.Name) != "online" {
			continue
		}

		// Discover databases for this instance
		db, ok := s.MsRepo.GetConn(inst.Name)
		if !ok {
			continue
		}

		var databases []string
		rows, err := db.Query("SELECT name FROM sys.databases WHERE state = 0 AND name NOT IN ('master','tempdb','model','msdb')")
		if err == nil {
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err == nil {
					databases = append(databases, name)
				}
			}
			rows.Close()
		}

		go s.collectPerformanceDebtForInstance(inst.ServerID, inst.Name, databases)
	}
}

func (s *MetricsService) collectPerformanceDebtForInstance(serverID uuid.UUID, instanceName string, databases []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	slog.Info("[PerformanceDebt] Starting analysis", "target", instanceName, "server_id", serverID)

	// 1. Unused Indexes
	for _, dbName := range databases {
		unused, err := s.MsRepo.FetchUnusedIndexes(ctx, serverID, instanceName, dbName, 1000, 50)
		if err != nil {
			slog.Error("[PerformanceDebt] Unused indexes failed", "target", instanceName, "db", dbName, "err", err)
			continue
		}

		var findings []hot.PerformanceDebtFindingRow
		for _, idx := range unused {
			title := fmt.Sprintf("Unused Index (Monitoring): %s", idx.IndexName)
			rec := fmt.Sprintf(
				"Index %s on %s has %d write operations but zero reads since the last SQL Server restart (%s). "+
					"Requires ≥28 days of continuous observation before acting. Do not drop based on a single scan.",
				idx.IndexName, idx.TableName, idx.UserUpdates, idx.ServerStartTime.Format("2006-01-02"),
			)
			details, _ := json.Marshal(map[string]interface{}{
				"index_name":        idx.IndexName,
				"table_name":        idx.TableName,
				"user_updates":      idx.UserUpdates,
				"server_start_time": idx.ServerStartTime,
				"index_create_date": idx.IndexCreateDate,
				"recommendation":    rec,
			})
			fixScript := fmt.Sprintf(
				"-- WARNING: Verify ≥28 days of continuous observation before dropping.\n"+
					"-- sys.dm_db_index_usage_stats resets to zero on every SQL Server restart.\n"+
					"-- Confirm no batch, monthly, quarterly, or seasonal queries depend on this index.\nDROP INDEX [%s] ON [%s];",
				idx.IndexName, idx.TableName,
			)
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   dbName,
				Section:        "Index Health",
				FindingType:    "unused_index",
				Severity:       "INFO",
				Title:          title,
				ObjectName:     idx.TableName + "." + idx.IndexName,
				ObjectType:     "index",
				FindingKey:     fmt.Sprintf("%s.%s.%s", dbName, idx.TableName, idx.IndexName),
				ImpactScore:    float64(idx.UserUpdates),
				Details:        string(details),
				Recommendation: rec,
				FixScript:      fixScript,
			})
		}
		if len(findings) > 0 {
			_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, findings)
		}
	}

	// 2. Missing Indexes
	for _, dbName := range databases {
		missing, err := s.MsRepo.FetchMissingIndexRecommendations(ctx, serverID, instanceName, dbName, 25)
		if err != nil {
			continue
		}
		var findings []hot.PerformanceDebtFindingRow
		for _, m := range missing {
			userImpact := m.AvgUserImpact
			if userImpact > 100 {
				userImpact = 100
			}
			title := fmt.Sprintf("Missing Index on %s", m.TableName)
			rec := fmt.Sprintf("Missing index on [%s]. Estimated query cost improvement: %.1f%%", m.EqualityColumns, userImpact)

			details, _ := json.Marshal(map[string]interface{}{
				"table_name":        m.TableName,
				"equality_columns":  m.EqualityColumns,
				"inequality_cols":   m.InequalityColumns,
				"included_columns":  m.IncludedColumns,
				"avg_user_impact":   userImpact,
				"ranking_score":     m.ImprovementScore,
				"recommendation":    rec,
			})

			severity := "WARNING"
			if m.ImprovementScore > 500000 {
				severity = "CRITICAL"
			}

			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   dbName,
				Section:        "Index Health",
				FindingType:    "missing_index",
				Severity:       severity,
				Title:          title,
				ObjectName:     m.TableName,
				ObjectType:     "table",
				FindingKey:     fmt.Sprintf("%s.%s:missing", dbName, m.TableName),
				ImpactScore:    m.ImprovementScore,
				Details:        string(details),
				Recommendation: rec,
				FixScript:      m.CreateStatement,
			})
		}
		if len(findings) > 0 {
			_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, findings)
		}
	}

	// 2.5 Index Fragmentation
	for _, dbName := range databases {
		frags, err := s.MsRepo.FetchIndexFragmentation(ctx, serverID, instanceName, dbName, 5, 100, 50)
		if err != nil {
			continue
		}
		var findings []hot.PerformanceDebtFindingRow
		for _, fr := range frags {
			isHeap := fr.IndexID == 0
			severity := "WARNING"
			action := "REORGANIZE"
			if fr.FragPct > 30 {
				severity = "CRITICAL"
				action = "REBUILD"
			}
			var title, rec, fixScript string
			if isHeap {
				title = fmt.Sprintf("Fragmented Heap: %s.%s (%.1f%%)", fr.SchemaName, fr.TableName, fr.FragPct)
				rec = fmt.Sprintf("Table %s.%s has no clustered index (heap) with %.1f%% forwarded-record fragmentation and %d pages. Consider adding a clustered index or rebuilding the heap.", fr.SchemaName, fr.TableName, fr.FragPct, fr.PageCount)
				fixScript = fmt.Sprintf("-- Heaps require ALTER TABLE REBUILD, not ALTER INDEX REBUILD\nALTER TABLE [%s].[%s] REBUILD;", fr.SchemaName, fr.TableName)
			} else if fr.FragPct > 30 {
				title = fmt.Sprintf("Fragmented Index: %s on %s.%s (%.1f%%)", fr.IndexName, fr.SchemaName, fr.TableName, fr.FragPct)
				rec = fmt.Sprintf("Index %s on %s.%s has %.1f%% fragmentation (%d pages). REBUILD required. Note: REBUILD takes an exclusive lock on Standard Edition.", fr.IndexName, fr.SchemaName, fr.TableName, fr.FragPct, fr.PageCount)
				fixScript = fmt.Sprintf("ALTER INDEX [%s] ON [%s].[%s] REBUILD WITH (ONLINE = ON);", fr.IndexName, fr.SchemaName, fr.TableName)
			} else {
				title = fmt.Sprintf("Fragmented Index: %s on %s.%s (%.1f%%)", fr.IndexName, fr.SchemaName, fr.TableName, fr.FragPct)
				rec = fmt.Sprintf("Index %s on %s.%s has %.1f%% fragmentation (%d pages). REORGANIZE is appropriate at this level.", fr.IndexName, fr.SchemaName, fr.TableName, fr.FragPct, fr.PageCount)
				fixScript = fmt.Sprintf("ALTER INDEX [%s] ON [%s].[%s] REORGANIZE;", fr.IndexName, fr.SchemaName, fr.TableName)
			}
			details, _ := json.Marshal(map[string]interface{}{
				"schema_name":    fr.SchemaName,
				"table_name":     fr.TableName,
				"index_name":     fr.IndexName,
				"index_id":       fr.IndexID,
				"frag_pct":       fr.FragPct,
				"page_count":     fr.PageCount,
				"action":         action,
				"is_heap":        isHeap,
				"recommendation": rec,
			})
			findingType := "index_fragmentation"
			if isHeap {
				findingType = "heap_fragmentation"
			}
			impactScore := fr.FragPct * float64(fr.PageCount) / 100.0
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   dbName,
				Section:        "Index Health",
				FindingType:    findingType,
				Severity:       severity,
				Title:          title,
				ObjectName:     fmt.Sprintf("%s.%s.%s", fr.SchemaName, fr.TableName, fr.IndexName),
				ObjectType:     "index",
				FindingKey:     fmt.Sprintf("%s.%s.%s:frag", dbName, fr.TableName, fr.IndexName),
				ImpactScore:    impactScore,
				Details:        string(details),
				Recommendation: rec,
				FixScript:      fixScript,
			})
		}
		if len(findings) > 0 {
			_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, findings)
		}
	}

	// 3. Statistics Health
	for _, dbName := range databases {
		stale, err := s.MsRepo.FetchStaleStatistics(ctx, instanceName, dbName, 50)
		if err != nil {
			continue
		}
		var findings []hot.PerformanceDebtFindingRow
		for _, st := range stale {
			lastUpdated := "never"
			if st.LastUpdated.Valid {
				lastUpdated = st.LastUpdated.Time.Format("2006-01-02")
			}
			var title, rec, severity, findingType string
			switch {
			case st.NoRecompute:
				findingType = "stats_no_recompute"
				severity = "WARNING"
				title = fmt.Sprintf("Statistics NO_RECOMPUTE: %s on %s", st.StatsName, st.TableName)
				rec = fmt.Sprintf("Statistics %s on %s has NO_RECOMPUTE enabled. Auto-update will never fire for this object regardless of modification count.", st.StatsName, st.TableName)
			case st.Rows > 0 && st.ModificationCounter > st.Rows:
				findingType = "stale_statistics"
				severity = "CRITICAL"
				title = fmt.Sprintf("Critically Stale Statistics: %s on %s", st.StatsName, st.TableName)
				rec = fmt.Sprintf("Statistics %s on %s has %d modifications exceeding its %d row count (last updated: %s).", st.StatsName, st.TableName, st.ModificationCounter, st.Rows, lastUpdated)
			default:
				findingType = "stale_statistics"
				severity = "WARNING"
				title = fmt.Sprintf("Stale Statistics: %s on %s", st.StatsName, st.TableName)
				rec = fmt.Sprintf("Statistics %s on %s has %d modifications since %s.", st.StatsName, st.TableName, st.ModificationCounter, lastUpdated)
			}
			details, _ := json.Marshal(map[string]interface{}{
				"table_name":           st.TableName,
				"stats_name":           st.StatsName,
				"last_updated":         lastUpdated,
				"rows":                 st.Rows,
				"modification_counter": st.ModificationCounter,
				"recommendation":       rec,
			})
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   dbName,
				Section:        "Statistics Health",
				FindingType:    findingType,
				Severity:       severity,
				Title:          title,
				ObjectName:     st.TableName + "." + st.StatsName,
				ObjectType:     "statistics",
				FindingKey:     fmt.Sprintf("%s.%s.%s", dbName, st.TableName, st.StatsName),
				ImpactScore:    float64(st.ModificationCounter),
				Details:        string(details),
				Recommendation: rec,
				FixScript:      fmt.Sprintf("UPDATE STATISTICS [%s] ([%s]) WITH FULLSCAN;", st.TableName, st.StatsName),
			})
		}
		if len(findings) > 0 {
			_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, findings)
		}
	}

	// 4. Storage & Growth
	for _, dbName := range databases {
		files, err := s.MsRepo.FetchAutogrowthRisks(ctx, instanceName, dbName, 30)
		if err != nil {
			continue
		}
		vlfStats, _ := s.MsRepo.FetchVLFCount(ctx, instanceName, dbName)
		vlfCount := vlfStats.VLFCount

		var findings []hot.PerformanceDebtFindingRow
		for _, f := range files {
			var severity, title, rec, fixScript string

			switch {
			case f.Growth == 0:
				severity = "CRITICAL"
				title = fmt.Sprintf("Autogrowth Disabled: %s", f.FileName)
				rec = fmt.Sprintf("File %s in %s has autogrowth disabled.", f.FileName, dbName)
				fixScript = fmt.Sprintf("ALTER DATABASE [%s] MODIFY FILE (NAME = N'%s', FILEGROWTH = 512MB);", dbName, f.FileName)
			case f.IsPercentGrowth && f.FileType == 0 && f.SizeMB > 100:
				severity = "CRITICAL"
				title = fmt.Sprintf("Percent Autogrowth on Data File: %s (%d%%)", f.FileName, f.Growth)
				rec = fmt.Sprintf("Data file %s uses percent-based autogrowth. Switch to a fixed increment.", f.FileName)
				fixScript = fmt.Sprintf("ALTER DATABASE [%s] MODIFY FILE (NAME = N'%s', FILEGROWTH = 512MB);", dbName, f.FileName)
			case f.IsPercentGrowth && f.FileType == 1:
				severity = "CRITICAL"
				title = fmt.Sprintf("Percent Autogrowth on Log File: %s (%d%%)", f.FileName, f.Growth)
				rec = "Log file uses percent-based autogrowth. Switch to fixed (e.g., 256MB)."
				fixScript = fmt.Sprintf("ALTER DATABASE [%s] MODIFY FILE (NAME = N'%s', FILEGROWTH = 256MB);", dbName, f.FileName)
			case !f.IsPercentGrowth && f.Growth == 128:
				severity = "WARNING"
				title = fmt.Sprintf("Legacy 1 MB Autogrowth: %s", f.FileName)
				rec = "File uses legacy 1 MB autogrowth. Increase to at least 512MB for data."
				targetMB := "512MB"
				if f.FileType == 1 { targetMB = "64MB" }
				fixScript = fmt.Sprintf("ALTER DATABASE [%s] MODIFY FILE (NAME = N'%s', FILEGROWTH = %s);", dbName, f.FileName, targetMB)
			}

			if severity != "" {
				findings = append(findings, hot.PerformanceDebtFindingRow{
					DatabaseName: dbName, Section: "Storage & Growth", FindingType: "autogrowth_risk",
					Severity: severity, Title: title, ObjectName: f.FileName, ObjectType: "database_file",
					FindingKey: fmt.Sprintf("%s.file.%s", dbName, f.FileName), ImpactScore: f.SizeMB,
					Recommendation: rec, FixScript: fixScript,
				})
			}
		}
		if vlfCount >= 50 {
			severity := "WARNING"
			if vlfCount > 500 { severity = "CRITICAL" }
			rec := fmt.Sprintf("Database %s has %d VLFs.", dbName, vlfCount)
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName: dbName, Section: "Storage & Growth", FindingType: "excessive_vlfs",
				Severity: severity, Title: fmt.Sprintf("Excessive VLFs: %d in %s", vlfCount, dbName),
				ObjectName: dbName + ".log", ObjectType: "log_file", FindingKey: fmt.Sprintf("%s.vlf_count", dbName),
				ImpactScore: float64(vlfCount), Recommendation: rec,
			})
		}
		if len(findings) > 0 { _ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, findings) }
	}

	// 5. Backup & Recovery
	for _, dbName := range databases {
		ageHours, err := s.MsRepo.FetchLastFullBackupAgeHours(ctx, instanceName, dbName)
		if err == nil {
			severity := ""
			if ageHours >= 999999 || ageHours > 168 { severity = "CRITICAL" } else if ageHours > 24 { severity = "WARNING" }
			if severity != "" {
				ageDesc := fmt.Sprintf("%.1f hours", ageHours)
				if ageHours >= 999999 { ageDesc = "never backed up" }
				_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, []hot.PerformanceDebtFindingRow{{
					DatabaseName: dbName, Section: "Backup & Recovery", FindingType: "backup_age",
					Severity: severity, Title: fmt.Sprintf("Overdue Full Backup: %s (%s)", dbName, ageDesc),
					ObjectName: dbName, ObjectType: "database", FindingKey: fmt.Sprintf("%s.full_backup_age", dbName),
					ImpactScore: ageHours, Recommendation: fmt.Sprintf("Database %s last full backup was %s ago.", dbName, ageDesc),
				}})
			}
		}
	}

	// 6. SQL Agent
	{
		var agentFindings []hot.PerformanceDebtFindingRow
		failed, err := s.MsRepo.FetchFailedAgentJobs24h(ctx, instanceName, 20)
		if err == nil {
			for _, job := range failed {
				name, _ := job["job_name"].(string)
				runDt, _ := job["run_dt"].(time.Time)
				agentFindings = append(agentFindings, hot.PerformanceDebtFindingRow{
					DatabaseName: "msdb", Section: "SQL Agent", FindingType: "failed_job",
					Severity: "WARNING", Title: fmt.Sprintf("Failed Agent Job: %s", name),
					ObjectName: name, ObjectType: "agent_job", FindingKey: fmt.Sprintf("agent.failed.%s", name),
					ImpactScore: 1, Recommendation: fmt.Sprintf("SQL Agent job '%s' failed at %s.", name, runDt.Format(time.RFC3339)),
				})
			}
		}
		if len(agentFindings) > 0 { _ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, agentFindings) }
	}

	// 7. Engine Config (instance-level)
	{
		checks := []struct { name string; severity, title, rec string } {
			{"optimize for ad hoc workloads", "WARNING", "optimize for ad hoc workloads is OFF", "Enable to reduce plan cache bloat."},
			{"cost threshold for parallelism", "WARNING", "cost threshold for parallelism is too low", "Raise to 25-50."},
			{"max server memory (MB)", "WARNING", "max server memory is uncapped", "Set explicit ceiling."},
		}
		var configFindings []hot.PerformanceDebtFindingRow
		for _, chk := range checks {
			val, err := s.MsRepo.FetchConfigValueInUse(ctx, instanceName, chk.name)
			if err == nil {
				flagged := false
				if chk.name == "max server memory (MB)" && val >= 2147483647 { flagged = true }
				if chk.name == "optimize for ad hoc workloads" && val == 0 { flagged = true }
				if chk.name == "cost threshold for parallelism" && val <= 5 { flagged = true }
				
				if flagged {
					configFindings = append(configFindings, hot.PerformanceDebtFindingRow{
						DatabaseName: "master", Section: "Engine Config", FindingType: "config_risk",
						Severity: chk.severity, Title: "Engine Config: " + chk.title, ObjectName: chk.name,
						ObjectType: "server_config", FindingKey: "config." + chk.name, Recommendation: chk.rec,
					})
				}
			}
		}
		if len(configFindings) > 0 { _ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, configFindings) }
	}

	// Phase 8-12: Enhanced evaluations
	s.collectServerVitals(ctx, serverID, instanceName)
	s.collectQueryStoreHealth(ctx, serverID, instanceName, databases)
	s.evaluateDestructiveSettings(ctx, serverID, instanceName)
	s.evaluateTempDBDesign(ctx, serverID, instanceName)
	s.evaluateIOLatency(ctx, serverID)

	slog.Info("[PerformanceDebt] Analysis complete", "target", instanceName, "server_id", serverID)
}

func (s *MetricsService) evaluateTempDBDesign(ctx context.Context, serverID uuid.UUID, instanceName string) {
	db, ok := s.MsRepo.GetConn(instanceName)
	if !ok {
		return
	}

	var cpuCount int
	_ = db.QueryRowContext(ctx, "SELECT cpu_count FROM sys.dm_os_sys_info").Scan(&cpuCount)
	if cpuCount <= 0 {
		cpuCount = 8
	}

	targetFiles := cpuCount
	if targetFiles > 8 {
		targetFiles = 8
	}

	rows, err := s.MsRepo.CollectTempDBUsage(ctx, db)
	if err != nil {
		return
	}

	var dataFiles []map[string]interface{}
	for _, r := range rows {
		typeDesc := ""
		if td, ok := r["type_desc"].(string); ok {
			typeDesc = td
		}
		if typeDesc == "ROWS" {
			dataFiles = append(dataFiles, r)
		}
	}

	var findings []hot.PerformanceDebtFindingRow
	if len(dataFiles) < targetFiles {
		rec := fmt.Sprintf("TempDB has %d data files but the server has %d CPUs. Recommendation is to have 1 file per CPU up to 8 files to reduce PAGELATCH_UP contention in allocation bitmaps.", len(dataFiles), cpuCount)
		det, _ := json.Marshal(map[string]interface{}{"current_files": len(dataFiles), "target_files": targetFiles, "cpus": cpuCount})
		findings = append(findings, hot.PerformanceDebtFindingRow{
			DatabaseName:   "tempdb",
			Section:        "Storage & Growth",
			FindingType:    "tempdb_too_few_files",
			Severity:       "WARNING",
			Title:          "Too Few TempDB Data Files",
			Recommendation: rec,
			FindingKey:     "tempdb_file_count",
			ImpactScore:    float64(targetFiles - len(dataFiles)),
			Details:        string(det),
			ObjectName:     "tempdb",
			ObjectType:     "database",
		})
	}

	if len(dataFiles) > 1 {
		var firstSize float64
		if sz, ok := dataFiles[0]["size_mb"].(int64); ok {
			firstSize = float64(sz)
		} else if sz, ok := dataFiles[0]["size_mb"].(float64); ok {
			firstSize = sz
		}
		
		unequal := false
		for i := 1; i < len(dataFiles); i++ {
			var curSize float64
			if sz, ok := dataFiles[i]["size_mb"].(int64); ok {
				curSize = float64(sz)
			} else if sz, ok := dataFiles[i]["size_mb"].(float64); ok {
				curSize = sz
			}
			if curSize != firstSize {
				unequal = true
				break
			}
		}
		if unequal {
			rec := "TempDB data files are not equally sized. SQL Server uses a proportional fill algorithm, meaning larger files will be hit more frequently, potentially causing I/O hotspots. Ensure all TempDB data files have the same initial size and growth increments."
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   "tempdb",
				Section:        "Storage & Growth",
				FindingType:    "tempdb_unequal_files",
				Severity:       "WARNING",
				Title:          "Unequal TempDB Data File Sizes",
				Recommendation: rec,
				FindingKey:     "tempdb_unequal_sizes",
				Details:        "Proportional fill algorithm detected issues due to file size mismatch.",
				ObjectName:     "tempdb",
				ObjectType:     "database",
			})
		}
	}

	if len(findings) > 0 {
		_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, findings)
	}
}

func (s *MetricsService) evaluateIOLatency(ctx context.Context, serverID uuid.UUID) {
	if s.tsHotStorage == nil {
		return
	}

	q := `
		WITH io_deltas AS (
			SELECT 
				db_name,
				file_id,
				capture_timestamp,
				io_stall_read_ms - LAG(io_stall_read_ms) OVER (PARTITION BY server_id, database_id, file_id ORDER BY capture_timestamp) AS read_ms_delta,
				num_of_reads - LAG(num_of_reads) OVER (PARTITION BY server_id, database_id, file_id ORDER BY capture_timestamp) AS reads_delta,
				io_stall_write_ms - LAG(io_stall_write_ms) OVER (PARTITION BY server_id, database_id, file_id ORDER BY capture_timestamp) AS write_ms_delta,
				num_of_writes - LAG(num_of_writes) OVER (PARTITION BY server_id, database_id, file_id ORDER BY capture_timestamp) AS writes_delta
			FROM staging.sqlserver_io_raw
			WHERE server_id = $1
			  AND capture_timestamp >= NOW() - INTERVAL '2 hours'
			  AND database_id > 4
		)
		SELECT 
			db_name,
			COALESCE(SUM(read_ms_delta) / NULLIF(SUM(reads_delta), 0), 0) AS avg_read_latency_ms,
			COALESCE(SUM(write_ms_delta) / NULLIF(SUM(writes_delta), 0), 0) AS avg_write_latency_ms
		FROM io_deltas
		WHERE reads_delta > 0 OR writes_delta > 0
		GROUP BY db_name
		HAVING COALESCE(SUM(read_ms_delta) / NULLIF(SUM(reads_delta), 0), 0) > 20
		    OR COALESCE(SUM(write_ms_delta) / NULLIF(SUM(writes_delta), 0), 0) > 20
	`

	rows, err := s.tsHotStorage.Pool().Query(ctx, q, serverID)
	if err != nil {
		return
	}
	defer rows.Close()

	var findings []hot.PerformanceDebtFindingRow
	for rows.Next() {
		var dbName string
		var readLat, writeLat float64
		if err := rows.Scan(&dbName, &readLat, &writeLat); err != nil {
			continue
		}

		if readLat > 20 || writeLat > 20 {
			severity := "WARNING"
			if readLat > 100 || writeLat > 100 {
				severity = "CRITICAL"
			}
			impact := readLat
			if writeLat > impact {
				impact = writeLat
			}

			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   dbName,
				Section:        "Storage & Growth",
				FindingType:    "high_io_latency",
				Severity:       severity,
				Title:          "High I/O Latency Detected",
				Recommendation: fmt.Sprintf("Average I/O latency for database [%s] is high: Read %.1fms, Write %.1fms. Check storage performance or I/O intensive processes.", dbName, readLat, writeLat),
				FindingKey:     "high_io_" + dbName,
				ImpactScore:    impact,
			})
		}
	}
	if len(findings) > 0 {
		_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, findings)
	}
}

func (s *MetricsService) collectServerVitals(ctx context.Context, serverID uuid.UUID, instanceName string) {
	// 1. Memory Snapshot
	mem, err := s.MsRepo.FetchMemoryAnalyzerSnapshot(ctx, serverID, instanceName)
	if err == nil {
		_ = s.tsLogger.LogSQLServerMemoryMetrics(ctx, serverID, mem)

		var findings []hot.PerformanceDebtFindingRow
		// Rule: Utilization > 98%
		if util, ok := mem["sql_memory_utilization_pct"].(int64); ok && util > 98 {
			findings = append(findings, hot.PerformanceDebtFindingRow{
				Section:        "Engine Config",
				FindingType:    "memory_pressure",
				Severity:       "WARNING",
				Title:          "High SQL Memory Utilization",
				Recommendation: "SQL Server is utilizing >98% of its allocated physical memory. Check for memory-intensive queries or external OS pressure.",
				FindingKey:     "memory_util_high",
			})
		}
		// Rule: Physical Memory Low
		if low, ok := mem["process_physical_low"].(bool); ok && low {
			findings = append(findings, hot.PerformanceDebtFindingRow{
				Section:        "Engine Config",
				FindingType:    "memory_physical_pressure",
				Severity:       "CRITICAL",
				Title:          "OS Physical Memory Low",
				Recommendation: "The operating system is reporting a low physical memory condition. This can lead to heavy paging and SQL Server performance degradation.",
				FindingKey:     "os_phys_low",
			})
		}
		// Rule: Virtual Memory Low
		if vlow, ok := mem["process_virtual_low"].(bool); ok && vlow {
			findings = append(findings, hot.PerformanceDebtFindingRow{
				Section:        "Engine Config",
				FindingType:    "memory_virtual_pressure",
				Severity:       "CRITICAL",
				Title:          "OS Virtual Memory Low",
				Recommendation: "The operating system is reporting low virtual memory (page file exhaustion). This is a critical condition that can cause SQL Server to crash or fail to spawn new threads.",
				FindingKey:     "os_virt_low",
			})
		}

		// Rule: Memory Headroom (External Pressure)
		// OS utilization > 90% AND SQL consuming >85% of total — server is sized too tight.
		osUtil, ok1 := mem["os_utilization_pct"].(int64)
		sqlUsed, ok2 := mem["sql_memory_used_mb"].(int64)
		osTotal, ok3 := mem["os_total_memory_mb"].(int64)
		if ok1 && ok2 && ok3 && osTotal > 0 {
			sqlPctOfOS := float64(sqlUsed) / float64(osTotal)
			if osUtil > 90 && sqlPctOfOS > 0.85 {
				findings = append(findings, hot.PerformanceDebtFindingRow{
					Section:        "Engine Config",
					FindingType:    "memory_headroom_low",
					Severity:       "WARNING",
					Title:          "Low Memory Headroom",
					Recommendation: fmt.Sprintf("OS memory utilization is %d%% and SQL Server is consuming %.1f%% of total physical memory. There is very little headroom for OS tasks or spikes in non-buffer pool memory.", osUtil, sqlPctOfOS*100),
					FindingKey:     "memory_headroom_low",
				})
			}
		}

		if len(findings) > 0 {
			_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, findings)
		}
	}

	// 2. Volume Stats
	vols, err := s.MsRepo.FetchVolumeStats(ctx, instanceName)
	if err == nil {
		_ = s.tsLogger.LogVolumeStats(ctx, serverID, vols)

		var findings []hot.PerformanceDebtFindingRow
		for _, v := range vols {
			// Thresholds: <15% → WARNING, <10% → CRITICAL (per §3 Storage Alert Rules)
			if v.VolumeFreePct < 15 {
				findingType := "volume_low"
				severity := "WARNING"
				if v.VolumeFreePct < 10 {
					findingType = "volume_critically_low"
					severity = "CRITICAL"
				}
				findings = append(findings, hot.PerformanceDebtFindingRow{
					Section:        "Storage & Growth",
					FindingType:    findingType,
					Severity:       severity,
					Title:          fmt.Sprintf("Low Disk Space: %s", v.VolumeMountPoint),
					Recommendation: fmt.Sprintf("Volume %s has only %.1f%% free space (%.1f GB available). No automated fix. Expand volume or relocate files: ALTER DATABASE [{db}] MODIFY FILE (NAME = N'{file}', FILENAME = N'{new_path}');", v.VolumeMountPoint, v.VolumeFreePct, v.VolumeAvailableGB),
					FindingKey:     "low_disk_" + v.VolumeMountPoint,
					ImpactScore:    100 - v.VolumeFreePct,
				})
			}
		}
		if len(findings) > 0 {
			_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, findings)
		}
	}
}

func (s *MetricsService) collectQueryStoreHealth(ctx context.Context, serverID uuid.UUID, instanceName string, databases []string) {
	var findings []hot.PerformanceDebtFindingRow
	for _, dbName := range databases {
		opts, err := s.MsRepo.FetchQueryStoreOptions(ctx, instanceName, dbName)
		if err != nil {
			continue
		}

		if opts.ActualStateDesc == "OFF" {
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   dbName,
				Section:        "Engine Config",
				FindingType:    "query_store_off",
				Severity:       "WARNING",
				Title:          "Query Store Disabled",
				Recommendation: fmt.Sprintf("Query Store is disabled for database [%s]. Query Store is essential for plan regression tracking and performance tuning.", dbName),
				FindingKey:     "qs_off_" + dbName,
				FixScript:      fmt.Sprintf("ALTER DATABASE [%s] SET QUERY_STORE = ON;", dbName),
				ObjectName:     dbName,
				ObjectType:     "database",
			})
		} else if opts.ActualStateDesc == "READ_ONLY" {
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   dbName,
				Section:        "Engine Config",
				FindingType:    "query_store_read_only",
				Severity:       "CRITICAL",
				Title:          "Query Store READ_ONLY",
				Recommendation: fmt.Sprintf("Query Store for database [%s] has transitioned to READ_ONLY state (Reason: %d). This usually indicates it has reached its storage limit or is under heavy load.", dbName, opts.ReadonlyReason),
				FindingKey:     "qs_ro_" + dbName,
				FixScript:      fmt.Sprintf("ALTER DATABASE [%s] SET QUERY_STORE (MAX_STORAGE_SIZE_MB = %.0f);", dbName, opts.MaxStorageSizeMB*2),
				ObjectName:     dbName,
				ObjectType:     "database",
			})
		}

		if opts.StorageUsedPct > 90 {
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   dbName,
				Section:        "Engine Config",
				FindingType:    "query_store_almost_full",
				Severity:       "WARNING",
				Title:          "Query Store Almost Full",
				Recommendation: fmt.Sprintf("Query Store for database [%s] is %.1f%% full (%.0f / %.0f MB). Increase MAX_STORAGE_SIZE_MB to avoid it switching to READ_ONLY mode.", dbName, opts.StorageUsedPct, opts.CurrentStorageSizeMB, opts.MaxStorageSizeMB),
				FindingKey:     "qs_full_" + dbName,
				ImpactScore:    opts.StorageUsedPct,
				ObjectName:     dbName,
				ObjectType:     "database",
			})
		}

		if opts.BrokenForcedPlans > 0 {
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   dbName,
				Section:        "Engine Config",
				FindingType:    "query_store_broken_forced_plan",
				Severity:       "WARNING",
				Title:          "Broken Forced Plans",
				Recommendation: fmt.Sprintf("Database [%s] has %d query plans that are forced but failed to apply. This can lead to unpredictable performance regressions.", dbName, opts.BrokenForcedPlans),
				FindingKey:     "qs_broken_plans_" + dbName,
				ImpactScore:    float64(opts.BrokenForcedPlans),
				ObjectName:     dbName,
				ObjectType:     "database",
			})
		}
	}
	if len(findings) > 0 {
		_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, findings)
	}
}

func (s *MetricsService) evaluateDestructiveSettings(ctx context.Context, serverID uuid.UUID, instanceName string) {
	catalog, err := s.MsRepo.FetchDatabaseCatalog(ctx, instanceName)
	if err != nil {
		return
	}

	// Get host compatibility level from master
	var hostCompat int
	db, ok := s.MsRepo.GetConn(instanceName)
	if ok {
		_ = db.QueryRowContext(ctx, "SELECT compatibility_level FROM sys.databases WHERE name = 'master'").Scan(&hostCompat)
	}

	var findings []hot.PerformanceDebtFindingRow
	for _, db := range catalog {
		if db.DatabaseID <= 4 {
			continue
		}
		if db.IsAutoShrinkOn {
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   db.DatabaseName,
				Section:        "Storage & Growth",
				FindingType:    "auto_shrink_on",
				Severity:       "CRITICAL",
				Title:          "Auto-Shrink Enabled",
				Recommendation: fmt.Sprintf("Database [%s] has AUTO_SHRINK enabled. This causes massive fragmentation and high I/O overhead.", db.DatabaseName),
				FindingKey:     "auto_shrink_" + db.DatabaseName,
				FixScript:      fmt.Sprintf("ALTER DATABASE [%s] SET AUTO_SHRINK OFF;", db.DatabaseName),
				ObjectName:     db.DatabaseName,
				ObjectType:     "database",
			})
		}
		if db.IsAutoCloseOn {
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   db.DatabaseName,
				Section:        "Engine Config",
				FindingType:    "auto_close_on",
				Severity:       "WARNING",
				Title:          "Auto-Close Enabled",
				Recommendation: fmt.Sprintf("Database [%s] has AUTO_CLOSE enabled. This causes the database to shutdown when the last user exits, leading to slow reconnection times and cache purging.", db.DatabaseName),
				FindingKey:     "auto_close_" + db.DatabaseName,
				FixScript:      fmt.Sprintf("ALTER DATABASE [%s] SET AUTO_CLOSE OFF;", db.DatabaseName),
				ObjectName:     db.DatabaseName,
				ObjectType:     "database",
			})
		}

		if hostCompat > 0 && db.CompatibilityLevel <= hostCompat-20 { // 2 or more major versions behind
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   db.DatabaseName,
				Section:        "Engine Config",
				FindingType:    "compat_level_stale",
				Severity:       "WARNING",
				Title:          fmt.Sprintf("Legacy Compatibility Level (%d)", db.CompatibilityLevel),
				Recommendation: fmt.Sprintf("Database [%s] is running in compatibility level %d, which is at least 2 versions behind the server version (%d). You are missing out on modern optimizer features and performance fixes.", db.DatabaseName, db.CompatibilityLevel, hostCompat),
				FindingKey:     "compat_stale_" + db.DatabaseName,
				FixScript:      fmt.Sprintf("ALTER DATABASE [%s] SET COMPATIBILITY_LEVEL = %d;", db.DatabaseName, hostCompat),
				ObjectName:     db.DatabaseName,
				ObjectType:     "database",
			})
		}
	}
	if len(findings) > 0 {
		_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, findings)
	}
}
