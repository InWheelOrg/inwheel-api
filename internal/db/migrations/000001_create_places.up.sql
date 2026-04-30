CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE IF NOT EXISTS places (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    osm_id     BIGINT,
    osm_type   TEXT,
    name       TEXT,
    lng        DOUBLE PRECISION,
    lat        DOUBLE PRECISION,
    geometry   JSONB,
    category   TEXT,
    rank       INTEGER,
    parent_id  UUID REFERENCES places(id),
    tags       JSONB,
    source     TEXT,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_places_geog
    ON places USING GIST (geography(ST_Point(lng, lat)));
