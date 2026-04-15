-- SQL Optima: Fix postgres_query_dictionary unique constraint
-- This migration adds a unique constraint required for ON CONFLICT upserts

DO $$
BEGIN
    -- Check if the primary key constraint exists
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'postgres_query_dictionary_pkey'
        AND conrelid = 'postgres_query_dictionary'::regclass
    ) THEN
        -- Check if there's already a unique index on these columns
        IF NOT EXISTS (
            SELECT 1 FROM pg_index i
            JOIN pg_class c ON c.oid = i.indexrelid
            WHERE i.indrelid = 'postgres_query_dictionary'::regclass
            AND i.indisunique = true
            AND array_to_string(i.indkey::int4[], ',') = (
                SELECT array_to_string(array_agg(attnum::int), ',')
                FROM pg_attribute
                WHERE attrelid = 'postgres_query_dictionary'::regclass
                AND attname IN ('server_instance_name', 'query_id')
            )
        ) THEN
            -- Add the primary key constraint
            ALTER TABLE postgres_query_dictionary 
            ADD CONSTRAINT postgres_query_dictionary_pkey 
            PRIMARY KEY (server_instance_name, query_id);
        END IF;
    END IF;
END $$;
