CREATE TYPE place_status AS ENUM ('active', 'closed', 'osm_removed');

ALTER TABLE places
    ADD COLUMN external_ids JSONB,
    ADD COLUMN status       place_status NOT NULL DEFAULT 'active';

CREATE INDEX idx_places_external_ids ON places USING GIN (external_ids);

ALTER TABLE accessibility_profiles
    ADD COLUMN user_verified BOOLEAN NOT NULL DEFAULT FALSE;
