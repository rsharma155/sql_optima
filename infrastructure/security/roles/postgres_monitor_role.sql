-- Minimal grants for an existing monitoring role (e.g. dbmonitor_user).
-- Create the role and set the password outside this file (vault / ALTER ROLE / corporate process).
--
--   CREATE ROLE dbmonitor_user WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
--   ALTER ROLE dbmonitor_user PASSWORD 'from-your-secret-manager';
--
-- Then run this script as a superuser.

GRANT pg_read_all_settings TO dbmonitor_user;
GRANT pg_read_all_stats TO dbmonitor_user;
GRANT pg_stat_scan_tables TO dbmonitor_user;
GRANT pg_monitor TO dbmonitor_user;
