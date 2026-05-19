-- +goose Up
-- +goose StatementBegin
-- Baseline: schema was bootstrapped by infrastructure/sql_scripts/01_timescale_schema.sql
-- run via the schema-setup Docker container.  This migration exists only so
-- that goose has a starting version and all subsequent incremental migrations
-- can be applied on top without re-running the full idempotent script.
--
-- If you are running on a fresh database that has NOT been bootstrapped by the
-- sql_scripts yet, run infrastructure/sql_scripts/01_timescale_schema.sql first,
-- then run `goose up` to apply incremental migrations starting from version 2.
SELECT 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 1; -- baseline is never rolled back
-- +goose StatementEnd
