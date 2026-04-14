// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background worker for performance debt analysis and debt accumulation tracking.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// StartPerformanceDebtCollector collects maintenance/risk findings periodically (default hourly).
// Enable/adjust interval by setting PERFDEBT_INTERVAL_MIN (min 15, max 1440).
func (s *MetricsService) StartPerformanceDebtCollector(ctx context.Context) {
	if s.tsLogger == nil {
		log.Printf("[PerfDebtCollector] TimescaleDB not connected; collector disabled")
		return
	}

	interval := time.Hour
	if v := strings.TrimSpace(os.Getenv("PERFDEBT_INTERVAL_MIN")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 15 && n <= 1440 {
			interval = time.Duration(n) * time.Minute
		}
	}
	log.Printf("[PerfDebtCollector] Starting Performance Debt collector (interval: %v)...", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run once on startup for fast first data.
	s.collectPerformanceDebtOnce()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[PerfDebtCollector] Shutting down Performance Debt collector")
			return
		case <-ticker.C:
			s.collectPerformanceDebtOnce()
		}
	}
}

func (s *MetricsService) collectPerformanceDebtOnce() {
	if s.tsLogger == nil {
		return
	}
	var wg sync.WaitGroup
	for _, inst := range s.Config.Instances {
		if inst.Type != "sqlserver" {
			continue
		}
		wg.Add(1)
		go func(instanceName string, databases []string) {
			defer wg.Done()
			s.collectPerformanceDebtForInstance(instanceName, databases)
		}(inst.Name, inst.Databases)
	}
	wg.Wait()
}

func (s *MetricsService) collectPerformanceDebtForInstance(instanceName string, databases []string) {
	if s.tsLogger == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	now := time.Now().UTC()
	findings := make([]hot.PerformanceDebtFindingRow, 0, 600)

	// Delta de-duplication: load latest fingerprints by finding_key so we only persist changes.
	prev, err := s.tsLogger.GetLatestPerformanceDebtFingerprintHashesByKey(ctx, instanceName, 4*time.Hour)
	if err != nil {
		// If this fails, we still collect (worst case: duplicates as before).
		log.Printf("[PerfDebtCollector] fingerprint lookup failed instance=%s err=%v", instanceName, err)
		prev = map[string]string{}
	}
	seenThisRun := map[string]bool{}

	if len(databases) == 0 {
		databases = []string{"master"}
	}

	// helper to append only when new/changed
	appendIfChanged := func(r hot.PerformanceDebtFindingRow) {
		k := strings.TrimSpace(r.FindingKey)
		if k == "" {
			return
		}
		if seenThisRun[k] {
			return
		}
		seenThisRun[k] = true

		currHash := hot.PerformanceDebtFingerprintHash(r)
		if prevHash, ok := prev[k]; ok && prevHash == currHash {
			return
		}
		findings = append(findings, r)
	}

	for _, db := range databases {
		if strings.EqualFold(db, "tempdb") {
			continue
		}

		// ------------------------------------------------------------------
		// Index Health
		// ------------------------------------------------------------------
		if unused, err := s.MsRepo.FetchUnusedIndexes(instanceName, db, 1000, 50); err == nil {
			for _, u := range unused {
				details, _ := json.Marshal(map[string]any{
					"user_updates": u.UserUpdates,
					"total_reads":  u.TotalReads,
				})
				appendIfChanged(hot.PerformanceDebtFindingRow{
					CaptureTimestamp:   now,
					ServerInstanceName: instanceName,
					DatabaseName:       db,
					Section:            "Index Health",
					FindingType:        "unused_index",
					Severity:           "WARNING",
					Title:              "Unused index candidate (writes without reads)",
					ObjectName:         fmt.Sprintf("%s.%s", u.TableName, u.IndexName),
					ObjectType:         "index",
					FindingKey:         fmt.Sprintf("%s.%s:%s", db, u.TableName, u.IndexName),
					Details:            details,
					Recommendation:     "Validate with workload and consider dropping if truly unused.",
					FixScript:          fmt.Sprintf("/* Review first */\nUSE %s;\nDROP INDEX %s ON %s;", bracket(db), bracket(u.IndexName), bracket(u.TableName)),
				})
			}
		}

		if miss, err := s.MsRepo.FetchMissingIndexRecommendations(instanceName, db, 25); err == nil {
			for _, m := range miss {
				sev := "INFO"
				if m.ImprovementScore >= 1_000_000 {
					sev = "CRITICAL"
				} else if m.ImprovementScore >= 250_000 {
					sev = "WARNING"
				}
				details, _ := json.Marshal(map[string]any{
					"improvement_score":  m.ImprovementScore,
					"table":              m.TableName,
					"equality_columns":   m.EqualityColumns,
					"inequality_columns": m.InequalityCols,
					"included_columns":   m.IncludedColumns,
				})
				appendIfChanged(hot.PerformanceDebtFindingRow{
					CaptureTimestamp:   now,
					ServerInstanceName: instanceName,
					DatabaseName:       db,
					Section:            "Index Health",
					FindingType:        "missing_index",
					Severity:           sev,
					Title:              "Missing index recommendation",
					ObjectName:         m.TableName,
					ObjectType:         "table",
					FindingKey:         fmt.Sprintf("%s:%s:%s:%s", db, m.TableName, m.EqualityColumns, m.InequalityCols),
					Details:            details,
					Recommendation:     "Validate against Query Store/top offenders before implementing.",
					FixScript:          "",
				})
			}
		}

		if frags, err := s.MsRepo.FetchIndexFragmentation(instanceName, db, 30, 1000, 50); err == nil {
			for _, f := range frags {
				sev := "WARNING"
				if f.FragPct >= 60 {
					sev = "CRITICAL"
				}
				details, _ := json.Marshal(map[string]any{
					"fragmentation_pct": f.FragPct,
					"page_count":        f.PageCount,
				})
				appendIfChanged(hot.PerformanceDebtFindingRow{
					CaptureTimestamp:   now,
					ServerInstanceName: instanceName,
					DatabaseName:       db,
					Section:            "Index Health",
					FindingType:        "index_fragmentation",
					Severity:           sev,
					Title:              "High index fragmentation",
					ObjectName:         fmt.Sprintf("%s.%s", f.TableName, f.IndexName),
					ObjectType:         "index",
					FindingKey:         fmt.Sprintf("%s.%s:%s", db, f.TableName, f.IndexName),
					Details:            details,
					Recommendation:     "Rebuild/reorganize as appropriate; verify fillfactor and workload pattern.",
					FixScript:          fmt.Sprintf("/* Review first */\nUSE %s;\nALTER INDEX %s ON %s REORGANIZE;", bracket(db), bracket(f.IndexName), bracket(f.TableName)),
				})
			}
		}

		// ------------------------------------------------------------------
		// Statistics Health
		// ------------------------------------------------------------------
		if stats, err := s.MsRepo.FetchStaleStatistics(instanceName, db, 0.20, 50); err == nil {
			for _, st := range stats {
				sev := "WARNING"
				if st.Rows > 0 && float64(st.ModificationCounter) > float64(st.Rows)*0.5 {
					sev = "CRITICAL"
				}
				last := ""
				if st.LastUpdated.Valid {
					last = st.LastUpdated.Time.UTC().Format(time.RFC3339)
				}
				details, _ := json.Marshal(map[string]any{
					"rows":                 st.Rows,
					"modification_counter": st.ModificationCounter,
					"last_updated":         last,
				})
				appendIfChanged(hot.PerformanceDebtFindingRow{
					CaptureTimestamp:   now,
					ServerInstanceName: instanceName,
					DatabaseName:       db,
					Section:            "Statistics Health",
					FindingType:        "stale_stats",
					Severity:           sev,
					Title:              "Stale statistics (high modifications)",
					ObjectName:         fmt.Sprintf("%s.%s", st.TableName, st.StatsName),
					ObjectType:         "stats",
					FindingKey:         fmt.Sprintf("%s.%s:%s", db, st.TableName, st.StatsName),
					Details:            details,
					Recommendation:     "Update statistics (consider FULLSCAN for critical objects).",
					FixScript:          fmt.Sprintf("USE %s;\nUPDATE STATISTICS %s %s WITH FULLSCAN;", bracket(db), bracket(st.TableName), bracket(st.StatsName)),
				})
			}
		}

		// ------------------------------------------------------------------
		// Storage & Growth
		// ------------------------------------------------------------------
		if files, err := s.MsRepo.FetchAutogrowthRisks(instanceName, db, 25); err == nil {
			for _, f := range files {
				sev := "INFO"
				// Percent growth and tiny fixed growth can cause frequent autogrowth.
				if f.IsPercentGrowth {
					sev = "WARNING"
				} else if f.Growth > 0 && f.Growth < 512 { // pages; <4MB
					sev = "WARNING"
				}
				details, _ := json.Marshal(map[string]any{
					"file":             f.FileName,
					"size_mb":          f.SizeMB,
					"growth":           f.Growth,
					"is_percent_growth": f.IsPercentGrowth,
				})
				appendIfChanged(hot.PerformanceDebtFindingRow{
					CaptureTimestamp:   now,
					ServerInstanceName: instanceName,
					DatabaseName:       db,
					Section:            "Storage & Growth",
					FindingType:        "autogrowth_risk",
					Severity:           sev,
					Title:              "Potential autogrowth risk",
					ObjectName:         f.FileName,
					ObjectType:         "database_file",
					FindingKey:         fmt.Sprintf("%s:file:%s", db, f.FileName),
					Details:            details,
					Recommendation:     "Prefer fixed growth in MB sized to workload; avoid percent growth for large files.",
					FixScript:          "",
				})
			}
		}

		if vlf, err := s.MsRepo.FetchVLFCount(instanceName, db); err == nil {
			sev := "INFO"
			if vlf >= 1000 {
				sev = "CRITICAL"
			} else if vlf >= 500 {
				sev = "WARNING"
			}
			details, _ := json.Marshal(map[string]any{
				"vlf_count": vlf,
			})
			appendIfChanged(hot.PerformanceDebtFindingRow{
				CaptureTimestamp:   now,
				ServerInstanceName: instanceName,
				DatabaseName:       db,
				Section:            "Storage & Growth",
				FindingType:        "vlf_high",
				Severity:           sev,
				Title:              "High VLF count",
				ObjectName:         db,
				ObjectType:         "database",
				FindingKey:         fmt.Sprintf("%s:vlf", db),
				Details:            details,
				Recommendation:     "Consider log file sizing and growth settings; rebuild log to reduce VLFs if needed.",
				FixScript:          "",
			})
		}

		// ------------------------------------------------------------------
		// Backup & Recovery
		// ------------------------------------------------------------------
		if ageH, err := s.MsRepo.FetchLastFullBackupAgeHours(instanceName, db); err == nil {
			sev := "INFO"
			if ageH >= 168 { // 7d
				sev = "CRITICAL"
			} else if ageH >= 48 {
				sev = "WARNING"
			}
			details, _ := json.Marshal(map[string]any{
				"last_full_backup_age_hours": ageH,
			})
			appendIfChanged(hot.PerformanceDebtFindingRow{
				CaptureTimestamp:   now,
				ServerInstanceName: instanceName,
				DatabaseName:       db,
				Section:            "Backup & Recovery",
				FindingType:        "backup_age",
				Severity:           sev,
				Title:              "Full backup age",
				ObjectName:         db,
				ObjectType:         "database",
				FindingKey:         fmt.Sprintf("%s:full_backup_age", db),
				Details:            details,
				Recommendation:     "Ensure backups meet RPO/RTO policy and verify backup jobs are running successfully.",
				FixScript:          "",
			})
		}
	}

	// ----------------------------------------------------------------------
	// SQL Agent (instance-level)
	// ----------------------------------------------------------------------
	if failed, err := s.MsRepo.FetchFailedAgentJobs24h(instanceName, 50); err == nil && len(failed) > 0 {
		details, _ := json.Marshal(map[string]any{"failed_jobs": failed})
		appendIfChanged(hot.PerformanceDebtFindingRow{
			CaptureTimestamp:   now,
			ServerInstanceName: instanceName,
			DatabaseName:       "msdb",
			Section:            "SQL Agent",
			FindingType:        "job_failed",
			Severity:           "CRITICAL",
			Title:              "SQL Agent job failures (last 24h)",
			ObjectName:         "SQL Agent",
			ObjectType:         "job",
			FindingKey:         "agent:failed_jobs_24h",
			Details:            details,
			Recommendation:     "Investigate job history and fix the underlying failure (permissions, storage, blocking, etc.).",
			FixScript:          "",
		})
	}

	if disabled, err := s.MsRepo.FetchDisabledAgentJobs(instanceName, 200); err == nil && len(disabled) > 0 {
		details, _ := json.Marshal(map[string]any{"disabled_jobs": disabled})
		appendIfChanged(hot.PerformanceDebtFindingRow{
			CaptureTimestamp:   now,
			ServerInstanceName: instanceName,
			DatabaseName:       "msdb",
			Section:            "SQL Agent",
			FindingType:        "job_disabled",
			Severity:           "WARNING",
			Title:              "Disabled SQL Agent jobs",
			ObjectName:         "SQL Agent",
			ObjectType:         "job",
			FindingKey:         "agent:disabled_jobs",
			Details:            details,
			Recommendation:     "Confirm whether disabled jobs are intentional; disable stale jobs or fix scheduling/ownership.",
			FixScript:          "",
		})
	}

	// ----------------------------------------------------------------------
	// Engine Config (instance-level)
	// ----------------------------------------------------------------------
	if maxdop, err := s.MsRepo.FetchConfigValueInUse(instanceName, "max degree of parallelism"); err == nil {
		sev := "INFO"
		if maxdop == 0 {
			sev = "WARNING"
		}
		details, _ := json.Marshal(map[string]any{"maxdop": maxdop})
		appendIfChanged(hot.PerformanceDebtFindingRow{
			CaptureTimestamp:   now,
			ServerInstanceName: instanceName,
			DatabaseName:       "master",
			Section:            "Engine Config",
			FindingType:        "maxdop",
			Severity:           sev,
			Title:              "MAXDOP configuration",
			ObjectName:         "sp_configure",
			ObjectType:         "config",
			FindingKey:         "config:maxdop",
			Details:            details,
			Recommendation:     "Set MAXDOP based on CPU topology and workload; avoid 0 for many OLTP systems.",
			FixScript:          "EXEC sp_configure 'show advanced options', 1; RECONFIGURE; EXEC sp_configure 'max degree of parallelism', 8; RECONFIGURE;",
		})
	}

	if ctfp, err := s.MsRepo.FetchConfigValueInUse(instanceName, "cost threshold for parallelism"); err == nil {
		sev := "INFO"
		if ctfp < 20 {
			sev = "WARNING"
		}
		details, _ := json.Marshal(map[string]any{"cost_threshold_for_parallelism": ctfp})
		appendIfChanged(hot.PerformanceDebtFindingRow{
			CaptureTimestamp:   now,
			ServerInstanceName: instanceName,
			DatabaseName:       "master",
			Section:            "Engine Config",
			FindingType:        "cost_threshold_for_parallelism",
			Severity:           sev,
			Title:              "Cost threshold for parallelism",
			ObjectName:         "sp_configure",
			ObjectType:         "config",
			FindingKey:         "config:ctfp",
			Details:            details,
			Recommendation:     "Consider raising above default 5 to reduce trivial parallelism.",
			FixScript:          "EXEC sp_configure 'show advanced options', 1; RECONFIGURE; EXEC sp_configure 'cost threshold for parallelism', 25; RECONFIGURE;",
		})
	}

	if err := s.tsLogger.LogPerformanceDebtFindings(ctx, findings); err != nil {
		log.Printf("[PerfDebtCollector] insert error instance=%s findings=%d err=%v", instanceName, len(findings), err)
		return
	}

	// Retention/cleanup: keep only meaningful history.
	keepPerKey := 10
	if v := strings.TrimSpace(os.Getenv("PERFDEBT_KEEP_PER_KEY")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 200 {
			keepPerKey = n
		}
	}
	retentionDays := 90
	if v := strings.TrimSpace(os.Getenv("PERFDEBT_RETENTION_DAYS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 7 && n <= 3650 {
			retentionDays = n
		}
	}
	if err := s.tsLogger.CleanupPerformanceDebtFindings(ctx, instanceName, keepPerKey, time.Duration(retentionDays)*24*time.Hour); err != nil {
		log.Printf("[PerfDebtCollector] retention cleanup failed instance=%s err=%v", instanceName, err)
	}

	log.Printf("[PerfDebtCollector] instance=%s findings_written=%d retention_keep_per_key=%d retention_days=%d", instanceName, len(findings), keepPerKey, retentionDays)
}

func bracket(s string) string {
	s = strings.ReplaceAll(s, "]", "")
	return "[" + s + "]"
}

