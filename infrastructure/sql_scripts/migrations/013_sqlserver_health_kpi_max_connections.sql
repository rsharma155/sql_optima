-- SQL Optima — add max_connections to SQL Server health KPIs (for % saturation alerts).
-- Idempotent upgrade migration.

ALTER TABLE IF EXISTS sqlserver_health_kpis_v2
    ADD COLUMN IF NOT EXISTS max_connections INTEGER;

COMMENT ON COLUMN sqlserver_health_kpis_v2.max_connections IS
    'user_connections ceiling from sys.configurations (user connections); 0/NULL when unknown.';
