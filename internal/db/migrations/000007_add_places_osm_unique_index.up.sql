CREATE UNIQUE INDEX IF NOT EXISTS places_osm_unique
    ON places (osm_id, osm_type)
    WHERE osm_id <> 0;
