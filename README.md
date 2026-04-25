# inwheel-server

Backend for the **InWheel** accessibility platform. A global registry of wheelchair accessibility data for physical places.

Licensed under [AGPL-3.0](./LICENSE).

## Services

| Service | Path | Description |
|---|---|---|
| `api` | `cmd/api` | Public REST API for places and accessibility profiles |
| `ingestion` | `cmd/ingestion` | OSM data ingestion worker (in development) |

## Data Model

**Place** — a physical location with coordinates, category, OSM metadata, and an optional parent place (e.g. a shop inside a mall).

**AccessibilityProfile** — attached to a place; contains:
- `overall_status`: `accessible` | `limited` | `inaccessible` | `unknown`
- `components`: structured data per feature type — `entrance`, `restroom`, `parking`, `elevator`

Child places inherit accessibility components from their parent for any component type they don't provide themselves.

## API Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/places` | List places. Supports proximity (`lat`, `lng`, `radius`) and bounding box (`min_lng`, `min_lat`, `max_lng`, `max_lat`) |
| `GET` | `/places/{id}` | Get a single place with its accessibility profile |
| `POST` | `/places` | Create a place (with optional accessibility data) |
| `PATCH` | `/places/{id}/accessibility` | Update or create an accessibility profile |

## Running with Docker Compose

```sh
cp .env.example .env  # set DB_USER, DB_PASSWORD, DB_NAME
docker compose up
```

The API will be available at `http://localhost:8080`.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | API server port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | Database user |
| `DB_PASSWORD` | `postgres` | Database password |
| `DB_NAME` | `inwheel` | Database name |
| `DB_SSLMODE` | `disable` | PostgreSQL SSL mode |
| `DB_MAX_OPEN_CONNS` | `25` | Connection pool max open |
| `DB_MAX_IDLE_CONNS` | `5` | Connection pool max idle |

## Development

```sh
go test ./...                                         # unit tests
go test -tags integration -timeout 120s ./...        # integration tests (requires Docker)
go vet ./...                                          # lint
```
