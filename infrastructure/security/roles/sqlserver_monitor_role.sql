-- Server-level grants for an existing monitoring login (e.g. dbmonitor_user).
-- Create the login with a strong password using your standard process, then run:
--
--   GRANT VIEW SERVER STATE TO [dbmonitor_user];
--   GRANT VIEW ANY DEFINITION TO [dbmonitor_user];
--   GRANT VIEW DATABASE STATE TO [dbmonitor_user];  -- add per-database as needed
--
-- msdb access for SQL Agent visibility (adjust server/user names).

USE master;
GO
GRANT VIEW SERVER STATE TO [dbmonitor_user];
GRANT VIEW ANY DEFINITION TO [dbmonitor_user];
GO
