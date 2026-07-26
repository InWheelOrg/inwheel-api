CREATE TABLE IF NOT EXISTS accessibility_profiles (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    place_id       UUID NOT NULL UNIQUE REFERENCES places(id),
    source_reports JSONB,
    entrance       JSONB,
    pathways       JSONB,
    restroom       JSONB,
    parking        JSONB,
    elevator       JSONB,
    updated_at     TIMESTAMPTZ
);
