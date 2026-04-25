-- Migration: 00016_add_avg_reads_to_qs_interval.sql
-- Description: Add missing avg_reads column to sqlserver_query_store_interval table.
-- Author: Ravi Sharma

ALTER TABLE monitor.sqlserver_query_store_interval
ADD COLUMN IF NOT EXISTS avg_reads DOUBLE PRECISION;
