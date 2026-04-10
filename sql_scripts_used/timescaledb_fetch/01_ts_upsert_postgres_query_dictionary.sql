-- Metric: ts_upsert_postgres_query_dictionary
-- Source: backend/internal/storage/hot/ts_logger.go:1111
-- Target Table: postgres_query_dictionary
-- Description: Upserts query dictionary entries with cumulative execution count

INSERT INTO postgres_query_dictionary (server_instance_name, query_id, query_text, first_seen, last_seen, execution_count)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (server_instance_name, query_id) 
DO UPDATE SET 
    last_seen = EXCLUDED.last_seen,
    execution_count = postgres_query_dictionary.execution_count + EXCLUDED.execution_count;
