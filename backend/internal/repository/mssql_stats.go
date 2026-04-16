// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: General SQL Server statistics and performance counters.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/sqlserver"
)

type MssqlRepository struct {
	conns          map[string]*sql.DB
	status         map[string]string // "online", "offline", "error"
	mutex          sync.RWMutex
	prevQueryCache map[string]map[string]QueryState // InstanceName -> QueryHash -> QueryState
}

type QueryState struct {
	Executions int64
	CPUMs      float64
}

// HasConnection returns true if the instance has an active connection in the pool.
func (c *MssqlRepository) HasConnection(instanceName string) bool {
	c.mutex.RLock()
	_, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	return ok
}

// ListSQLServerUserDatabases returns ONLINE databases with database_id > 4 that the login can access.
// Used when Instances[].databases is empty so Storage & Index Health collection still runs.
func (c *MssqlRepository) ListSQLServerUserDatabases(instanceName string) ([]string, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	const q = `
		SELECT d.name
		FROM sys.databases d
		WHERE d.database_id > 4
		  AND d.state = 0
		  AND LOWER(d.name) <> N'distribution'
		ORDER BY d.name
	`
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			continue
		}
		n = strings.TrimSpace(n)
		if n != "" {
			names = append(names, n)
		}
	}
	return names, rows.Err()
}

func NewMssqlRepository(cfg *config.Config) *MssqlRepository {
	c := &MssqlRepository{
		conns:  make(map[string]*sql.DB),
		status: make(map[string]string),
	}

	for i, inst := range cfg.Instances {
		if inst.Type == "sqlserver" {
			port := inst.Port
			if port == 0 {
				port = 1433
			}

			user := inst.User
			password := inst.Password

			// Environment Variable injections targeting Docker/Kubernetes secrets mapping
			envPrefix := fmt.Sprintf("DB_%s", strings.ToUpper(strings.ReplaceAll(inst.Name, "-", "_")))
			if user == "" && !inst.IntegratedSecurity {
				user = os.Getenv(envPrefix + "_USER")
			}
			if password == "" && !inst.IntegratedSecurity {
				password = os.Getenv(envPrefix + "_PASSWORD")
			}

			catalog := strings.TrimSpace(inst.Database)
			if catalog == "" {
				catalog = "master"
			}
			// Construct DSN (encrypt=true for Azure SQL / MI / cloud endpoints)
			var connStr string
			if inst.IntegratedSecurity {
				// Windows Authentication (Passwordless / Active Directory)
				connStr = fmt.Sprintf("server=%s;port=%d;database=%s;Integrated Security=true;encrypt=true;", inst.Host, port, catalog)
			} else {
				// SQL Authentication natively
				connStr = fmt.Sprintf("server=%s;port=%d;database=%s;user id=%s;password=%s;encrypt=true;", inst.Host, port, catalog, user, password)
			}

			if inst.TrustServerCertificate {
				connStr += "TrustServerCertificate=true;"
			} else {
				connStr += "TrustServerCertificate=false;"
			}

			db, err := sqlserver.OpenMetricsPool(connStr)
			if err != nil {
				c.status[inst.Name] = "offline"
				log.Printf("[MSSQL] DSN Parse Error %s: %v", inst.Name, err)
				continue
			}

			db.SetMaxOpenConns(5)
			db.SetMaxIdleConns(2)
			db.SetConnMaxLifetime(time.Minute * 10)

			if err := db.Ping(); err != nil {
				c.status[inst.Name] = "offline"
				log.Printf("[MSSQL] Connection ping failure %s: %v", inst.Name, err)
			} else {
				c.status[inst.Name] = "online"
			}

			c.conns[inst.Name] = db

			// Auto-Discover Database Arrays if explicitly omitted from config.yaml dynamically
			if len(inst.Databases) == 0 {
				query := "SELECT name FROM sys.databases WHERE database_id > 4 AND state_desc = 'ONLINE'"
				rows, err := db.Query(query)
				if err == nil {
					var discoverDbs []string
					for rows.Next() {
						var dbName string
						rows.Scan(&dbName)
						discoverDbs = append(discoverDbs, dbName)
					}
					rows.Close()
					cfg.Instances[i].Databases = discoverDbs
				} else {
					log.Printf("[MSSQL] Dynamic Database Binding failure %s: %v", inst.Name, err)
				}
			}
		}
	}
	return c
}

func (c *MssqlRepository) PingAll() {
	var wg sync.WaitGroup
	for name, db := range c.conns {
		wg.Add(1)
		go func(n string, connection *sql.DB) {
			defer wg.Done()
			err := connection.Ping()
			c.mutex.Lock()
			if err != nil {
				c.status[n] = "offline"
				log.Printf("[MSSQL] Handshake warning to %s: %v", n, err)
			} else {
				c.status[n] = "online"
				log.Printf("[MSSQL] Success with %s", n)
			}
			c.mutex.Unlock()
		}(name, db)
	}
	wg.Wait()
}

// GetConn returns a live SQL Server connection for an instance name.
func (c *MssqlRepository) GetConn(instanceName string) (*sql.DB, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	db, ok := c.conns[instanceName]
	return db, ok
}

func (c *MssqlRepository) GetInstanceStatus(instanceName string) string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	if status, ok := c.status[instanceName]; ok {
		return status
	}
	return "unknown"
}

func (c *MssqlRepository) GetAllInstanceStatuses() map[string]string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	statuses := make(map[string]string, len(c.status))
	for name, status := range c.status {
		statuses[name] = status
	}
	return statuses
}

func (c *MssqlRepository) UpdateInstanceStatus(instanceName string) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	c.mutex.Lock()
	defer c.mutex.Unlock()
	if !ok || db == nil {
		c.status[instanceName] = "offline"
		return
	}

	if err := db.Ping(); err != nil {
		c.status[instanceName] = "offline"
	} else {
		c.status[instanceName] = "online"
	}
}

// GetGlobalMetric executes explicit raw DMVs mapped to physical host limits resolving CPU / RAM
func (c *MssqlRepository) GetGlobalMetric(name string, base models.GlobalInstanceMetric) models.GlobalInstanceMetric {
	c.mutex.RLock()
	db, ok := c.conns[name]
	c.mutex.RUnlock()

	if !ok || db == nil {
		base.Status = 2
		base.Error = "Connection Context Lost"
		return base
	}

	if err := db.Ping(); err != nil {
		base.Status = 2
		base.Error = err.Error()
		return base
	}

	base.Status = 0

	// 1. Fetch Physical CPU Utilization safely via ring buffers natively parsed into XML
	cpuQuery := `
		SELECT TOP 1 
			record.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int') AS [SQLProcessUtilization]
		FROM (
			SELECT [timestamp], CONVERT(xml, record) AS [record]
			FROM sys.dm_os_ring_buffers
			WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR'
			AND record LIKE '%<SystemHealth>%'
		) AS x ORDER BY [timestamp] DESC
	`
	var cpu int
	if err := db.QueryRow(cpuQuery).Scan(&cpu); err == nil {
		base.CPUUsage = float64(cpu)
	}

	// 2. Note: Cannot calculate real memory percentage without total system memory
	// For now, return 0 to avoid dummy values
	base.MemoryPct = 0

	return base
}

// QueryStoreStats represents a row from SQL Server Query Store
type QueryStoreStats struct {
	DatabaseName    string
	QueryHash       string
	QueryText       string
	Executions      int64
	AvgDurationMs   float64
	AvgCpuMs        float64
	AvgLogicalReads float64
	TotalCpuMs      float64
}

type LongRunningQueryStats struct {
	SessionID            int
	RequestID            int
	DatabaseName         string
	LoginName            string
	HostName             string
	ProgramName          string
	QueryHash            string
	QueryText            string
	WaitType             string
	BlockingSessionID    int
	Status               string
	CPUTimeMs            int64
	TotalElapsedTimeMs   int64
	Reads                int64
	Writes               int64
	GrantedQueryMemoryMB int
	RowCount             int64
}

func (c *MssqlRepository) FetchLongRunningQueries(instanceName string, minDurationSeconds int) ([]LongRunningQueryStats, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	query := `
		SELECT TOP 50
			r.session_id,
			r.request_id,
			DB_NAME(r.database_id) AS database_name,
			s.login_name,
			s.host_name,
			s.program_name,
			CONVERT(VARCHAR(64), r.sql_handle, 1) AS query_hash,
			CASE 
				WHEN qp.objectid IS NOT NULL 
				THEN QUOTENAME(OBJECT_SCHEMA_NAME(qp.objectid, r.database_id)) 
					 + '.' + QUOTENAME(OBJECT_NAME(qp.objectid, r.database_id))
				ELSE
					SUBSTRING(
						qt.text,
						(r.statement_start_offset/2) + 1,
						(
							CASE r.statement_end_offset
								WHEN -1 THEN DATALENGTH(qt.text)
								ELSE r.statement_end_offset
							END - r.statement_start_offset
						) / 2 + 1
					)
			END AS query_text,
			r.wait_type,
			r.blocking_session_id,
			r.status,
			r.cpu_time AS cpu_time_ms,
			r.total_elapsed_time AS total_elapsed_time_ms,
			r.reads,
			r.writes,
			(r.granted_query_memory * 8) / 1024 AS granted_query_memory_mb,
			r.row_count
		FROM sys.dm_exec_requests r
		JOIN sys.dm_exec_sessions s 
			ON r.session_id = s.session_id
		CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) qt
		OUTER APPLY sys.dm_exec_query_plan(r.plan_handle) qp
		WHERE r.session_id <> @@SPID AND r.session_id > 50
		AND r.total_elapsed_time >= 5000
		AND s.is_user_process = 1
		AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		ORDER BY r.total_elapsed_time DESC`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[MSSQL] FetchLongRunningQueries Error for %s: %v", instanceName, err)
		return nil, err
	}
	defer rows.Close()

	var results []LongRunningQueryStats
	for rows.Next() {
		var s LongRunningQueryStats
		var qhash sql.NullString
		var waitType sql.NullString
		var cpuTime, totalElapsed, reads, writes, grantedMemory, rowCount sql.NullInt64
		var blockingSessionID sql.NullInt64

		if err := rows.Scan(
			&s.SessionID, &s.RequestID, &s.DatabaseName, &s.LoginName,
			&s.HostName, &s.ProgramName, &qhash, &s.QueryText, &waitType,
			&blockingSessionID, &s.Status, &cpuTime, &totalElapsed,
			&reads, &writes, &grantedMemory, &rowCount,
		); err != nil {
			log.Printf("[MSSQL] FetchLongRunningQueries Scan Error: %v", err)
			continue
		}

		if waitType.Valid {
			s.WaitType = waitType.String
		}
		if qhash.Valid {
			s.QueryHash = qhash.String
		}
		if cpuTime.Valid {
			s.CPUTimeMs = cpuTime.Int64
		}
		if totalElapsed.Valid {
			s.TotalElapsedTimeMs = totalElapsed.Int64
		}
		if reads.Valid {
			s.Reads = reads.Int64
		}
		if writes.Valid {
			s.Writes = writes.Int64
		}
		if blockingSessionID.Valid {
			s.BlockingSessionID = int(blockingSessionID.Int64)
		}
		if grantedMemory.Valid {
			s.GrantedQueryMemoryMB = int(grantedMemory.Int64)
		}
		if rowCount.Valid {
			s.RowCount = rowCount.Int64
		}

		results = append(results, s)
	}

	return results, rows.Err()
}

func sqlServerQuoteBracket(ident string) string {
	if ident == "" {
		return "[]"
	}
	return "[" + strings.ReplaceAll(ident, "]", "]]") + "]"
}

// FetchQueryStoreSQLText returns the full Query Store SQL text (nvarchar(max)) for a query hash.
// This is used for UI drill-down so list payloads can keep shorter snippets.
func (c *MssqlRepository) FetchQueryStoreSQLText(instanceName, databaseName, queryHash string) (string, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return "", fmt.Errorf("no connection for instance: %s", instanceName)
	}
	databaseName = strings.TrimSpace(databaseName)
	if databaseName == "" {
		return "", fmt.Errorf("database is required")
	}
	queryHash = strings.TrimSpace(queryHash)
	if queryHash == "" {
		return "", fmt.Errorf("query_hash is required")
	}

	dbPrefix := sqlServerQuoteBracket(databaseName)
	q := fmt.Sprintf(`
		SELECT TOP 1
			qt.query_sql_text
		FROM %s.sys.query_store_query q
		INNER JOIN %s.sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
		INNER JOIN %s.sys.query_store_plan p ON q.query_id = p.query_id
		INNER JOIN %s.sys.query_store_runtime_stats rs ON p.plan_id = rs.plan_id
		WHERE q.is_internal_query = 0
		  AND CONVERT(VARCHAR(40), q.query_hash) = @p1
		ORDER BY rs.last_execution_time DESC;
	`, dbPrefix, dbPrefix, dbPrefix, dbPrefix)

	var sqlText sql.NullString
	if err := db.QueryRow(q, queryHash).Scan(&sqlText); err != nil {
		return "", err
	}
	if !sqlText.Valid {
		return "", nil
	}
	return sqlText.String, nil
}

// queryStoreStatsSelectSQL returns a Query Store aggregate query scoped to a database.
// dbPrefix must be a bracket-quoted database name (e.g. [MyDb]) so we avoid USE on a pooled
// connection (which would race other collectors on the same *sql.DB).
func queryStoreStatsSelectSQL(dbPrefix string) string {
	return fmt.Sprintf(`
		SELECT TOP 50
			CONVERT(VARCHAR(40), q.query_hash) AS query_hash,
			LEFT(qt.query_sql_text, 500) AS query_text,
			ISNULL(rs.count_executions, 0) AS executions,
			ISNULL(rs.avg_duration, 0) / 1000.0 AS avg_duration_ms,
			ISNULL(rs.avg_cpu_time, 0) / 1000.0 AS avg_cpu_ms,
			ISNULL(rs.avg_logical_io_reads, 0) AS avg_logical_reads,
			(ISNULL(rs.avg_cpu_time, 0) * ISNULL(rs.count_executions, 1)) / 1000.0 AS total_cpu_ms
		FROM %s.sys.query_store_query q
		INNER JOIN %s.sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
		INNER JOIN %s.sys.query_store_plan p ON q.query_id = p.query_id
		INNER JOIN %s.sys.query_store_runtime_stats rs ON p.plan_id = rs.plan_id
		LEFT JOIN %s.sys.query_store_runtime_stats_interval rsi ON rs.runtime_stats_interval_id = rsi.runtime_stats_interval_id
		WHERE q.is_internal_query = 0
		  AND ISNULL(rs.count_executions, 0) > 0
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%%sql_optima%%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%%sqloptima%%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%%sys.dm_%%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%%sys.query_store_%%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%%sp_server_diagnostics%%'
		  AND (
			rs.last_execution_time >= DATEADD(day, -7, SYSDATETIMEOFFSET())
			OR rsi.end_time >= DATEADD(day, -7, GETDATE())
		  )
		ORDER BY (ISNULL(rs.avg_cpu_time, 0) * ISNULL(rs.count_executions, 0)) DESC
	`, dbPrefix, dbPrefix, dbPrefix, dbPrefix, dbPrefix)
}

// FetchQueryStoreStats fetches aggregated Query Store statistics from each user database
// where Query Store is available. Query Store does not record login or application name;
// those exist only on session DMVs (see long-running / top-queries collectors).
func (c *MssqlRepository) FetchQueryStoreStats(instanceName string) ([]QueryStoreStats, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	dbNames, err := c.listUserDatabaseNamesForQueryStore(db)
	if err != nil || len(dbNames) == 0 {
		log.Printf("[MSSQL] FetchQueryStoreStats: no user DBs with Query Store or list error for %s: %v — trying current database only", instanceName, err)
		return c.fetchQueryStoreStatsSingleDB(db, "")
	}

	var merged []QueryStoreStats
	for _, dbn := range dbNames {
		qb := sqlServerQuoteBracket(dbn)
		query := queryStoreStatsSelectSQL(qb)
		rows, err := db.Query(query)
		if err != nil {
			log.Printf("[MSSQL] FetchQueryStoreStats query in %s: %v", dbn, err)
			continue
		}
		for rows.Next() {
			var qs QueryStoreStats
			qs.DatabaseName = dbn
			var queryText sql.NullString
			if err := rows.Scan(&qs.QueryHash, &queryText, &qs.Executions,
				&qs.AvgDurationMs, &qs.AvgCpuMs, &qs.AvgLogicalReads, &qs.TotalCpuMs); err != nil {
				log.Printf("[MSSQL] FetchQueryStoreStats Scan Error: %v", err)
				continue
			}
			if queryText.Valid {
				qs.QueryText = queryText.String
			} else {
				qs.QueryText = "N/A"
			}
			merged = append(merged, qs)
		}
		rows.Close()
	}

	if len(merged) == 0 {
		return c.fetchQueryStoreStatsSingleDB(db, "")
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].TotalCpuMs > merged[j].TotalCpuMs
	})
	if len(merged) > 200 {
		merged = merged[:200]
	}
	return merged, nil
}

func (c *MssqlRepository) listUserDatabaseNamesForQueryStore(db *sql.DB) ([]string, error) {
	q := `
		SELECT d.name
		FROM sys.databases d
		WHERE d.database_id > 4
		  AND d.state = 0
		  AND d.is_query_store_on = 1
		ORDER BY d.name`
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			continue
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

func (c *MssqlRepository) fetchQueryStoreStatsSingleDB(db *sql.DB, labelDB string) ([]QueryStoreStats, error) {
	query := `
		SELECT TOP 50
			CONVERT(VARCHAR(40), q.query_hash) AS query_hash,
			LEFT(qt.query_sql_text, 500) AS query_text,
			ISNULL(rs.count_executions, 0) AS executions,
			ISNULL(rs.avg_duration, 0) / 1000.0 AS avg_duration_ms,
			ISNULL(rs.avg_cpu_time, 0) / 1000.0 AS avg_cpu_ms,
			ISNULL(rs.avg_logical_io_reads, 0) AS avg_logical_reads,
			(ISNULL(rs.avg_cpu_time, 0) * ISNULL(rs.count_executions, 1)) / 1000.0 AS total_cpu_ms
		FROM sys.query_store_query q
		INNER JOIN sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
		INNER JOIN sys.query_store_plan p ON q.query_id = p.query_id
		INNER JOIN sys.query_store_runtime_stats rs ON p.plan_id = rs.plan_id
		LEFT JOIN sys.query_store_runtime_stats_interval rsi ON rs.runtime_stats_interval_id = rsi.runtime_stats_interval_id
		WHERE q.is_internal_query = 0
		  AND ISNULL(rs.count_executions, 0) > 0
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%sql_optima%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%sqloptima%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%sys.dm_%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%sys.query_store_%'
		  AND LOWER(ISNULL(qt.query_sql_text, '')) NOT LIKE '%sp_server_diagnostics%'
		  AND (
			rs.last_execution_time >= DATEADD(day, -7, SYSDATETIMEOFFSET())
			OR rsi.end_time >= DATEADD(day, -7, GETDATE())
		  )
		ORDER BY (ISNULL(rs.avg_cpu_time, 0) * ISNULL(rs.count_executions, 0)) DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []QueryStoreStats
	for rows.Next() {
		var qs QueryStoreStats
		qs.DatabaseName = labelDB
		if labelDB == "" {
			qs.DatabaseName = "default"
		}
		var queryText sql.NullString
		if err := rows.Scan(&qs.QueryHash, &queryText, &qs.Executions,
			&qs.AvgDurationMs, &qs.AvgCpuMs, &qs.AvgLogicalReads, &qs.TotalCpuMs); err != nil {
			continue
		}
		if queryText.Valid {
			qs.QueryText = queryText.String
		} else {
			qs.QueryText = "N/A"
		}
		results = append(results, qs)
	}
	return results, rows.Err()
}

// AGHealthStats represents AlwaysOn Availability Group health metrics
type AGHealthStats struct {
	AGName               string
	ReplicaServerName    string
	DatabaseName         string
	ReplicaRole          string
	SynchronizationState string
	SyncStateDesc        string
	IsPrimaryReplica     bool
	LogSendQueueKB       int64
	RedoQueueKB          int64
	LogSendRateKB        int64
	RedoRateKB           int64
	LastSentTime         sql.NullTime
	LastReceivedTime     sql.NullTime
	LastHardenedTime     sql.NullTime
	LastRedoneTime       sql.NullTime
	SecondaryLagSecs     int64
}

// FetchAGHealthStats fetches AlwaysOn Availability Group health metrics
func (c *MssqlRepository) FetchAGHealthStats(instanceName string) ([]AGHealthStats, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	checkQuery := `SELECT COUNT(*) FROM sys.dm_hadr_availability_group_states`
	var agCount int
	if err := db.QueryRow(checkQuery).Scan(&agCount); err != nil {
		log.Printf("[MSSQL] FetchAGHealthStats AG not available for %s: %v (not Enterprise Edition or AG not configured)", instanceName, err)
		return []AGHealthStats{}, nil
	}

	if agCount == 0 {
		return []AGHealthStats{}, nil
	}

	// Check if database states table exists (may not exist on all AG configurations)
	hasDbStates := true
	var dbStatesCheck int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sys.dm_hadr_availability_database_states`).Scan(&dbStatesCheck); err != nil {
		log.Printf("[MSSQL] FetchAGHealthStats: dm_hadr_availability_database_states not available for %s: %v", instanceName, err)
		hasDbStates = false
	}

	// Check if secondary_lag_seconds column exists (added in SQL Server 2016 SP1)
	hasSecondaryLag := true
	var lagCheck int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_replica_states') AND name = 'secondary_lag_seconds'`).Scan(&lagCheck); err != nil || lagCheck == 0 {
		log.Printf("[MSSQL] FetchAGHealthStats: secondary_lag_seconds not available for %s (pre-2016 SP1)", instanceName)
		hasSecondaryLag = false
	}

	// Check if last_redone_time column exists (added in SQL Server 2016)
	hasLastRedoneTime := true
	var redoCheck int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_replica_states') AND name = 'last_redone_time'`).Scan(&redoCheck); err != nil || redoCheck == 0 {
		log.Printf("[MSSQL] FetchAGHealthStats: last_redone_time not available for %s (pre-2016)", instanceName)
		hasLastRedoneTime = false
	}

	// Check if last_hardened_time column exists (added in SQL Server 2016 SP1 CU6)
	hasLastHardenedTime := true
	var hardenedCheck int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_replica_states') AND name = 'last_hardened_time'`).Scan(&hardenedCheck); err != nil || hardenedCheck == 0 {
		log.Printf("[MSSQL] FetchAGHealthStats: last_hardened_time not available for %s (pre-2016 SP1 CU6)", instanceName)
		hasLastHardenedTime = false
	}

	// Check if log_send_rate column exists (added in SQL Server 2016)
	hasLogSendRate := true
	var logRateCheck int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_replica_states') AND name = 'log_send_rate'`).Scan(&logRateCheck); err != nil || logRateCheck == 0 {
		log.Printf("[MSSQL] FetchAGHealthStats: log_send_rate not available for %s (pre-2016)", instanceName)
		hasLogSendRate = false
	}

	// Check if undo_rate column exists (added in SQL Server 2016)
	hasUndoRate := true
	var undoRateCheck int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_replica_states') AND name = 'undo_rate'`).Scan(&undoRateCheck); err != nil || undoRateCheck == 0 {
		log.Printf("[MSSQL] FetchAGHealthStats: undo_rate not available for %s (pre-2016)", instanceName)
		hasUndoRate = false
	}

	// Check if undo_queue_size column exists (added in SQL Server 2016)
	hasUndoQueueSize := true
	var undoQueueCheck int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('sys.dm_hadr_availability_replica_states') AND name = 'undo_queue_size'`).Scan(&undoQueueCheck); err != nil || undoQueueCheck == 0 {
		log.Printf("[MSSQL] FetchAGHealthStats: undo_queue_size not available for %s (pre-2016)", instanceName)
		hasUndoQueueSize = false
	}

	// If any column is missing, use the ultra-minimal query with all literals
	var query string
	if !hasSecondaryLag || !hasLastRedoneTime || !hasLastHardenedTime || !hasLogSendRate || !hasUndoRate || !hasUndoQueueSize {
		log.Printf("[MSSQL] FetchAGHealthStats: Using minimal fallback query for %s (missing columns detected)", instanceName)
		query = fmt.Sprintf(`
			SELECT 
				ag.name AS ag_name,
				ar.replica_server_name,
				'N/A' AS database_name,
				'UNKNOWN' AS replica_role,
				0 AS synchronization_state,
				'UNKNOWN' AS synchronization_state_desc,
				0 AS is_primary_replica,
				CAST(0 AS BIGINT) AS log_send_queue_kb,
				CAST(0 AS BIGINT) AS redo_queue_kb,
				CAST(0 AS BIGINT) AS log_send_rate_kb,
				CAST(0 AS BIGINT) AS redo_rate_kb,
				CAST(NULL AS DATETIME) AS last_sent_time,
				CAST(NULL AS DATETIME) AS last_received_time,
				CAST(NULL AS DATETIME) AS last_hardened_time,
				CAST(NULL AS DATETIME) AS last_redone_time,
				CAST(0 AS BIGINT) AS secondary_lag_seconds
			FROM sys.availability_groups ag
			INNER JOIN sys.availability_replicas ar ON ag.group_id = ar.group_id
			ORDER BY ag.name, ar.replica_server_name
		`)
	} else if hasDbStates {
		query = `
			SELECT 
				ag.name AS ag_name,
				ar.replica_server_name,
				COALESCE(DB_NAME(dbs.database_id), 'N/A') AS database_name,
				rs.role_desc AS replica_role,
				COALESCE(drs.synchronization_state, 0) AS synchronization_state,
				COALESCE(drs.synchronization_state_desc, 'UNKNOWN') AS synchronization_state_desc,
				CASE WHEN rs.role_desc = 'PRIMARY' THEN 1 ELSE 0 END AS is_primary_replica,
				ISNULL(drs.log_send_queue_size, 0) / 1024 AS log_send_queue_kb,
				ISNULL(drs.undo_queue_size, 0) / 1024 AS redo_queue_kb,
				ISNULL(drs.log_send_rate, 0) / 1024 AS log_send_rate_kb,
				ISNULL(drs.undo_rate, 0) / 1024 AS redo_rate_kb,
				drs.last_sent_time,
				drs.last_received_time,
				drs.last_hardened_time,
				drs.last_redone_time,
				ISNULL(drs.secondary_lag_seconds, 0) AS secondary_lag_seconds
			FROM sys.availability_groups ag
			INNER JOIN sys.availability_replicas ar ON ag.group_id = ar.group_id
			INNER JOIN sys.dm_hadr_availability_group_states rs ON ag.group_id = rs.group_id
			INNER JOIN sys.dm_hadr_availability_replica_states drs ON ar.replica_id = drs.replica_id
			LEFT JOIN sys.dm_hadr_availability_database_states dbs ON ar.replica_id = dbs.replica_id AND dbs.database_id IS NOT NULL
			ORDER BY ag.name, ar.replica_server_name
		`
	} else {
		query = `
			SELECT 
				ag.name AS ag_name,
				ar.replica_server_name,
				'N/A' AS database_name,
				rs.role_desc AS replica_role,
				COALESCE(drs.synchronization_state, 0) AS synchronization_state,
				COALESCE(drs.synchronization_state_desc, 'UNKNOWN') AS synchronization_state_desc,
				CASE WHEN rs.role_desc = 'PRIMARY' THEN 1 ELSE 0 END AS is_primary_replica,
				ISNULL(drs.log_send_queue_size, 0) / 1024 AS log_send_queue_kb,
				ISNULL(drs.undo_queue_size, 0) / 1024 AS redo_queue_kb,
				ISNULL(drs.log_send_rate, 0) / 1024 AS log_send_rate_kb,
				ISNULL(drs.undo_rate, 0) / 1024 AS redo_rate_kb,
				drs.last_sent_time,
				drs.last_received_time,
				drs.last_hardened_time,
				drs.last_redone_time,
				ISNULL(drs.secondary_lag_seconds, 0) AS secondary_lag_seconds
			FROM sys.availability_groups ag
			INNER JOIN sys.availability_replicas ar ON ag.group_id = ar.group_id
			INNER JOIN sys.dm_hadr_availability_group_states rs ON ag.group_id = rs.group_id
			INNER JOIN sys.dm_hadr_availability_replica_states drs ON ar.replica_id = drs.replica_id
			ORDER BY ag.name, ar.replica_server_name
		`
	}

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[MSSQL] FetchAGHealthStats Error for %s: %v", instanceName, err)
		// If query fails due to missing columns, retry with an ultra-minimal query using only literals
		if strings.Contains(err.Error(), "Invalid column name") {
			log.Printf("[MSSQL] FetchAGHealthStats: Retrying with ultra-minimal fallback query for %s", instanceName)
			query = fmt.Sprintf(`
				SELECT 
					ag.name AS ag_name,
					ar.replica_server_name,
					'N/A' AS database_name,
					'UNKNOWN' AS replica_role,
					0 AS synchronization_state,
					'UNKNOWN' AS synchronization_state_desc,
					0 AS is_primary_replica,
					CAST(0 AS BIGINT) AS log_send_queue_kb,
					CAST(0 AS BIGINT) AS redo_queue_kb,
					CAST(0 AS BIGINT) AS log_send_rate_kb,
					CAST(0 AS BIGINT) AS redo_rate_kb,
					CAST(NULL AS DATETIME) AS last_sent_time,
					CAST(NULL AS DATETIME) AS last_received_time,
					CAST(NULL AS DATETIME) AS last_hardened_time,
					CAST(NULL AS DATETIME) AS last_redone_time,
					CAST(0 AS BIGINT) AS secondary_lag_seconds
				FROM sys.availability_groups ag
				INNER JOIN sys.availability_replicas ar ON ag.group_id = ar.group_id
				ORDER BY ag.name, ar.replica_server_name
			`)
			rows, err = db.Query(query)
			if err != nil {
				log.Printf("[MSSQL] FetchAGHealthStats Fallback Error for %s: %v", instanceName, err)
				// Last resort: return empty results instead of erroring out the collector
				return []AGHealthStats{}, nil
			}
		} else {
			return nil, err
		}
	}
	defer rows.Close()

	var results []AGHealthStats
	for rows.Next() {
		var s AGHealthStats
		var dbName sql.NullString
		var roleDesc, syncState, syncStateDesc sql.NullString
		var isPrimary int
		var logSendQueue, redoQueue, logSendRate, redoRate sql.NullInt64
		var secondaryLag int64

		if err := rows.Scan(&s.AGName, &s.ReplicaServerName, &dbName, &roleDesc, &syncState, &syncStateDesc,
			&isPrimary, &logSendQueue, &redoQueue, &logSendRate, &redoRate,
			&s.LastSentTime, &s.LastReceivedTime, &s.LastHardenedTime, &s.LastRedoneTime, &secondaryLag); err != nil {
			log.Printf("[MSSQL] FetchAGHealthStats Scan Error: %v", err)
			continue
		}

		if dbName.Valid {
			s.DatabaseName = dbName.String
		}
		if roleDesc.Valid {
			s.ReplicaRole = roleDesc.String
		}
		if syncState.Valid {
			s.SynchronizationState = syncState.String
		}
		if syncStateDesc.Valid {
			s.SyncStateDesc = syncStateDesc.String
		}
		s.IsPrimaryReplica = isPrimary == 1
		if logSendQueue.Valid {
			s.LogSendQueueKB = logSendQueue.Int64
		}
		if redoQueue.Valid {
			s.RedoQueueKB = redoQueue.Int64
		}
		if logSendRate.Valid {
			s.LogSendRateKB = logSendRate.Int64
		}
		if redoRate.Valid {
			s.RedoRateKB = redoRate.Int64
		}
		s.SecondaryLagSecs = secondaryLag

		results = append(results, s)
	}

	return results, rows.Err()
}

// DatabaseThroughputStats represents database-level throughput metrics
type DatabaseThroughputStats struct {
	DatabaseName        string
	UserSeeks           int64
	UserScans           int64
	UserLookups         int64
	UserWrites          int64
	TotalReads          int64
	TotalWrites         int64
	TPS                 float64
	BatchRequestsPerSec float64
}

// FetchDatabaseThroughput fetches per-database throughput statistics
func (c *MssqlRepository) FetchDatabaseThroughput(instanceName string) ([]DatabaseThroughputStats, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	query := `
		SELECT 
			DB_NAME(s.database_id) AS database_name,
			ISNULL(SUM(s.user_seeks), 0) AS idx_seeks,
			ISNULL(SUM(s.user_scans), 0) AS idx_scans,
			ISNULL(SUM(s.user_lookups), 0) AS idx_lookups,
			ISNULL(SUM(s.user_updates), 0) AS idx_updates,
			ISNULL(SUM(s.user_seeks + s.user_scans + s.user_lookups), 0) AS total_idx_reads,
			ISNULL(SUM(s.user_seeks + s.user_scans + s.user_lookups + s.user_updates), 0) AS total_idx_activity
		FROM sys.dm_db_index_usage_stats s
		WHERE s.database_id > 4
		GROUP BY s.database_id
		HAVING ISNULL(SUM(s.user_seeks + s.user_scans + s.user_lookups + s.user_updates), 0) > 0
		ORDER BY total_idx_activity DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[MSSQL] FetchDatabaseThroughput Error for %s: %v", instanceName, err)
		return nil, err
	}
	defer rows.Close()

	var results []DatabaseThroughputStats
	for rows.Next() {
		var s DatabaseThroughputStats
		var idxSeeks, idxScans, idxLookups, idxUpdates, totalReads, totalActivity int64
		if err := rows.Scan(&s.DatabaseName, &idxSeeks, &idxScans, &idxLookups, &idxUpdates, &totalReads, &totalActivity); err != nil {
			log.Printf("[MSSQL] FetchDatabaseThroughput Scan Error: %v", err)
			continue
		}
		s.UserSeeks = idxSeeks
		s.UserScans = idxScans
		s.UserLookups = idxLookups
		s.UserWrites = idxUpdates
		s.TotalReads = totalReads
		s.TotalWrites = idxUpdates
		s.TPS = float64(totalActivity) / 60.0
		results = append(results, s)
	}

	// Also get batch requests per database
	batchQuery := `
		SELECT 
			DB_NAME(r.database_id) AS database_name,
			COUNT(*) AS batch_count
		FROM sys.dm_exec_requests r
		WHERE r.database_id > 4
		GROUP BY r.database_id
	`

	batchRows, batchErr := db.Query(batchQuery)
	if batchErr == nil {
		defer batchRows.Close()
		batchMap := make(map[string]float64)
		for batchRows.Next() {
			var dbName string
			var bps float64
			if err := batchRows.Scan(&dbName, &bps); err == nil {
				batchMap[dbName] = bps
			}
		}
		// Merge batch data into results
		for i := range results {
			if bps, ok := batchMap[results[i].DatabaseName]; ok {
				results[i].BatchRequestsPerSec = bps
			}
		}
	}

	return results, rows.Err()
}

// FetchLatchStats returns latch wait statistics from sys.dm_os_latch_stats.
func (c *MssqlRepository) FetchLatchStats(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectLatchStats(db)
}

// FetchWaitingTasks returns currently waiting tasks from sys.dm_os_waiting_tasks.
func (c *MssqlRepository) FetchWaitingTasks(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectWaitingTasks(db)
}

// FetchMemoryGrants returns active memory grants from sys.dm_exec_query_memory_grants.
func (c *MssqlRepository) FetchMemoryGrants(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectMemoryGrants(db)
}

// FetchSchedulerWG returns scheduler and workload group stats.
func (c *MssqlRepository) FetchSchedulerWG(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// Resource Governor workload group stats. This works when Resource Governor DMVs exist
	// (Enterprise, Developer, and some editions depending on features). If unavailable,
	// callers should handle empty results gracefully.
	query := `
		SELECT
			COALESCE(rp.name, 'default') AS pool_name,
			COALESCE(wg.name, 'default') AS group_name,
			COALESCE(wgs.active_request_count, 0) AS active_requests,
			COALESCE(wgs.queued_request_count, 0) AS queued_requests,
			COALESCE(wgs.total_cpu_usage_ms * 100.0 / NULLIF(rgs.total_cpu_usage_ms, 0), 0) AS cpu_usage_percent
		FROM sys.dm_resource_governor_workload_groups wg
		LEFT JOIN sys.resource_governor_workload_groups wgm ON wg.group_id = wgm.group_id
		LEFT JOIN sys.resource_governor_resource_pools rp ON wgm.pool_id = rp.pool_id
		LEFT JOIN sys.dm_resource_governor_workload_groups_stats wgs ON wg.group_id = wgs.group_id
		CROSS JOIN (SELECT SUM(total_cpu_usage_ms) AS total_cpu_usage_ms FROM sys.dm_resource_governor_workload_groups_stats) rgs
		ORDER BY cpu_usage_percent DESC, pool_name, group_name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []map[string]interface{}
	for rows.Next() {
		var poolName, groupName string
		var active, queued int64
		var cpuPct float64
		if err := rows.Scan(&poolName, &groupName, &active, &queued, &cpuPct); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"pool_name":        poolName,
			"group_name":       groupName,
			"active_requests":  active,
			"queued_requests":  queued,
			"cpu_usage_percent": cpuPct,
		})
	}
	return results, nil
}

// FetchProcedureStats returns cached procedure execution statistics.
func (c *MssqlRepository) FetchProcedureStats(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectProcedureStats(db)
}

// FetchFileIOLatency returns file I/O latency statistics.
func (c *MssqlRepository) FetchFileIOLatency(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectFileIOLatency(db)
}

// FetchSpinlockStats returns spinlock contention statistics.
func (c *MssqlRepository) FetchSpinlockStats(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectSpinlockStats(db)
}

// FetchMemoryClerks returns memory allocation by clerk.
func (c *MssqlRepository) FetchMemoryClerks(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectMemoryClerks(db)
}

// FetchTempdbStats returns TempDB file usage statistics.
func (c *MssqlRepository) FetchTempdbStats(instanceName string) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectTempDBStats(db)
}
