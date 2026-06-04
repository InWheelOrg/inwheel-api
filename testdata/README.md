# Test fixtures

## `andorra-sample.osm.pbf`

A small extract of OpenStreetMap data covering Andorra, used by
`cmd/ingestion`'s integration test (`go test -tags integration ./cmd/ingestion/...`)
to verify the full-import pipeline against a real Postgres/PostGIS container.

### Attribution and license

OpenStreetMap data, including this extract, is © OpenStreetMap contributors
and licensed under the [Open Database License (ODbL) 1.0](https://opendatacommons.org/licenses/odbl/1-0/).

See https://www.openstreetmap.org/copyright for full attribution requirements.

The ODbL governs only the contents of `.osm.pbf` files in this directory.
The surrounding source code is licensed under AGPL-3.0 — see `LICENSE`
at the repository root.
