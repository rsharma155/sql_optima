-- ============================================================================
-- SQL Server Deadlock Capture Extended Events Session
-- ============================================================================
-- Purpose: Sets up a production-grade Extended Events session to capture
--          deadlocks and grants required permissions to the monitoring user.
--
-- Session Name: sqloptima_deadlocks
-- Target:       event_file (sqloptima_deadlocks.xel)
--
-- Note:         This script should be run by a sysadmin or a user with
--               CONTROL SERVER permissions.
-- ============================================================================

USE master;
GO

PRINT 'Starting SQL Server Deadlock XE Session Setup...';

-- 1. Grant required permissions to the monitoring user
-- The monitoring user needs ALTER ANY EVENT SESSION to manage XE sessions
-- and VIEW SERVER STATE to read them.
IF EXISTS (SELECT name FROM sys.server_principals WHERE name = 'dbmonitor_user')
BEGIN
    PRINT 'Granting XE management permissions to [dbmonitor_user]...';
    GRANT ALTER ANY EVENT SESSION TO [dbmonitor_user];
    GRANT VIEW SERVER STATE TO [dbmonitor_user];
    PRINT 'Permissions granted successfully.';
END
ELSE
BEGIN
    PRINT 'Warning: [dbmonitor_user] login not found. If you are using a different login,';
    PRINT 'please grant ALTER ANY EVENT SESSION and VIEW SERVER STATE to it manually.';
END
GO

-- 2. Create the Deadlock Extended Events Session
-- Check if any other custom session is already capturing deadlocks to avoid duplicates
DECLARE @ExistingSession NVARCHAR(256);
SELECT TOP 1 @ExistingSession = s.name
FROM sys.server_event_session_events e
JOIN sys.server_event_sessions s ON e.event_session_id = s.event_session_id
WHERE e.name = 'xml_deadlock_report'
  AND s.name NOT IN ('sqloptima_deadlocks', 'system_health');

IF @ExistingSession IS NOT NULL
BEGIN
    PRINT 'Note: Another session [' + @ExistingSession + '] is already capturing deadlock reports.';
    PRINT 'To avoid duplicate overhead, creation of [sqloptima_deadlocks] will be skipped.';
END
ELSE IF NOT EXISTS (SELECT * FROM sys.server_event_sessions WHERE name = 'sqloptima_deadlocks')
BEGIN
    PRINT 'Creating Extended Events session [sqloptima_deadlocks]...';
    CREATE EVENT SESSION [sqloptima_deadlocks] ON SERVER 
    ADD EVENT sqlserver.xml_deadlock_report
    ADD TARGET package0.event_file(
        SET filename = N'sqloptima_deadlocks.xel',
            max_file_size = (50), -- 50 MB
            max_rollover_files = (10)
    )
    WITH (
        MAX_MEMORY = 4096 KB,
        EVENT_RETENTION_MODE = ALLOW_SINGLE_EVENT_LOSS,
        MAX_DISPATCH_LATENCY = 30 SECONDS,
        MAX_EVENT_SIZE = 0 KB,
        MEMORY_PARTITION_MODE = NONE,
        TRACK_CAUSALITY = OFF,
        STARTUP_STATE = ON
    );
    PRINT 'Extended Events session [sqloptima_deadlocks] created.';
END
ELSE
BEGIN
    PRINT 'Extended Events session [sqloptima_deadlocks] already exists.';
END
GO

-- 3. Start the session if it's not running
IF EXISTS (SELECT * FROM sys.server_event_sessions WHERE name = 'sqloptima_deadlocks')
   AND NOT EXISTS (SELECT * FROM sys.dm_xe_sessions WHERE name = 'sqloptima_deadlocks')
BEGIN
    PRINT 'Starting Extended Events session [sqloptima_deadlocks]...';
    ALTER EVENT SESSION [sqloptima_deadlocks] ON SERVER STATE = START;
    PRINT 'Extended Events session [sqloptima_deadlocks] started.';
END
ELSE IF EXISTS (SELECT * FROM sys.dm_xe_sessions WHERE name = 'sqloptima_deadlocks')
BEGIN
    PRINT 'Extended Events session [sqloptima_deadlocks] is already running.';
END
GO

-- 4. Verify the session status
SELECT 
    name, 
    CASE WHEN EXISTS (SELECT 1 FROM sys.dm_xe_sessions WHERE name = s.name) 
         THEN 'RUNNING' 
         ELSE 'STOPPED' 
    END as [Status],
    startup_state_desc
FROM sys.server_event_sessions s
WHERE name = 'sqloptima_deadlocks';
GO

PRINT 'SQL Server Deadlock XE Session Setup Complete.';
GO
