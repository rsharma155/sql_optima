-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Wait stats snapshot for V2 Dashboard trends.
-- Metadata:
--   - Version: 1.0
--   - Scope: Instance
--   - Frequency: 60s
--   - Source: sys.dm_os_wait_stats
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

SELECT 
    wait_type, 
    wait_time_ms, 
    waiting_tasks_count, 
    signal_wait_time_ms
FROM sys.dm_os_wait_stats WITH (NOLOCK)
WHERE wait_time_ms > 0
  AND wait_type NOT IN (
    'CLR_SEMAPHORE', 'LAZYWRITER_SLEEP', 'RESOURCE_QUEUE', 'SLEEP_TASK',
    'SLEEP_SYSTEMTASK', 'SQLTRACE_BUFFER_FLUSH', 'WAITFOR', 'LOGMGR_QUEUE',
    'CHECKPOINT_QUEUE', 'REQUEST_FOR_DEADLOCK_SEARCH', 'XE_TIMER_EVENT',
    'BROKER_TO_FLUSH', 'BROKER_TASK_STOP', 'CLR_MANUAL_EVENT',
    'CLR_AUTO_EVENT', 'DISPATCHER_QUEUE_SEMAPHORE', 'FT_IFTS_SCHEDULER_IDLE_WAIT',
    'XE_DISPATCHER_WAIT', 'XE_DISPATCHER_JOIN', 'SQLTRACE_INCREMENTAL_FLUSH_SLEEP',
    'ONDEMAND_TASK_QUEUE', 'BROKER_EVENTHANDLER', 'DIRTY_PAGE_POLL',
    'HADR_FILESTREAM_IOMGR_IOCOMPLETION', 'SP_SERVER_DIAGNOSTICS_SLEEP'
  )
ORDER BY wait_time_ms DESC;
