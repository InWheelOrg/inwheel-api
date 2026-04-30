CREATE TABLE IF NOT EXISTS api_keys (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email      TEXT NOT NULL,
    key_hash   TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_api_keys_email
    ON api_keys (email);

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_email_active
    ON api_keys (email) WHERE revoked_at IS NULL;
