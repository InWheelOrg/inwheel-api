CREATE TYPE place_status AS ENUM ('active', 'closed', 'osm_removed');

ALTER TABLE places
    ADD COLUMN external_ids JSONB,
    ADD COLUMN status       place_status NOT NULL DEFAULT 'active';

CREATE INDEX idx_places_external_ids ON places USING GIN (external_ids);

ALTER TABLE accessibility_profiles
    ADD COLUMN user_verified BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE unmatched_external (
    id             BIGSERIAL        PRIMARY KEY,
    source         TEXT             NOT NULL,
    source_id      TEXT             NOT NULL,
    name           TEXT             NOT NULL DEFAULT '',
    category       TEXT             NOT NULL DEFAULT '',
    street         TEXT             NOT NULL DEFAULT '',
    housenumber    TEXT             NOT NULL DEFAULT '',
    payload        JSONB            NOT NULL,
    lat            DOUBLE PRECISION NOT NULL,
    lng            DOUBLE PRECISION NOT NULL,
    geom           GEOGRAPHY(POINT, 4326) NOT NULL,
    last_attempted TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    attempts       INT              NOT NULL DEFAULT 1,
    UNIQUE (source, source_id)
);

CREATE INDEX unmatched_external_geom_idx ON unmatched_external USING GIST (geom);
