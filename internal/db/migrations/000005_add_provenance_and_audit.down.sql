ALTER TABLE places
    DROP COLUMN IF EXISTS submitted_by,
    DROP COLUMN IF EXISTS submitted_at;

ALTER TABLE accessibility_profiles
    DROP COLUMN IF EXISTS submitted_by,
    DROP COLUMN IF EXISTS submitted_at;

DROP TABLE IF EXISTS write_logs;
