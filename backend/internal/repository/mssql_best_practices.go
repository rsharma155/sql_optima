// Package repository handles best practices configuration auditing for SQL Server
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server best practices configuration validation.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

// FetchBestPractices executes configuration audit queries and applies health rules
func (c *MssqlRepository) FetchBestPractices(instanceName string) models.BestPracticesResult {
	var result models.BestPracticesResult
	result.InstanceName = instanceName
	result.Timestamp = fmt.Sprintf("%d", time.Now().Unix())

	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("No database connection for instance: %s", instanceName)
		return result
	}

	// Query 1: Instance Level Configurations
	serverConfigs, err := c.queryServerConfigurations(db)
	if err != nil {
		log.Printf("Error querying server configurations: %v", err)
	} else {
		result.ServerConfig = c.evaluateServerRules(serverConfigs)
	}

	// Query 2: Database Level Configurations
	dbConfigs, err := c.queryDatabaseConfigurations(db)
	if err != nil {
		log.Printf("Error querying database configurations: %v", err)
	} else {
		result.DatabaseConfig = evaluateDatabaseRules(dbConfigs)
	}

	return result
}

// queryServerConfigurations fetches server-level configuration settings
func (c *MssqlRepository) queryServerConfigurations(db *sql.DB) (map[string]string, error) {
	query := `
		SELECT
			name AS [Configuration_Name],
			CAST(value_in_use AS VARCHAR(50)) AS [Current_Value]
		FROM sys.configurations WITH (NOLOCK)
		WHERE name IN (
			'max server memory (MB)',
			'max degree of parallelism',
			'cost threshold for parallelism',
			'optimize for ad hoc workloads',
			'backup compression default'
		)
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	configs := make(map[string]string)
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		configs[name] = value
	}

	return configs, rows.Err()
}

// queryDatabaseConfigurations fetches database-level configuration settings
func (c *MssqlRepository) queryDatabaseConfigurations(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT
			name AS [Database_Name],
			page_verify_option_desc AS [Page_Verify],
			is_auto_shrink_on AS [Auto_Shrink],
			is_auto_close_on AS [Auto_Close],
			target_recovery_time_in_seconds AS [Target_Recovery_Time]
		FROM sys.databases WITH (NOLOCK)
		WHERE database_id > 4 -- Exclude system databases
		  AND state_desc = 'ONLINE'
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []map[string]interface{}
	for rows.Next() {
		var dbName, pageVerify string
		var autoShrink, autoClose bool
		var targetRecoveryTime int

		if err := rows.Scan(&dbName, &pageVerify, &autoShrink, &autoClose, &targetRecoveryTime); err != nil {
			return nil, err
		}

		config := map[string]interface{}{
			"database_name":        dbName,
			"page_verify":          pageVerify,
			"auto_shrink":          autoShrink,
			"auto_close":           autoClose,
			"target_recovery_time": targetRecoveryTime,
		}
		configs = append(configs, config)
	}

	return configs, rows.Err()
}

// evaluateServerRules applies health rules to server configurations
func (c *MssqlRepository) evaluateServerRules(configs map[string]string) []models.ServerConfigCheck {
	var checks []models.ServerConfigCheck

	// Max Server Memory check
	if value, exists := configs["max server memory (MB)"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "Max Server Memory (MB)",
			CurrentValue:      value,
			Status:            "GREEN",
		}

		if value == "2147483647" {
			check.Status = "RED"
			check.Message = "SQL Server memory is uncapped. It will eventually starve the Windows OS. Cap it to leave at least 4GB-8GB for the OS."
		}

		checks = append(checks, check)
	}

	// Cost Threshold for Parallelism check
	if value, exists := configs["cost threshold for parallelism"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "Cost Threshold for Parallelism",
			CurrentValue:      value,
			Status:            "GREEN",
		}

		if value == "5" {
			check.Status = "YELLOW"
			check.Message = "Default value of 5 is too low for modern workloads. Consider raising to 50 to prevent trivial queries from using multiple CPUs."
		}

		checks = append(checks, check)
	}

	// Optimize for Ad Hoc Workloads check
	if value, exists := configs["optimize for ad hoc workloads"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "Optimize for Ad Hoc Workloads",
			CurrentValue:      value,
			Status:            "GREEN",
		}

		if value == "0" {
			check.Status = "YELLOW"
			check.Message = "Plan cache bloat risk. Enable this to prevent single-use queries from stealing cache memory."
		}

		checks = append(checks, check)
	}

	// Backup Compression Default check
	if value, exists := configs["backup compression default"]; exists {
		check := models.ServerConfigCheck{
			ConfigurationName: "Backup Compression Default",
			CurrentValue:      value,
			Status:            "GREEN",
		}

		if value == "0" {
			check.Status = "YELLOW"
			check.Message = "Enable backup compression to save disk space and reduce disk I/O during backup windows."
		}

		checks = append(checks, check)
	}

	return checks
}

func evaluateDatabaseRules(configs []map[string]interface{}) []models.DatabaseConfigCheck {
	var checks []models.DatabaseConfigCheck
	for _, config := range configs {
		dbName := config["database_name"].(string)
		pageVerify := config["page_verify"].(string)
		autoShrink := config["auto_shrink"].(bool)
		autoClose := config["auto_close"].(bool)
		targetRecoveryTime := config["target_recovery_time"].(int)
		check := models.DatabaseConfigCheck{
			DatabaseName: dbName, PageVerify: pageVerify, AutoShrink: autoShrink,
			AutoClose: autoClose, TargetRecoveryTime: targetRecoveryTime, Status: "GREEN", Message: "",
		}
		if autoShrink {
			check.Status = "RED"
			check.Message = "Auto Shrink is enabled. Turn this off immediately."
		}
		if autoClose {
			check.Status = "RED"
			check.Message = "Auto Close is enabled. Turn this off immediately."
		}
		if !strings.EqualFold(pageVerify, "CHECKSUM") {
			check.Status = "RED"
			check.Message = "Page verify is not set to CHECKSUM."
		}
		if targetRecoveryTime == 0 {
			check.Status = "YELLOW"
			check.Message = "Set Target Recovery Time to 60 seconds."
		}
		checks = append(checks, check)
	}
	return checks
}

// FetchGuardrails executes comprehensive guardrails audit for SQL Server
func (c *MssqlRepository) FetchGuardrails(instanceName string) models.GuardrailsResult {
	var result models.GuardrailsResult
	result.InstanceName = instanceName
	result.Timestamp = fmt.Sprintf("%d", time.Now().Unix())

	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("No database connection for instance: %s", instanceName)
		return result
	}

	result.StorageRisks = c.queryStorageRisks(db)
	result.DiskSpace = c.queryDiskSpace(db)
	result.LogHealth = c.queryLogHealth(db)
	result.LogBackups = c.queryLogBackups(db)
	result.LongTxns = c.queryLongRunningTransactions(db)
	result.Autogrowth = c.queryAutogrowth(db)
	result.TempDBConfig = c.queryTempDBConfig(db)
	result.ResourceGov = c.queryResourceGovernor(db)

	result.HealthScore, result.HealthStatus = c.calculateHealthScore(result)
	result.Summary = c.generateRiskSummary(result)

	return result
}

func (c *MssqlRepository) queryStorageRisks(db *sql.DB) []models.StorageRisk {
	query := `
		SELECT DB_NAME(mf.database_id), mf.type_desc, mf.name, mf.physical_name, mf.size * 8 / 1024, LEFT(mf.physical_name, 1)
		FROM sys.master_files mf WITH (NOLOCK)
		WHERE mf.database_id > 4 AND mf.state_desc = 'ONLINE' AND mf.physical_name LIKE 'C:%'
		ORDER BY mf.size DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var risks []models.StorageRisk
	for rows.Next() {
		var r models.StorageRisk
		if err := rows.Scan(&r.DatabaseName, &r.FileType, &r.LogicalName, &r.PhysicalName, &r.SizeMB, &r.Path); err == nil {
			r.Severity = "CRITICAL"
			r.Message = fmt.Sprintf("Database file on C: drive - %s (%dMB)", r.FileType, r.SizeMB)
			r.DrillDown = "Storing data/logs on the OS drive risks system crashes if the drive fills. Move files to a data drive immediately."
			r.MitigationSQL = "-- Step 1: Add new files to a different drive\nALTER DATABASE [DB_NAME] ADD FILE (NAME = N'NewData', FILENAME = N'D:\\Data\\NewData.ndf');\n-- Step 2: Move existing files (Requires OFFLINE)\nALTER DATABASE [DB_NAME] SET OFFLINE;\n-- Physically move files then:\nALTER DATABASE [DB_NAME] MODIFY FILE (NAME = [LogicalName], FILENAME = 'New_Path');\nALTER DATABASE [DB_NAME] SET ONLINE;"
			risks = append(risks, r)
		}
	}

	// Check for data and log files on same drive
	sameDriveQuery := `
		SELECT DB_NAME(database_id) AS db_name, LEFT(physical_name, 1) AS drive
		FROM sys.master_files WITH (NOLOCK)
		WHERE database_id > 4 AND state_desc = 'ONLINE'
		GROUP BY DB_NAME(database_id), LEFT(physical_name, 1)
		HAVING COUNT(DISTINCT type_desc) > 1
	`
	rows2, err2 := db.Query(sameDriveQuery)
	if err2 == nil {
		defer rows2.Close()
		for rows2.Next() {
			var dbName, drive string
			if err := rows2.Scan(&dbName, &drive); err == nil {
				risks = append(risks, models.StorageRisk{
					DatabaseName:  dbName,
					FileType:      "DATA+LOG",
					Path:          drive + ":\\",
					Severity:      "WARNING",
					Message:       fmt.Sprintf("Data and Log files on same drive %s:", drive),
					DrillDown:     "Co-locating Data and Logs limits I/O throughput due to conflicting read/write patterns. Consider separating to different drives.",
					MitigationSQL: "-- Move log file to separate drive\nALTER DATABASE [" + dbName + "] MODIFY FILE (NAME = N'" + dbName + "_log', FILENAME = 'E:\\Data\\" + dbName + "_log.ldf');",
				})
			}
		}
	}

	return risks
}

func (c *MssqlRepository) queryDiskSpace(db *sql.DB) []models.DiskSpaceInfo {
	// Get disk space with log file sizes
	query := `
		SELECT 
			LEFT(vs.volume_mount_point, 1) AS drive_letter,
			vs.total_bytes / 1024 / 1024 AS total_size_mb,
			vs.available_bytes / 1024 / 1024 AS free_space_mb,
			ISNULL(SUM(CASE WHEN mf.type_desc = 'LOG' THEN mf.size * 8 / 1024 ELSE 0 END), 0) AS log_size_mb
		FROM sys.dm_os_volume_stats() vs
		LEFT JOIN sys.master_files mf ON LEFT(mf.physical_name, 1) = LEFT(vs.volume_mount_point, 1) AND mf.database_id > 4 AND mf.state_desc = 'ONLINE'
		GROUP BY vs.volume_mount_point, vs.total_bytes, vs.available_bytes
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var disks []models.DiskSpaceInfo
	for rows.Next() {
		var d models.DiskSpaceInfo
		if err := rows.Scan(&d.DriveLetter, &d.TotalSizeMB, &d.FreeSpaceMB, &d.LogSizeMB); err == nil {
			d.FreePercent = float64(d.FreeSpaceMB) / float64(d.TotalSizeMB) * 100

			// Check if log growth would exceed free space
			d.LogGrowthGreaterThanFree = d.LogSizeMB > d.FreeSpaceMB

			if d.FreePercent < 5 || d.LogGrowthGreaterThanFree {
				d.Severity = "CRITICAL"
				if d.LogGrowthGreaterThanFree {
					d.Message = "Log file growth exceeds free disk space"
					d.DrillDown = "The volume is reaching capacity. The log file's defined growth increment exceeds the available disk space, which will cause the next write transaction to fail."
					d.MitigationSQL = "-- Perform log backup to truncate log\nBACKUP LOG [DB_NAME] TO DISK = 'D:\\Backup\\DB_NAME_log.bak';\n-- Or add physical disk capacity to volume " + d.DriveLetter + ":"
				}
			} else if d.FreePercent < 10 {
				d.Severity = "WARNING"
				d.DrillDown = "Disk space is low. Monitor and plan for capacity expansion."
			} else {
				d.Severity = "GREEN"
			}
			disks = append(disks, d)
		}
	}
	return disks
}

func (c *MssqlRepository) queryLogHealth(db *sql.DB) []models.LogHealthInfo {
	query := `
		SELECT d.name, d.recovery_model_desc, d.log_reuse_wait_desc
		FROM sys.databases d WITH (NOLOCK)
		WHERE d.database_id > 4 AND d.state_desc = 'ONLINE'
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var logs []models.LogHealthInfo
	for rows.Next() {
		var l models.LogHealthInfo
		if err := rows.Scan(&l.DatabaseName, &l.RecoveryModel, &l.LogReuseWait); err == nil {
			l.Severity = "GREEN"
			if l.LogReuseWait != "NOTHING" && l.LogReuseWait != "" {
				l.Severity = "WARNING"
				l.Message = fmt.Sprintf("Log reuse wait: %s", l.LogReuseWait)
				l.DrillDown = "Transaction log cannot truncate. Active transactions or backup chain breaks preventing log reuse."
				l.MitigationSQL = "-- Check what's blocking log reuse\nSELECT name, log_reuse_wait_desc FROM sys.databases WHERE name = '" + l.DatabaseName + "';"
			}
			logs = append(logs, l)
		}
	}

	// Get VLF count for each database
	vlfQuery := `
		SELECT DB_NAME(database_id), COUNT(*) AS vlf_count
		FROM sys.dm_db_log_info
		GROUP BY database_id
		HAVING DB_NAME(database_id) IS NOT NULL
	`
	vlfRows, _ := db.Query(vlfQuery)
	if vlfRows != nil {
		defer vlfRows.Close()
		vlfMap := make(map[string]int)
		for vlfRows.Next() {
			var dbName string
			var vlfCount int
			if err := vlfRows.Scan(&dbName, &vlfCount); err == nil {
				vlfMap[dbName] = vlfCount
			}
		}
		// Update VLF counts in existing logs
		for i := range logs {
			if vlf, ok := vlfMap[logs[i].DatabaseName]; ok {
				logs[i].VLFCount = vlf
				if vlf > 1000 {
					logs[i].Severity = "CRITICAL"
					logs[i].Message = fmt.Sprintf("VLF Count: %d (CRITICAL)", vlf)
					logs[i].DrillDown = "High VLF counts are caused by small, frequent autogrowth events. This slows down 'Crash Recovery' and backups."
					logs[i].MitigationSQL = "-- Shrink log to minimum and grow in large chunks to reset VLFs\nDBCC SHRINKFILE (N'" + logs[i].DatabaseName + "_log', 0, TRUNCATEONLY);\nALTER DATABASE [" + logs[i].DatabaseName + "] MODIFY FILE (NAME = N'" + logs[i].DatabaseName + "_log', SIZE = 4096MB);"
				} else if vlf > 500 {
					if logs[i].Severity == "GREEN" {
						logs[i].Severity = "WARNING"
					}
					logs[i].Message = fmt.Sprintf("VLF Count: %d (WARNING)", vlf)
					logs[i].DrillDown = "High VLF counts can impact recovery time."
				}
			}
		}
	}

	return logs
}

func (c *MssqlRepository) queryLogBackups(db *sql.DB) []models.LogBackupInfo {
	query := `
		SELECT d.name, MAX(b.backup_finish_date), DATEDIFF(MINUTE, MAX(b.backup_finish_date), GETDATE())
		FROM sys.databases d WITH (NOLOCK)
		LEFT JOIN msdb.dbo.backupset b ON d.name = b.database_name AND b.type = 'L' AND b.backup_finish_date >= DATEADD(DAY, -7, GETDATE())
		WHERE d.database_id > 4 AND d.recovery_model_desc IN ('FULL', 'BULK_LOGGED') AND d.state_desc = 'ONLINE'
		GROUP BY d.name
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var backups []models.LogBackupInfo
	for rows.Next() {
		var b models.LogBackupInfo
		var lastBackup sql.NullString
		var mins sql.NullInt64
		if err := rows.Scan(&b.DatabaseName, &lastBackup, &mins); err == nil {
			if mins.Valid {
				b.MinutesAgo = int(mins.Int64)
			}
			if b.MinutesAgo < 0 || b.MinutesAgo > 10080 {
				b.Severity = "CRITICAL"
				b.Message = "No log backup in last 7 days"
				b.DrillDown = "Transaction log backups are not running. The log will continue to grow until it consumes all disk space. Risk of data loss since the last successful backup."
				b.MitigationSQL = "-- Verify SQL Agent is running and check job history\nEXEC msdb.dbo.sp_help_job;\n-- Run immediate log backup\nBACKUP LOG [" + b.DatabaseName + "] TO DISK = 'D:\\Backup\\" + b.DatabaseName + "_log.bak';"
			} else if b.MinutesAgo > 30 {
				b.Severity = "CRITICAL"
				b.Message = fmt.Sprintf("Last log backup: %d min ago (CRITICAL)", b.MinutesAgo)
				b.DrillDown = "Log backup overdue. Transaction log cannot truncate, risking disk space exhaustion."
			} else if b.MinutesAgo > 15 {
				b.Severity = "WARNING"
				b.Message = fmt.Sprintf("Last log backup: %d min ago", b.MinutesAgo)
				b.DrillDown = "Log backup not recent. Monitor and ensure backup jobs are running."
			} else {
				b.Severity = "GREEN"
				b.Message = "Log backup OK"
			}
			backups = append(backups, b)
		}
	}
	return backups
}

func (c *MssqlRepository) queryLongRunningTransactions(db *sql.DB) []models.LongTxnInfo {
	query := `
		SELECT TOP 20 r.session_id, s.login_name, DB_NAME(r.database_id), r.status, r.cpu_time,
		       r.total_elapsed_time / 1000, r.logical_reads, r.writes, r.blocking_session_id
		FROM sys.dm_exec_requests r WITH (NOLOCK)
		JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
		WHERE r.database_id > 4 AND r.total_elapsed_time / 1000 > 300
		ORDER BY r.total_elapsed_time DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var txns []models.LongTxnInfo
	for rows.Next() {
		var t models.LongTxnInfo
		if err := rows.Scan(&t.SessionID, &t.LoginName, &t.DatabaseName, &t.Status, &t.CPUTime, &t.ElapsedSeconds, &t.LogicalReads, &t.Writes, &t.BlockingSessionID); err == nil {
			// Check for orphaned transaction (sleeping > 15 min)
			t.IsOrphaned = t.ElapsedSeconds > 900 && t.Status == "sleeping"

			if t.IsOrphaned {
				t.Severity = "CRITICAL"
				t.Message = fmt.Sprintf("ORPHANED transaction - sleeping for %d seconds", t.ElapsedSeconds)
				t.DrillDown = "An open transaction has been idle for over 15 minutes. This prevents transaction log truncation and holds locks on tables, potentially blocking other users."
				t.MitigationSQL = "-- Identify the offending SQL text\nSELECT st.text FROM sys.dm_exec_connections c \nCROSS APPLY sys.dm_exec_sql_text(c.most_recent_sql_handle) AS st WHERE c.session_id = " + fmt.Sprintf("%d", t.SessionID) + ";\n-- Terminate if necessary\nKILL " + fmt.Sprintf("%d", t.SessionID) + ";"
			} else if t.ElapsedSeconds > 900 {
				t.Severity = "CRITICAL"
				t.Message = fmt.Sprintf("Running for %d seconds", t.ElapsedSeconds)
				t.DrillDown = "Transaction running for over 15 minutes. May indicate a long-running report or ad-hoc query."
			} else if t.ElapsedSeconds > 300 {
				t.Severity = "WARNING"
				t.Message = fmt.Sprintf("Running for %d seconds", t.ElapsedSeconds)
				t.DrillDown = "Transaction running for over 5 minutes. Monitor for blocking issues."
			} else {
				t.Severity = "GREEN"
				t.Message = fmt.Sprintf("Running for %d seconds", t.ElapsedSeconds)
			}
			txns = append(txns, t)
		}
	}
	return txns
}

func (c *MssqlRepository) queryAutogrowth(db *sql.DB) []models.AutogrowthInfo {
	query := `
		SELECT DB_NAME(mf.database_id), mf.type_desc, mf.name, mf.is_percent_growth, mf.growth
		FROM sys.master_files mf WITH (NOLOCK)
		WHERE mf.database_id > 4 AND mf.growth > 0
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ags []models.AutogrowthInfo
	for rows.Next() {
		var a models.AutogrowthInfo
		var isPct, growth sql.NullInt64
		if err := rows.Scan(&a.DatabaseName, &a.FileType, &a.LogicalName, &isPct, &growth); err == nil {
			if isPct.Valid {
				a.IsPercentGrowth = isPct.Int64 == 1
			}
			if growth.Valid {
				a.Growth = int(growth.Int64)
			}
			if a.IsPercentGrowth {
				a.Severity = "CRITICAL"
				a.Message = fmt.Sprintf("Percentage growth: %d%%", a.Growth)
				a.DrillDown = "Percentage-based growth becomes dangerously large as the file grows (e.g., 10% of a 1TB file is 100GB)."
				a.MitigationSQL = "ALTER DATABASE [" + a.DatabaseName + "] MODIFY FILE (NAME = N'" + a.LogicalName + "', FILEGROWTH = 256MB);"
			} else if a.Growth < 65536 {
				a.Severity = "WARNING"
				a.Message = fmt.Sprintf("Small growth: %d pages", a.Growth)
				a.DrillDown = "Tiny growth increments cause extreme file fragmentation and frequent autogrowth events."
				a.MitigationSQL = "ALTER DATABASE [" + a.DatabaseName + "] MODIFY FILE (NAME = N'" + a.LogicalName + "', FILEGROWTH = 256MB);"
			} else {
				a.Severity = "GREEN"
				a.Message = "Growth OK"
			}
			ags = append(ags, a)
		}
	}
	return ags
}

func (c *MssqlRepository) queryTempDBConfig(db *sql.DB) models.TempDBInfo {
	var td models.TempDBInfo

	query := `SELECT COUNT(*), SUM(size * 8 / 1024) FROM sys.master_files WITH (NOLOCK) WHERE database_id = 2 AND type_desc = 'DATA'`
	var fc, ts sql.NullInt64
	if err := db.QueryRow(query).Scan(&fc, &ts); err == nil {
		if fc.Valid {
			td.FileCount = int(fc.Int64)
		}
		if ts.Valid {
			td.TotalSizeMB = int(ts.Int64)
		}
	}

	fileQuery := `SELECT name, size * 8 / 1024 FROM sys.master_files WITH (NOLOCK) WHERE database_id = 2 AND type_desc = 'DATA'`
	rows, _ := db.Query(fileQuery)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var f models.TempDBFile
			var sz sql.NullInt64
			if err := rows.Scan(&f.LogicalName, &sz); err == nil && sz.Valid {
				f.SizeMB = int(sz.Int64)
				td.Files = append(td.Files, f)
			}
		}
	}

	// Check if files are equal size
	td.Severity = "GREEN"
	if td.FileCount < 4 {
		td.Severity = "WARNING"
		td.Message = fmt.Sprintf("Only %d files - recommend 4+", td.FileCount)
		td.DrillDown = "TempDB uses proportional fill. Having at least 4 files helps reduce contention."
		td.MitigationSQL = "-- Add more tempdb data files\nALTER DATABASE tempdb ADD FILE (NAME = N'tempdev3', FILENAME = 'D:\\Data\\tempdev3.ndf', SIZE = 1024MB, FILEGROWTH = 256MB);"
	} else if td.FileCount > 8 {
		td.Severity = "WARNING"
		td.Message = fmt.Sprintf("Too many files: %d", td.FileCount)
	} else {
		td.Message = "File count OK"
	}

	// Check for unequal file sizes
	if len(td.Files) > 1 {
		firstSize := td.Files[0].SizeMB
		allEqual := true
		for _, f := range td.Files {
			if f.SizeMB != firstSize {
				allEqual = false
				break
			}
		}
		if !allEqual {
			td.Severity = "CRITICAL"
			td.Message = "Files are not equal size"
			td.DrillDown = "TempDB utilizes a proportional fill algorithm. If files are not equal in size, SQL Server will favor one file, creating a metadata bottleneck (PAGELATCH contention)."
			td.MitigationSQL = "-- Ensure all files have the same size and growth\nALTER DATABASE [tempdb] MODIFY FILE (NAME = N'tempdev', SIZE = 1024MB, FILEGROWTH = 256MB);\nALTER DATABASE [tempdb] MODIFY FILE (NAME = N'tempdev2', SIZE = 1024MB, FILEGROWTH = 256MB);"
		}
	}
	return td
}

func (c *MssqlRepository) queryResourceGovernor(db *sql.DB) models.ResourceGovInfo {
	var rg models.ResourceGovInfo
	query := `SELECT is_enabled FROM sys.resource_governor`
	if err := db.QueryRow(query).Scan(&rg.IsEnabled); err != nil {
		rg.Severity = "UNKNOWN"
		rg.Message = "Could not query RG"
		return rg
	}
	if !rg.IsEnabled {
		rg.Severity = "CRITICAL"
		rg.Message = "RG is disabled"
		rg.DrillDown = "Workload isolation is disabled. A single ad-hoc query or report can consume 100% of CPU and Memory, crashing production application performance."
		rg.MitigationSQL = "-- Enable Resource Governor\nALTER RESOURCE GOVERNOR RECONFIGURE;"
	} else {
		rg.Severity = "GREEN"
		rg.Message = "RG is enabled"
	}
	return rg
}

func (c *MssqlRepository) calculateHealthScore(result models.GuardrailsResult) (int, string) {
	score := 100

	for _, r := range result.StorageRisks {
		if r.Severity == "CRITICAL" {
			score -= 15
		}
	}
	for _, d := range result.DiskSpace {
		if d.Severity == "CRITICAL" {
			score -= 15
		} else if d.Severity == "WARNING" {
			score -= 5
		}
	}
	for _, l := range result.LogHealth {
		if l.Severity == "CRITICAL" {
			score -= 10
		} else if l.Severity == "WARNING" {
			score -= 5
		}
	}
	for _, b := range result.LogBackups {
		if b.Severity == "CRITICAL" {
			score -= 10
		} else if b.Severity == "WARNING" {
			score -= 5
		}
	}
	for _, t := range result.LongTxns {
		if t.Severity == "CRITICAL" {
			score -= 10
		} else if t.Severity == "WARNING" {
			score -= 5
		}
	}
	for _, a := range result.Autogrowth {
		if a.Severity == "CRITICAL" {
			score -= 10
		} else if a.Severity == "WARNING" {
			score -= 5
		}
	}
	if result.TempDBConfig.Severity == "CRITICAL" {
		score -= 10
	} else if result.TempDBConfig.Severity == "WARNING" {
		score -= 5
	}
	if result.ResourceGov.Severity == "WARNING" {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	status := "GREEN"
	if score < 50 {
		status = "CRITICAL"
	} else if score < 75 {
		status = "WARNING"
	}
	return score, status
}

func (c *MssqlRepository) generateRiskSummary(result models.GuardrailsResult) []models.RiskSummary {
	var summary []models.RiskSummary

	addSummary := func(cat string, crit, warn int) {
		sev := "GREEN"
		if crit > 0 {
			sev = "CRITICAL"
		} else if warn > 0 {
			sev = "WARNING"
		}
		summary = append(summary, models.RiskSummary{Category: cat, Count: crit + warn, Critical: crit, Warning: warn, Severity: sev})
	}

	addSummary("Storage & Files", len(result.StorageRisks), 0)

	countSev := func(items []string) (int, int) {
		crit, warn := 0, 0
		for _, s := range items {
			if s == "CRITICAL" {
				crit++
			} else if s == "WARNING" {
				warn++
			}
		}
		return crit, warn
	}

	getDiskSev := func(d []models.DiskSpaceInfo) []string {
		s := make([]string, len(d))
		for i, x := range d {
			s[i] = x.Severity
		}
		return s
	}
	crit, warn := countSev(getDiskSev(result.DiskSpace))
	addSummary("Disk Space", crit, warn)

	getLogHealthSev := func(l []models.LogHealthInfo) []string {
		s := make([]string, len(l))
		for i, x := range l {
			s[i] = x.Severity
		}
		return s
	}
	crit, warn = countSev(getLogHealthSev(result.LogHealth))
	addSummary("Transaction Logs", crit, warn)

	getLogBackSev := func(b []models.LogBackupInfo) []string {
		s := make([]string, len(b))
		for i, x := range b {
			s[i] = x.Severity
		}
		return s
	}
	crit, warn = countSev(getLogBackSev(result.LogBackups))
	addSummary("Log Backups", crit, warn)

	getLongTxnSev := func(t []models.LongTxnInfo) []string {
		s := make([]string, len(t))
		for i, x := range t {
			s[i] = x.Severity
		}
		return s
	}
	crit, warn = countSev(getLongTxnSev(result.LongTxns))
	addSummary("Long Transactions", crit, warn)

	getAutoSev := func(a []models.AutogrowthInfo) []string {
		s := make([]string, len(a))
		for i, x := range a {
			s[i] = x.Severity
		}
		return s
	}
	crit, warn = countSev(getAutoSev(result.Autogrowth))
	addSummary("Autogrowth", crit, warn)

	if result.TempDBConfig.Severity != "GREEN" {
		addSummary("TempDB", 0, 1)
	}
	if result.ResourceGov.Severity != "GREEN" {
		addSummary("Resource Governor", 0, 1)
	}

	return summary
}
