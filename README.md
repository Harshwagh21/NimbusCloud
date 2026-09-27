# Nimbus API

Backend for **NimbusCloud**, a cloud file storage service. Go and Gin, PostgreSQL for
metadata, S3-compatible object storage for file contents.

Frontend: [NimbusUI](../NimbusUI)

## The defining design choice

File bytes never pass through this API. An upload request is authorised, quota is reserved,
and the client receives a short-lived presigned URL; the browser then transfers the bytes
straight to object storage. A 512 MB container can therefore serve a multi-gigabyte upload.

```text
Browser ──1. POST /uploads──────────► API ──reserve quota, create row (UPLOADING)
Browser ◄─2. presigned PUT URL───────┘
Browser ──3. bytes───────────────────────────────────► MinIO / R2
Browser ──4. POST /uploads/:id/complete─► API ──HeadObject verifies size and type,
                                                row becomes READY, quota committed
```

## Stack

| Concern | Choice |
| --- | --- |
| Language / HTTP | Go, Gin |
| Database | PostgreSQL — pgx driver, sqlc-generated queries, no ORM |
| Object storage | MinIO locally, Cloudflare R2 in production (one S3 implementation) |
| Auth | Argon2id passwords, short-lived JWT access tokens, rotating refresh tokens |
| Logging | `log/slog`, structured, request-id correlated |

## Quick start

Requirements: Go 1.26+, Docker, GNU Make.

```powershell
git clone <this repo> && cd NimbusCloud
copy .env.example .env      # defaults work as-is for local development
make tools                  # sqlc, golang-migrate, golangci-lint
make up                     # Postgres + MinIO
make run                    # API on http://localhost:8080
```

Verify:

```powershell
curl http://localhost:8080/healthz    # {"status":"ok","version":"dev"}
curl http://localhost:8080/readyz     # {"status":"ok","checks":{"postgres":"ok"}}
```

### Local services

| Service | Address | Notes |
| --- | --- | --- |
| API | http://localhost:8080 | |
| PostgreSQL | localhost:**5433** | 5433, not 5432, to avoid clashing with a host install |
| MinIO API | http://localhost:9000 | bucket `nimbus-files`, private |
| MinIO console | http://localhost:9001 | `nimbus_minio` / `nimbus_minio_dev_password` |

## Make targets

```text
make up | down | logs | reset        docker compose lifecycle
make run | build                     run or build the API
make migrate-up | migrate-down       apply or roll back migrations
make migrate-new name=add_users      create a migration pair
make sqlc                            regenerate typed queries
make test | test-unit | lint | fmt   quality gates
make tools                           install sqlc, migrate, golangci-lint
```

## Layout

```text
cmd/api/              wiring only
internal/<domain>/    handler.go · service.go · repository.go  (auth, user, folder, file, share, search)
internal/platform/    apierr, httpx, database, logging
internal/health/      liveness and readiness probes
db/migrations/        numbered, each reversible
db/queries/           hand-written SQL, input to sqlc
db/generated/         sqlc output, committed, never hand-edited
storage/              ObjectStorage interface + S3 implementation
middleware/           request id, logging, recovery, security headers, CORS
config/               typed environment config, fails fast at startup
```

Strict layering: `handler → service → repository → PostgreSQL`, with object storage reached
from services through an interface. Handlers hold no business logic, and services import no
HTTP package — which is what makes them testable without a server.

## Conventions worth knowing before contributing

- **Tests come first.** See `AGENTS.md`.
- **Authorization is a query predicate**, never a check after fetching:
  `WHERE id = $1 AND owner_id = $2`.
- **Another user's resource returns 404, not 403**, so the API never confirms it exists.
- **One error envelope everywhere**: `{"error": {"code": "...", "message": "..."}}`. Status
  codes come from a single mapper in `internal/platform/apierr`.
- **Correlate with `X-Request-Id`** — it is on every response and in every log line.
- Configuration is validated at startup and reports *all* problems at once.

## Documentation

Architecture notes, decision records, and per-feature write-ups live in the project's
Obsidian vault rather than in this repository. `AGENTS.md` describes what must be recorded
there and when.

## Status

Phase 0 complete: infrastructure, configuration, middleware, health probes, CI.
Next: Phase 1, the database schema and typed query layer.
