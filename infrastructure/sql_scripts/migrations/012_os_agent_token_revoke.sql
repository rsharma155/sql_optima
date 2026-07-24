-- SQL Optima — OS agent JWT revoke list (jti)
-- Apply on upgrade after 01_timescale_schema.sql when OS agent tokens need instant kill.
-- Idempotent.

CREATE TABLE IF NOT EXISTS optima_os_agent_revoked_tokens (
    jti         TEXT PRIMARY KEY,
    server_id   UUID,
    revoked_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_by  TEXT,
    expires_at  TIMESTAMPTZ NOT NULL,
    reason      TEXT
);

CREATE INDEX IF NOT EXISTS idx_os_agent_revoked_expires
    ON optima_os_agent_revoked_tokens (expires_at);

COMMENT ON TABLE optima_os_agent_revoked_tokens IS
    'Revoked OS-agent JWT ids (jti). Rows older than expires_at may be pruned; middleware ignores expired rows.';
