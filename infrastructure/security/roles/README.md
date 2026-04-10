# Least-privilege monitoring roles

Enterprise-friendly SQL to create dedicated monitoring principals:

- [postgres_monitor_role.sql](postgres_monitor_role.sql) — PostgreSQL (`pg_monitor`, read stats/settings).
- [sqlserver_monitor_role.sql](sqlserver_monitor_role.sql) — SQL Server (`VIEW SERVER STATE`, `VIEW ANY DEFINITION`, msdb agent reads).

These mirror the fuller bootstrap scripts under `infrastructure/sql_scripts/` but are intended as reviewable, minimal grants for security questionnaires.

**Do not use example passwords in production.** Set passwords via your secret manager or DBA process.
