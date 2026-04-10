package collectors

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

// CollectCPUMemory returns CPU usage from sys.dm_os_ring_buffers and memory from sys.dm_os_sys_memory
// Parameters: ctx context.Context, db *sql.DB
// Returns: *CPUTick, *MemoryStats, error
func CollectCPUMemory(ctx context.Context, db *sql.DB) (*models.CPUTick, *models.MemoryStats, error) {
	cpuQuery := `
		DECLARE @ts_now bigint = (SELECT cpu_ticks/(cpu_ticks/ms_ticks) FROM sys.dm_os_sys_info WITH (NOLOCK)); 
		SELECT TOP(1)
		    SQLProcessUtilization AS SQL_Server_CPU, 
		    SystemIdle AS System_Idle_CPU, 
		    100 - SystemIdle - SQLProcessUtilization AS Other_Process_CPU,
		    CONVERT(varchar, DATEADD(ms, -1 * (@ts_now - [timestamp]), GETDATE()), 120) AS Event_Time
		FROM ( 
		    SELECT record.value('(./Record/@id)[1]', 'int') AS record_id, 
		        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/SystemIdle)[1]', 'int') AS SystemIdle, 
		        record.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int') AS SQLProcessUtilization, [timestamp] 
		    FROM ( 
		        SELECT [timestamp], CONVERT(xml, record) AS [record]
		        FROM sys.dm_os_ring_buffers
		        WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR'
		        AND record LIKE '%<SystemHealth>%'
		    ) AS x ORDER BY [timestamp] DESC
		) AS y
		ORDER BY record_id DESC`

	var cpu models.CPUTick
	var eventTime sql.NullString

	err := db.QueryRowContext(ctx, cpuQuery).Scan(
		&cpu.SQLProcess, &cpu.SystemIdle, &cpu.OtherProcess, &eventTime,
	)
	if err != nil {
		log.Printf("[Collector] CollectCPUMemory CPU Error: %v", err)
	} else if eventTime.Valid && eventTime.String != "" {
		cpu.EventTime = eventTime.String
	}

	memQuery := `
		SELECT 
			ISNULL(available_physical_memory_kb, 0) / 1024 AS available_mb,
			ISNULL(total_physical_memory_kb, 0) / 1024 AS total_mb
		FROM sys.dm_os_sys_memory`

	var mem models.MemoryStats
	var avail, total sql.NullInt64
	err = db.QueryRowContext(ctx, memQuery).Scan(&avail, &total)
	if err != nil {
		log.Printf("[Collector] CollectCPUMemory Memory Error: %v", err)
	} else {
		mem.AvailableMB = int(avail.Int64)
		mem.TotalMB = int(total.Int64)
		if total.Valid && total.Int64 > 0 {
			mem.UsagePercent = float64(total.Int64-avail.Int64) / float64(total.Int64) * 100
		}
		mem.CapturedAt = time.Now().Format("2006-01-02 15:04:05")
	}

	return &cpu, &mem, nil
}
