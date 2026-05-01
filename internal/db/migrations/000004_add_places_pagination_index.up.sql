CREATE INDEX IF NOT EXISTS idx_places_pagination
    ON places (updated_at DESC, id ASC);
