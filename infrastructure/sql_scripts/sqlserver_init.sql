-- ============================================================================
-- SQL Server Monitoring User Initialization Script
-- ============================================================================
-- Purpose: Creates a dedicated monitoring user with minimal required permissions
--          for the SQL Optima monitoring system.
--
-- Usage:   Execute this script on your SQL Server instance as a sysadmin or
--          user with privileges to create logins and grant permissions.
--
-- Note:    Adjust the password complexity according to your security policy.
-- ============================================================================

USE master;
GO

-- Create login for monitoring user (if it doesn't exist)
IF NOT EXISTS (SELECT name FROM sys.server_principals WHERE name = 'dbmonitor_user')
BEGIN
    CREATE LOGIN [dbmonitor_user] WITH 
        PASSWORD = N'Hello@123',
        DEFAULT_DATABASE = [master],
        CHECK_POLICY = ON,
        CHECK_EXPIRATION = OFF;
    
    PRINT 'Login [dbmonitor_user] created successfully.';
END
ELSE
BEGIN
    PRINT 'Login [dbmonitor_user] already exists.';
END
GO

-- Grant server-level permissions for performance counters
USE master;
GO
GRANT VIEW SERVER STATE TO [dbmonitor_user];
GRANT VIEW ANY DEFINITION TO [dbmonitor_user];
GO

-- Create user in msdb for SQL Agent job monitoring
USE msdb;
GO

-- Create user if it doesn't exist
IF NOT EXISTS (SELECT name FROM sys.database_principals WHERE name = 'dbmonitor_user')
BEGIN
    CREATE USER [dbmonitor_user] FOR LOGIN [dbmonitor_user];
    PRINT 'User [dbmonitor_user] created in msdb.';
END
ELSE
BEGIN
    PRINT 'User [dbmonitor_user] already exists in msdb.';
END
GO

-- Grant SELECT permissions on SQL Agent tables for job monitoring
GRANT SELECT ON dbo.sysjobs TO [dbmonitor_user];
GRANT SELECT ON dbo.sysjobschedules TO [dbmonitor_user];
GRANT SELECT ON dbo.sysjobactivity TO [dbmonitor_user];
GRANT SELECT ON dbo.sysjobhistory TO [dbmonitor_user];
GRANT SELECT ON dbo.sysschedules TO [dbmonitor_user];
GRANT SELECT ON dbo.syscategories TO [dbmonitor_user];
GRANT SELECT ON dbo.sysjobsteps TO [dbmonitor_user];
GRANT SELECT ON dbo.sysoperators TO [dbmonitor_user];
GO

PRINT 'Granted SELECT permissions on msdb SQL Agent tables to [dbmonitor_user].';
GO

-- Add to SQLAgentReaderRole for enhanced job visibility (if role exists)
EXEC sp_addrolemember 'SQLAgentReaderRole', 'dbmonitor_user';
GO

-- Create user in each target database for query monitoring
-- Note: Run this section for each database you want to monitor
/*
USE [YourDatabaseName];
GO
IF NOT EXISTS (SELECT name FROM sys.database_principals WHERE name = 'dbmonitor_user')
BEGIN
    CREATE USER [dbmonitor_user] FOR LOGIN [dbmonitor_user];
END
GRANT SELECT ON SCHEMA::dbo TO [dbmonitor_user];
GO
*/

PRINT '';
PRINT '========================================';
PRINT 'SQL Server monitoring user setup complete.';
PRINT '========================================';
PRINT 'Login: dbmonitor_user';
PRINT 'Password: MonitorPass123! (change this!)';
PRINT '';
PRINT 'Required grants applied:';
PRINT '  - VIEW SERVER STATE';
PRINT '  - VIEW ANY DEFINITION';
PRINT '  - msdb: sysjobs, sysjobschedules, sysjobactivity';
PRINT '  - msdb: sysjobhistory, sysschedules, syscategories';
PRINT '  - msdb: sysjobsteps, sysoperators';
PRINT '  - SQLAgentReaderRole membership';
PRINT '========================================';
GO
