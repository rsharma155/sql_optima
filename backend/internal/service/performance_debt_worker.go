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

func (s *MetricsService) collectPerformanceDebtOnce() {
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	slog.Info("[PerformanceDebt] Starting analysis", "arg1", instanceName, "arg2", serverID)

	// 1. Unused Indexes
	// SAFETY: sys.dm_db_index_usage_stats resets to zero on every SQL Server restart.
	// All unused-index findings are permanently marked INFO ("Monitoring") until 28+ days of
	// continuous observation have been collected. The DROP INDEX fix script is intentionally
	// gated with a safety warning comment and blocked in the UI for first-scan data.
	for _, dbName := range databases {
		unused, err := s.MsRepo.FetchUnusedIndexes(ctx, instanceName, dbName, 1000, 50)
		if err != nil {
			slog.Error(fmt.Sprintf("[PerformanceDebt] ERROR FetchUnusedIndexes for %s/%s: %v", instanceName, dbName, err))
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
		missing, err := s.MsRepo.FetchMissingIndexRecommendations(ctx, instanceName, dbName, 25)
		if err != nil {
			continue
		}
		var findings []hot.PerformanceDebtFindingRow
		for _, m := range missing {
			// avg_user_impact is the actual estimated improvement percentage (0–100) from SQL Server DMVs.
			// ImprovementScore is a composite ranking value and must not be displayed as a percentage.
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
		frags, err := s.MsRepo.FetchIndexFragmentation(ctx, instanceName, dbName, 10, 100, 50)
		if err != nil {
			slog.Error(fmt.Sprintf("[PerformanceDebt] ERROR FetchIndexFragmentation for %s/%s: %v", instanceName, dbName, err))
			continue
		}
		var findings []hot.PerformanceDebtFindingRow
		for _, fr := range frags {
			isHeap := fr.IndexID == 0
			// fragmentation > 30% → REBUILD (exclusive lock on Standard Edition); 10-30% → REORGANIZE (online)
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
				fixScript = fmt.Sprintf("-- Heaps require ALTER TABLE REBUILD, not ALTER INDEX REBUILD\n-- WITH (ONLINE = ON) requires Enterprise Edition; remove for Standard Edition\nALTER TABLE [%s].[%s] REBUILD;", fr.SchemaName, fr.TableName)
			} else if fr.FragPct > 30 {
				title = fmt.Sprintf("Fragmented Index: %s on %s.%s (%.1f%%)", fr.IndexName, fr.SchemaName, fr.TableName, fr.FragPct)
				rec = fmt.Sprintf("Index %s on %s.%s has %.1f%% fragmentation (%d pages). REBUILD required — REORGANIZE is not sufficient at this level. Note: REBUILD takes an exclusive lock on Standard Edition.", fr.IndexName, fr.SchemaName, fr.TableName, fr.FragPct, fr.PageCount)
				fixScript = fmt.Sprintf("-- WITH (ONLINE = ON) requires Enterprise Edition; remove for Standard Edition\nALTER INDEX [%s] ON [%s].[%s] REBUILD WITH (ONLINE = ON);", fr.IndexName, fr.SchemaName, fr.TableName)
			} else {
				title = fmt.Sprintf("Fragmented Index: %s on %s.%s (%.1f%%)", fr.IndexName, fr.SchemaName, fr.TableName, fr.FragPct)
				rec = fmt.Sprintf("Index %s on %s.%s has %.1f%% fragmentation (%d pages). REORGANIZE is appropriate at this level (online, minimal locking).", fr.IndexName, fr.SchemaName, fr.TableName, fr.FragPct, fr.PageCount)
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
			// Weight impact by both fragmentation level and physical size.
			impactScore := fr.FragPct * float64(fr.PageCount) / 100.0
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   dbName,
				Section:        "Index Health",
				FindingType:    findingType,
				Severity:       severity,
				Title:          title,
				ObjectName:     fmt.Sprintf("%s.%s.%s", fr.SchemaName, fr.TableName, fr.IndexName),
				ObjectType:     "index",
				FindingKey:     fmt.Sprintf("%s.%s.%s.%s:frag", dbName, fr.SchemaName, fr.TableName, fr.IndexName),
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
			slog.Error(fmt.Sprintf("[PerformanceDebt] ERROR FetchStaleStatistics for %s/%s: %v", instanceName, dbName, err))
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
				rec = fmt.Sprintf("Statistics %s on %s has NO_RECOMPUTE enabled. Auto-update will never fire for this object regardless of modification count. Verify this is intentional and that it is updated via a scheduled maintenance job.", st.StatsName, st.TableName)
			case st.Rows > 0 && st.ModificationCounter > st.Rows:
				findingType = "stale_statistics"
				severity = "CRITICAL"
				title = fmt.Sprintf("Critically Stale Statistics: %s on %s", st.StatsName, st.TableName)
				rec = fmt.Sprintf("Statistics %s on %s has %d modifications exceeding its %d row count (last updated: %s). The optimizer is working with severely outdated distribution data.", st.StatsName, st.TableName, st.ModificationCounter, st.Rows, lastUpdated)
			case !st.LastUpdated.Valid || st.LastUpdated.Time.Before(time.Now().AddDate(0, 0, -30)):
				findingType = "stale_statistics"
				severity = "WARNING"
				title = fmt.Sprintf("Stale Statistics (>30d): %s on %s", st.StatsName, st.TableName)
				rec = fmt.Sprintf("Statistics %s on %s was last updated %s. Small lookup tables with infrequent inserts can accumulate staleness silently.", st.StatsName, st.TableName, lastUpdated)
			default:
				findingType = "stale_statistics"
				severity = "WARNING"
				title = fmt.Sprintf("Stale Statistics: %s on %s", st.StatsName, st.TableName)
				rec = fmt.Sprintf("Statistics %s on %s has %d modifications exceeding the SQL Server 2016+ dynamic threshold for %d rows (last updated: %s).", st.StatsName, st.TableName, st.ModificationCounter, st.Rows, lastUpdated)
			}
			details, _ := json.Marshal(map[string]interface{}{
				"table_name":           st.TableName,
				"stats_name":           st.StatsName,
				"last_updated":         lastUpdated,
				"rows":                 st.Rows,
				"modification_counter": st.ModificationCounter,
				"no_recompute":         st.NoRecompute,
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
			slog.Error(fmt.Sprintf("[PerformanceDebt] ERROR FetchAutogrowthRisks for %s/%s: %v", instanceName, dbName, err))
			continue
		}
		vlfCount, _ := s.MsRepo.FetchVLFCount(ctx, instanceName, dbName)

		var findings []hot.PerformanceDebtFindingRow
		for _, f := range files {
			// growthMB: only meaningful when is_percent_growth = false; growth is in 8KB pages.
			growthMB := float64(f.Growth) * 8.0 / 1024.0

			var severity, title, rec, fixScript string

			switch {
			case f.Growth == 0:
				severity = "CRITICAL"
				title = fmt.Sprintf("Autogrowth Disabled: %s", f.FileName)
				rec = fmt.Sprintf("File %s in %s has autogrowth disabled. Without autogrowth the database will go offline when the file is full.", f.FileName, dbName)
				fixScript = fmt.Sprintf("ALTER DATABASE [%s] MODIFY FILE (NAME = N'%s', FILEGROWTH = 512MB);", dbName, f.FileName)

			case f.IsPercentGrowth && f.FileType == 0 && f.SizeMB > 100:
				severity = "CRITICAL"
				title = fmt.Sprintf("Percent Autogrowth on Data File: %s (%d%%)", f.FileName, f.Growth)
				rec = fmt.Sprintf("Data file %s (%.0f MB) uses %.0f%% percent-based autogrowth. Each event grows the file by %.0f MB, causing unpredictable I/O stalls. Switch to a fixed increment.", f.FileName, f.SizeMB, float64(f.Growth), f.SizeMB*float64(f.Growth)/100.0)
				fixScript = fmt.Sprintf("ALTER DATABASE [%s] MODIFY FILE (NAME = N'%s', FILEGROWTH = 512MB);", dbName, f.FileName)

			case f.IsPercentGrowth && f.FileType == 1:
				severity = "CRITICAL"
				title = fmt.Sprintf("Percent Autogrowth on Log File: %s (%d%%)", f.FileName, f.Growth)
				rec = fmt.Sprintf("Log file %s uses %d%% percent-based autogrowth. This creates excessive VLFs and unpredictable log-space stalls. Switch to a fixed increment.", f.FileName, f.Growth)
				fixScript = fmt.Sprintf("ALTER DATABASE [%s] MODIFY FILE (NAME = N'%s', FILEGROWTH = 256MB);", dbName, f.FileName)

			case !f.IsPercentGrowth && f.FileType == 0 && f.SizeMB > 1024 && growthMB < 64:
				severity = "WARNING"
				title = fmt.Sprintf("Small Fixed Autogrowth on Large Data File: %s (%.0f MB)", f.FileName, growthMB)
				rec = fmt.Sprintf("Data file %s is %.0f MB but grows only %.0f MB per event. On a file this large, frequent growth events cause write stalls. Increase to at least 512 MB.", f.FileName, f.SizeMB, growthMB)
				fixScript = fmt.Sprintf("ALTER DATABASE [%s] MODIFY FILE (NAME = N'%s', FILEGROWTH = 512MB);", dbName, f.FileName)

			case !f.IsPercentGrowth && f.FileType == 1 && f.SizeMB > 256 && growthMB < 8:
				severity = "WARNING"
				title = fmt.Sprintf("Small Fixed Autogrowth on Log File: %s (%.0f MB)", f.FileName, growthMB)
				rec = fmt.Sprintf("Log file %s is %.0f MB but grows only %.0f MB per event. Small log growth increments create excessive VLFs. Increase to at least 64 MB.", f.FileName, f.SizeMB, growthMB)
				fixScript = fmt.Sprintf("ALTER DATABASE [%s] MODIFY FILE (NAME = N'%s', FILEGROWTH = 64MB);", dbName, f.FileName)

			case !f.IsPercentGrowth && f.Growth == 128:
				// 128 pages × 8 KB = 1 MB — the legacy SQL Server 2014 and earlier default
				severity = "WARNING"
				title = fmt.Sprintf("Legacy 1 MB Autogrowth: %s", f.FileName)
				rec = fmt.Sprintf("File %s uses the legacy 1 MB autogrowth default from SQL Server 2014 and earlier. This causes excessive VLF fragmentation on busy systems.", f.FileName)
				targetMB := "512MB"
				if f.FileType == 1 {
					targetMB = "64MB"
				}
				fixScript = fmt.Sprintf("ALTER DATABASE [%s] MODIFY FILE (NAME = N'%s', FILEGROWTH = %s);", dbName, f.FileName, targetMB)
			}

			if severity == "" {
				continue
			}

			details, _ := json.Marshal(map[string]interface{}{
				"file_name":         f.FileName,
				"file_type":         f.FileType,
				"size_mb":           f.SizeMB,
				"growth":            f.Growth,
				"growth_mb":         growthMB,
				"is_percent_growth": f.IsPercentGrowth,
				"recommendation":    rec,
			})
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   dbName,
				Section:        "Storage & Growth",
				FindingType:    "autogrowth_risk",
				Severity:       severity,
				Title:          title,
				ObjectName:     f.FileName,
				ObjectType:     "database_file",
				FindingKey:     fmt.Sprintf("%s.file.%s", dbName, f.FileName),
				ImpactScore:    f.SizeMB,
				Details:        string(details),
				Recommendation: rec,
				FixScript:      fixScript,
			})
		}

		// VLF thresholds: <50 = OK (no finding), 50-500 = WARNING, >500 = CRITICAL
		if vlfCount >= 50 {
			severity := "WARNING"
			if vlfCount > 500 {
				severity = "CRITICAL"
			}
			rec := fmt.Sprintf("Database %s has %d VLFs. Excessive VLFs slow crash recovery, log backups, and database mirroring. Shrink the log file then expand with one large pre-allocated increment.", dbName, vlfCount)
			details, _ := json.Marshal(map[string]interface{}{
				"vlf_count":      vlfCount,
				"recommendation": rec,
			})
			findings = append(findings, hot.PerformanceDebtFindingRow{
				DatabaseName:   dbName,
				Section:        "Storage & Growth",
				FindingType:    "excessive_vlfs",
				Severity:       severity,
				Title:          fmt.Sprintf("Excessive VLFs: %d in %s", vlfCount, dbName),
				ObjectName:     dbName + ".log",
				ObjectType:     "log_file",
				FindingKey:     fmt.Sprintf("%s.vlf_count", dbName),
				ImpactScore:    float64(vlfCount),
				Details:        string(details),
				Recommendation: rec,
				FixScript: fmt.Sprintf(
					"-- Step 1: Check for active transactions, then shrink log\n-- Step 2: Expand with a single large allocation to reduce VLF count\nUSE [%s];\nDBCC SHRINKFILE (N'%s_log', 1);\nALTER DATABASE [%s] MODIFY FILE (NAME = N'%s_log', SIZE = 2048MB);",
					dbName, dbName, dbName, dbName,
				),
			})
		}
		if len(findings) > 0 {
			_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, findings)
		}
	}

	// 5. Backup & Recovery
	for _, dbName := range databases {
		ageHours, err := s.MsRepo.FetchLastFullBackupAgeHours(ctx, instanceName, dbName)
		if err != nil {
			slog.Error(fmt.Sprintf("[PerformanceDebt] ERROR FetchLastFullBackupAgeHours for %s/%s: %v", instanceName, dbName, err))
			continue
		}
		// Thresholds: >7 days → CRITICAL, >1 day → WARNING (industry standard)
		severity := ""
		if ageHours >= 999999 {
			severity = "CRITICAL"
		} else if ageHours > 168 { // 7 days
			severity = "CRITICAL"
		} else if ageHours > 24 { // 1 day
			severity = "WARNING"
		}
		if severity != "" {
			ageDesc := fmt.Sprintf("%.1f hours", ageHours)
			if ageHours >= 999999 {
				ageDesc = "never backed up"
			}
			rec := fmt.Sprintf("Database %s last full backup was %s ago. Full backups should run at least every 24 hours.", dbName, ageDesc)
			details, _ := json.Marshal(map[string]interface{}{
				"database_name":    dbName,
				"backup_age_hours": ageHours,
				"recommendation":   rec,
			})
			_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, []hot.PerformanceDebtFindingRow{{
				DatabaseName:   dbName,
				Section:        "Backup & Recovery",
				FindingType:    "backup_age",
				Severity:       severity,
				Title:          fmt.Sprintf("Overdue Full Backup: %s (%s)", dbName, ageDesc),
				ObjectName:     dbName,
				ObjectType:     "database",
				FindingKey:     fmt.Sprintf("%s.full_backup_age", dbName),
				ImpactScore:    ageHours,
				Details:        string(details),
				Recommendation: rec,
			}})
		}

		// Log backup check: only relevant for FULL and BULK_LOGGED recovery model databases.
		recoveryModel, err := s.MsRepo.FetchDatabaseRecoveryModel(ctx, instanceName, dbName)
		if err == nil && (recoveryModel == "FULL" || recoveryModel == "BULK_LOGGED") {
			logAgeHours, logErr := s.MsRepo.FetchLastLogBackupAgeHours(ctx, instanceName, dbName)
			if logErr == nil {
				logSeverity := ""
				if logAgeHours >= 999999 {
					logSeverity = "CRITICAL" // no log backups ever → log space exhaustion risk
				} else if logAgeHours > 4 {
					logSeverity = "WARNING"
				}
				if logSeverity != "" {
					logAgeDesc := fmt.Sprintf("%.1f hours", logAgeHours)
					if logAgeHours >= 999999 {
						logAgeDesc = "never backed up (log chain broken)"
					}
					logRec := fmt.Sprintf(
						"Database %s (%s recovery) last log backup was %s ago. Without regular log backups the transaction log cannot be truncated, point-in-time recovery is unavailable, and log space exhaustion is a risk.",
						dbName, recoveryModel, logAgeDesc,
					)
					logDetails, _ := json.Marshal(map[string]interface{}{
						"database_name":        dbName,
						"recovery_model":       recoveryModel,
						"log_backup_age_hours": logAgeHours,
						"recommendation":       logRec,
					})
					_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, []hot.PerformanceDebtFindingRow{{
						DatabaseName:   dbName,
						Section:        "Backup & Recovery",
						FindingType:    "log_backup_age",
						Severity:       logSeverity,
						Title:          fmt.Sprintf("Overdue Log Backup: %s (%s)", dbName, logAgeDesc),
						ObjectName:     dbName,
						ObjectType:     "database",
						FindingKey:     fmt.Sprintf("%s.log_backup_age", dbName),
						ImpactScore:    logAgeHours,
						Details:        string(logDetails),
						Recommendation: logRec,
					}})
				}
			}
		}
	}

	// 6. SQL Agent (instance-level, not per-database)
	{
		var agentFindings []hot.PerformanceDebtFindingRow

		failed, err := s.MsRepo.FetchFailedAgentJobs24h(ctx, instanceName, 20)
		if err != nil {
			slog.Error("[PerformanceDebt] ERROR FetchFailedAgentJobs24h", "target", instanceName, "err", err)
		} else {
			for _, job := range failed {
				name, _ := job["job_name"].(string)
				runDt, _ := job["run_dt"].(time.Time)
				rec := fmt.Sprintf("SQL Agent job '%s' failed at %s. Investigate job history.", name, runDt.Format(time.RFC3339))
				details, _ := json.Marshal(map[string]interface{}{
					"job_name":       name,
					"run_dt":         runDt.Format(time.RFC3339),
					"recommendation": rec,
				})
				agentFindings = append(agentFindings, hot.PerformanceDebtFindingRow{
					DatabaseName:   "msdb",
					Section:        "SQL Agent",
					FindingType:    "failed_job",
					Severity:       "WARNING",
					Title:          fmt.Sprintf("Failed Agent Job: %s", name),
					ObjectName:     name,
					ObjectType:     "agent_job",
					FindingKey:     fmt.Sprintf("agent.failed.%s", name),
					ImpactScore:    1,
					Details:        string(details),
					Recommendation: rec,
				})
			}
		}

		disabled, err := s.MsRepo.FetchDisabledAgentJobs(ctx, instanceName, 20)
		if err != nil {
			slog.Error("[PerformanceDebt] ERROR FetchDisabledAgentJobs", "target", instanceName, "err", err)
		} else if len(disabled) > 0 {
			rec := fmt.Sprintf("%d SQL Agent jobs are disabled. Review whether they should be re-enabled.", len(disabled))
			details, _ := json.Marshal(map[string]interface{}{
				"disabled_jobs":  disabled,
				"count":          len(disabled),
				"recommendation": rec,
			})
			agentFindings = append(agentFindings, hot.PerformanceDebtFindingRow{
				DatabaseName:   "msdb",
				Section:        "SQL Agent",
				FindingType:    "disabled_jobs",
				Severity:       "INFO",
				Title:          fmt.Sprintf("%d Agent Jobs Disabled", len(disabled)),
				ObjectName:     "SQL Agent",
				ObjectType:     "agent_job",
				FindingKey:     "agent.disabled_jobs",
				ImpactScore:    float64(len(disabled)),
				Details:        string(details),
				Recommendation: rec,
			})
		}
		if len(agentFindings) > 0 {
			_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, agentFindings)
		}
	}

	// 7. Engine Config (instance-level)
	{
		type configCheck struct {
			name      string
			findingFn func(val int64) (severity, title, rec string, impact float64, ok bool)
		}
		checks := []configCheck{
			{
				name: "optimize for ad hoc workloads",
				findingFn: func(val int64) (string, string, string, float64, bool) {
					if val == 1 {
						return "", "", "", 0, false
					}
					return "WARNING",
						"Engine Config: optimize for ad hoc workloads is OFF",
						"Enable 'optimize for ad hoc workloads' to reduce plan cache bloat from single-use plans.",
						10, true
				},
			},
			{
				name: "cost threshold for parallelism",
				findingFn: func(val int64) (string, string, string, float64, bool) {
					if val > 5 {
						return "", "", "", 0, false
					}
					return "WARNING",
						fmt.Sprintf("Engine Config: cost threshold for parallelism = %d (too low)", val),
						"Cost threshold for parallelism of 5 is the old default. Raise to 25–50 to reduce unnecessary parallelism on OLTP workloads.",
						float64(val), true
				},
			},
			{
				name: "max degree of parallelism",
				findingFn: func(val int64) (string, string, string, float64, bool) {
					if val != 0 {
						return "", "", "", 0, false
					}
					return "INFO",
						"Engine Config: MAXDOP = 0 (unlimited)",
						"MAXDOP 0 allows queries to use all CPUs. Consider setting to half the logical CPU count on OLTP systems.",
						float64(val), true
				},
			},
			{
				name: "max server memory (MB)",
				findingFn: func(val int64) (string, string, string, float64, bool) {
					if val < 2147483647 {
						return "", "", "", 0, false
					}
					return "WARNING",
						"Engine Config: max server memory is uncapped (2147483647 MB)",
						"SQL Server max server memory is at default (unlimited). Set an explicit ceiling to leave OS memory headroom and prevent memory pressure.",
						float64(val), true
				},
			},
			{
				name: "xp_cmdshell",
				findingFn: func(val int64) (string, string, string, float64, bool) {
					if val == 0 {
						return "", "", "", 0, false
					}
					return "CRITICAL",
						"Engine Config: xp_cmdshell is ENABLED",
						"xp_cmdshell allows SQL Server to execute arbitrary OS commands. This is a significant security risk. Disable unless absolutely required and compensating controls are in place.",
						float64(val), true
				},
			},
			{
				name: "remote admin connections",
				findingFn: func(val int64) (string, string, string, float64, bool) {
					if val == 1 {
						return "", "", "", 0, false
					}
					return "INFO",
						"Engine Config: remote admin connections (DAC) is disabled",
						"Enabling the Dedicated Admin Connection (DAC) allows emergency remote access when the server is under extreme load and cannot accept regular connections.",
						float64(val), true
				},
			},
			{
				name: "backup compression default",
				findingFn: func(val int64) (string, string, string, float64, bool) {
					if val == 1 {
						return "", "", "", 0, false
					}
					return "INFO",
						"Engine Config: backup compression default is OFF",
						"Enabling backup compression by default reduces backup size and I/O duration. Enable unless storage constraints or compliance policies require uncompressed backups.",
						float64(val), true
				},
			},
		}

		var configFindings []hot.PerformanceDebtFindingRow
		for _, chk := range checks {
			val, err := s.MsRepo.FetchConfigValueInUse(ctx, instanceName, chk.name)
			if err != nil {
				continue
			}
			severity, title, rec, impact, flagged := chk.findingFn(val)
			if !flagged {
				continue
			}
			details, _ := json.Marshal(map[string]interface{}{
				"config_name":    chk.name,
				"value_in_use":   val,
				"recommendation": rec,
			})
			configFindings = append(configFindings, hot.PerformanceDebtFindingRow{
				DatabaseName:   "master",
				Section:        "Engine Config",
				FindingType:    "config_risk",
				Severity:       severity,
				Title:          title,
				ObjectName:     chk.name,
				ObjectType:     "server_config",
				FindingKey:     fmt.Sprintf("config.%s", strings.ReplaceAll(chk.name, " ", "_")),
				ImpactScore:    impact,
				Details:        string(details),
				Recommendation: rec,
				FixScript:      fmt.Sprintf("EXEC sp_configure '%s', <value>; RECONFIGURE;", chk.name),
			})
		}
		if len(configFindings) > 0 {
			_ = s.tsLogger.LogPerformanceDebtFindings(ctx, serverID, configFindings)
		}
	}

	slog.Info("[PerformanceDebt] Analysis complete", "arg1", instanceName, "arg2", serverID)
}
