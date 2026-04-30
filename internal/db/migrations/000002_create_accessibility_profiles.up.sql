CREATE TABLE IF NOT EXISTS accessibility_profiles (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    place_id       UUID NOT NULL UNIQUE REFERENCES places(id),
    overall_status TEXT,
    components     JSONB,
    updated_at     TIMESTAMPTZ
);
