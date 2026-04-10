-- Metric: mssql_xe_file_target_read
-- Source: backend/internal/service/xe_file_target_worker.go:280
-- Target Table: sql_server_xevents (SQLite)
-- Description: Reads Extended Events from file target with cursor-based pagination

SELECT 
    object_name AS event_type,
    CAST(event_data AS XML) AS event_data_xml,
    file_name,
    file_offset
FROM sys.fn_xe_file_target_read_file(@p1, NULL, @p2, @p3)
ORDER BY file_name, file_offset;
