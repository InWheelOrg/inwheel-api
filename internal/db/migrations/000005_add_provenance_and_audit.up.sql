ALTER TABLE places
    ADD COLUMN submitted_by UUID,
    ADD COLUMN submitted_at TIMESTAMPTZ;

ALTER TABLE accessibility_profiles
    ADD COLUMN submitted_by UUID,
    ADD COLUMN submitted_at TIMESTAMPTZ;

CREATE TABLE write_logs (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    target_table TEXT        NOT NULL,
    record_id    UUID        NOT NULL,
    api_key_id   UUID,
    action       TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_write_logs_record_id  ON write_logs (record_id);
CREATE INDEX idx_write_logs_api_key_id ON write_logs (api_key_id);
CREATE INDEX idx_write_logs_created_at ON write_logs (created_at);
