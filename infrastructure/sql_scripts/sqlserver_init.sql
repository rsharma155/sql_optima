-- ============================================================================
-- SQL Server Monitoring User Initialization Script
-- ============================================================================
-- Purpose: Creates a dedicated monitoring user with minimal required permissions
--          for the SQL Optima monitoring system.
--
-- Usage:   Execute this script on your SQL Server instance as a sysadmin or
--          user with privileges to create logins and grant permissions.
--
-- Note:    Replace @LoginName and @Password values before running, or create the login
--          outside this script and skip the CREATE LOGIN block.
-- ============================================================================


SET NOCOUNT ON;

-------------------------------------------------------------------------------
-- CONFIGURATION
-------------------------------------------------------------------------------
DECLARE 
    @LoginName         SYSNAME        = N'dbmonitor_user',
    @Password          NVARCHAR(256)  = N'ChangeThisStrongPassword!',
    @DefaultDatabase   SYSNAME        = N'master';

DECLARE @SQL NVARCHAR(MAX);

-------------------------------------------------------------------------------
-- CREATE LOGIN
-------------------------------------------------------------------------------
IF NOT EXISTS (
    SELECT 1
    FROM sys.server_principals
    WHERE name = @LoginName
)
BEGIN
    SET @SQL = N'
    CREATE LOGIN ' + QUOTENAME(@LoginName) + N'
    WITH 
        PASSWORD = ' + QUOTENAME(@Password, '''') + N',
        DEFAULT_DATABASE = ' + QUOTENAME(@DefaultDatabase) + N',
        CHECK_POLICY = ON,
        CHECK_EXPIRATION = OFF;
    ';

    EXEC sp_executesql @SQL;

    PRINT 'Login created: ' + @LoginName;
END
ELSE
BEGIN
    PRINT 'Login already exists: ' + @LoginName;
END;

-------------------------------------------------------------------------------
-- SERVER LEVEL PERMISSIONS
-------------------------------------------------------------------------------
SET @SQL = N'
GRANT VIEW SERVER STATE TO ' + QUOTENAME(@LoginName) + N';
GRANT VIEW ANY DEFINITION TO ' + QUOTENAME(@LoginName) + N';
GRANT VIEW ANY DATABASE TO ' + QUOTENAME(@LoginName) + N';
';

EXEC sp_executesql @SQL;

-------------------------------------------------------------------------------
-- Optional: Extended Events Management
-------------------------------------------------------------------------------
BEGIN TRY
    SET @SQL = N'
    GRANT ALTER ANY EVENT SESSION TO ' + QUOTENAME(@LoginName) + N';
    ';

    EXEC sp_executesql @SQL;
END TRY
BEGIN CATCH
    PRINT 'ALTER ANY EVENT SESSION grant skipped.';
END CATCH;

-------------------------------------------------------------------------------
-- MSDB PERMISSIONS
-------------------------------------------------------------------------------
USE msdb;

IF NOT EXISTS (
    SELECT 1
    FROM sys.database_principals
    WHERE name = @LoginName
)
BEGIN
    SET @SQL = N'
    CREATE USER ' + QUOTENAME(@LoginName) + N'
    FOR LOGIN ' + QUOTENAME(@LoginName) + N';
    ';

    EXEC sp_executesql @SQL;
END;

SET @SQL = N'
GRANT SELECT ON dbo.sysjobs TO ' + QUOTENAME(@LoginName) + N';
GRANT SELECT ON dbo.sysjobschedules TO ' + QUOTENAME(@LoginName) + N';
GRANT SELECT ON dbo.sysjobactivity TO ' + QUOTENAME(@LoginName) + N';
GRANT SELECT ON dbo.sysjobhistory TO ' + QUOTENAME(@LoginName) + N';
GRANT SELECT ON dbo.sysschedules TO ' + QUOTENAME(@LoginName) + N';
GRANT SELECT ON dbo.syscategories TO ' + QUOTENAME(@LoginName) + N';
GRANT SELECT ON dbo.sysjobsteps TO ' + QUOTENAME(@LoginName) + N';
GRANT SELECT ON dbo.sysoperators TO ' + QUOTENAME(@LoginName) + N';
GRANT EXECUTE ON dbo.agent_datetime TO ' + QUOTENAME(@LoginName) + N';
';

EXEC sp_executesql @SQL;

-------------------------------------------------------------------------------
-- SQL Agent Role Membership
-------------------------------------------------------------------------------
IF NOT EXISTS (
    SELECT 1
    FROM msdb.sys.database_role_members rm
    JOIN msdb.sys.database_principals r
        ON rm.role_principal_id = r.principal_id
    JOIN msdb.sys.database_principals m
        ON rm.member_principal_id = m.principal_id
    WHERE r.name = 'SQLAgentReaderRole'
      AND m.name = @LoginName
)
BEGIN
    EXEC sp_addrolemember 
        @rolename = 'SQLAgentReaderRole',
        @membername = @LoginName;
END;

-------------------------------------------------------------------------------
-- DISTRIBUTION DATABASE PERMISSIONS
-------------------------------------------------------------------------------
DECLARE 
    @DistributorServer SYSNAME,
    @DistributionDB SYSNAME;

EXEC master.dbo.sp_get_distributor
    @distributor = @DistributorServer OUTPUT;

IF @DistributorServer IS NOT NULL
BEGIN

    SELECT TOP (1)
        @DistributionDB = name
    FROM master.sys.databases
    WHERE is_distributor = 1;

    IF @DistributionDB IS NOT NULL
    BEGIN

        SET @SQL = N'
        USE ' + QUOTENAME(@DistributionDB) + N';

        IF NOT EXISTS (
            SELECT 1
            FROM sys.database_principals
            WHERE name = N''' + @LoginName + N'''
        )
        BEGIN
            CREATE USER ' + QUOTENAME(@LoginName) + N'
            FOR LOGIN ' + QUOTENAME(@LoginName) + N';
        END;

        GRANT SELECT ON dbo.MSdistribution_agents
            TO ' + QUOTENAME(@LoginName) + N';

        GRANT SELECT ON dbo.MSdistribution_history
            TO ' + QUOTENAME(@LoginName) + N';

        GRANT SELECT ON dbo.MSdistribution_status
            TO ' + QUOTENAME(@LoginName) + N';

        GRANT SELECT ON dbo.MSpublications
            TO ' + QUOTENAME(@LoginName) + N';

        GRANT SELECT ON dbo.MSarticles
            TO ' + QUOTENAME(@LoginName) + N';
        ';

        EXEC sp_executesql @SQL;

        PRINT 'Replication permissions granted.';
        PRINT 'Distributor : ' + @DistributorServer;
        PRINT 'Database    : ' + @DistributionDB;
    END
END
ELSE
BEGIN
    PRINT 'Replication distributor not configured.';
END;

-------------------------------------------------------------------------------
-- MASTER DATABASE PERMISSIONS
-------------------------------------------------------------------------------
USE master;

IF NOT EXISTS (
    SELECT 1
    FROM sys.database_principals
    WHERE name = @LoginName
)
BEGIN
    SET @SQL = N'
    CREATE USER ' + QUOTENAME(@LoginName) + N'
    FOR LOGIN ' + QUOTENAME(@LoginName) + N';
    ';

    EXEC sp_executesql @SQL;
END;

SET @SQL = N'
GRANT SELECT ON sys.databases TO ' + QUOTENAME(@LoginName) + N';
GRANT SELECT ON sys.master_files TO ' + QUOTENAME(@LoginName) + N';
';

EXEC sp_executesql @SQL;

-------------------------------------------------------------------------------
-- OPTIONAL: Availability Groups
-------------------------------------------------------------------------------
BEGIN TRY

    SET @SQL = N'
    GRANT SELECT ON sys.availability_groups
        TO ' + QUOTENAME(@LoginName) + N';

    GRANT SELECT ON sys.availability_replicas
        TO ' + QUOTENAME(@LoginName) + N';
    ';

    EXEC sp_executesql @SQL;

END TRY
BEGIN CATCH
    PRINT 'Availability Group permissions skipped.';
END CATCH;

-------------------------------------------------------------------------------
-- SUMMARY
-------------------------------------------------------------------------------
PRINT '';
PRINT '========================================';
PRINT 'SQL Server monitoring user setup complete';
PRINT '========================================';
PRINT 'Login Name : ' + @LoginName;
PRINT '========================================';