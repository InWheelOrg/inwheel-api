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
    rank       INTEGER NOT NULL CHECK (rank IN (1, 2, 3)),
    parent_id  UUID REFERENCES places(id),
    tags       JSONB,
    source     TEXT,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_places_geog
    ON places USING GIST (geography(ST_Point(lng, lat)));

COMMENT ON COLUMN places.rank IS '1 = Landmark (major hubs, hospitals, universities), 2 = Establishment (standard commercial/public), 3 = Feature (minor utilities: toilets, ATMs, shelters)';
