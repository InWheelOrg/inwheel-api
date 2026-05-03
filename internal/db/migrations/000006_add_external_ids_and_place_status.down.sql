DROP INDEX IF EXISTS idx_places_external_ids;

ALTER TABLE places
    DROP COLUMN IF EXISTS external_ids,
    DROP COLUMN IF EXISTS status;

DROP TYPE IF EXISTS place_status;

ALTER TABLE accessibility_profiles
    DROP COLUMN IF EXISTS user_verified;
