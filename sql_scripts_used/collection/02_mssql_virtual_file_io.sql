-- Metric: mssql_virtual_file_io
-- Source: backend/internal/repository/mssql_dashboard.go:252
-- Target Table: sqlserver_disk_history (TimescaleDB)
-- Description: Gets virtual file I/O stats joined with master_files for latency calculation

SELECT DB_NAME(vfs.database_id), mf.physical_name, mf.type_desc, vfs.num_of_reads, vfs.num_of_writes, vfs.io_stall_read_ms, vfs.io_stall_write_ms FROM sys.dm_io_virtual_file_stats(NULL, NULL) AS vfs JOIN sys.master_files AS mf WITH (NOLOCK) ON vfs.database_id = mf.database_id AND vfs.file_id = mf.file_id;
