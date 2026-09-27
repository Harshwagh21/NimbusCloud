# AGENTS.md — Nimbus API (backend)

Workspace-wide rules are in `../AGENTS.md` and apply here too. This file covers the backend.

## Stack — fixed

Go · Gin · pgx/v5 · sqlc · PostgreSQL · AWS SDK for Go v2 against S3-compatible storage
(MinIO locally, Cloudflare R2 in production) · JWT · Argon2id · `slog` for logging.

No ORM. No `database/sql` hand-scanning. No framework swap without an ADR.

## Layout

```text
cmd/api/main.go        wiring only
internal/<domain>/     handler.go · service.go · repository.go · dto.go · *_test.go
internal/platform/     apierr (error → status mapping) · validate
db/migrations/         NNNN_name.up.sql + .down.sql
db/queries/            hand-written SQL, input to sqlc
db/generated/          sqlc output — committed, never hand-edited
storage/               ObjectStorage interface + S3 implementation
middleware/            requestid · logger · recovery · auth · cors · ratelimit
config/                typed env config, fails fast
tests/                 integration + authorization suites
```

Domains: `auth`, `user`, `folder`, `file`, `share`, `search`. A domain is a package, not a
service — this is a modular monolith.

## Layering — enforced

```text
handler → service → repository (sqlc) → PostgreSQL
                 ↘ storage.ObjectStorage → MinIO / R2
```

- **Handlers**: bind, validate shape, call one service method, map the error, write the
  response. No business logic, no SQL, no direct storage calls.
- **Services**: business rules, authorization, transactions, orchestration. **Must not import
  Gin or any HTTP package.** Take and return plain Go types.
- **Repositories**: sqlc-generated queries only. No business rules, no HTTP types.
- **Storage**: reached only from a service, only through the `ObjectStorage` interface.

Dependencies are injected through constructors: `NewFileService(repo FileRepository, store
storage.ObjectStorage, cfg Config)`. Depend on interfaces defined in the consuming package so
every service is testable with fakes.

## Database rules

- Every schema change is a numbered migration with a working `down`. Never edit an applied
  migration; add a new one.
- Write the SQL in `db/queries/`, then run `make sqlc`. Never hand-edit `db/generated/`.
- Justify every new index in `02_Architecture/NC_Database_Schema.md` by naming the query it
  serves, before adding it.
- Authorization lives in the `WHERE` clause: `WHERE id = $1 AND owner_id = $2`.
- Soft-deleted rows are excluded at the query level — `deleted_at IS NULL` — not filtered in Go.
- The service layer owns transactions. Quota reservation plus status change is one transaction.
- Concurrent quota updates use `SELECT ... FOR UPDATE`. A read-then-write quota check is a bug.
- List endpoints are paginated. Keyset pagination, not `OFFSET`.

## Errors and status codes

Services return typed or sentinel errors; a single mapper in `internal/platform/apierr`
converts them to HTTP. Handlers never build status codes from strings.

| Situation | Status |
| --- | --- |
| malformed body or params | 400 |
| missing or invalid access token | 401 |
| authenticated but forbidden action | 403 |
| not found **or owned by someone else** | 404 |
| duplicate name, invalid state transition | 409 |
| semantically invalid (size mismatch, bad enum) | 422 |
| rate limited | 429 |
| unexpected | 500, logged with the request ID, details never returned to the client |

Response envelope: `{"error": {"code": "FILE_NOT_FOUND", "message": "..."}}`. Never leak
internal error text, SQL, or stack traces.

## Testing — write first

- Services: unit tests with fake repositories and a fake object store. Cover the error paths,
  not just the happy one.
- Repositories: integration tests against a real throwaway Postgres.
- Handlers: `httptest` against the router, asserting status codes and the error envelope.
- `tests/authorization_test.go` is a standing suite whose only purpose is to prove user A
  cannot read, modify, delete, move, or share user B's resources. Every new endpoint adds a
  case to it. **This suite is the release gate.**
- Table-driven tests with descriptive case names. No `t.Skip` left in the tree.

## Uploads — the rules that keep this safe

- File bytes never pass through this API. Presigned URLs only.
- Quota is reserved **before** the presigned URL is issued.
- On completion, verify the object with `HeadObject`: real size and content type must match
  what was declared, or the upload is rejected and the reservation released.
- MIME type is sniffed server-side from the object. The client's `Content-Type` is a hint.
- Storage keys are `users/{user_id}/files/{file_id}/{filename}` — built from server-generated
  IDs. A user-supplied path is a path-traversal vulnerability.
- Presigned URL TTLs are short and configured, never hardcoded long.
- Every `UPLOADING` row needs a path to a terminal state, including abandonment.

## Never do these

- Store file bytes in Postgres
- Log secrets, tokens, presigned URLs, or password hashes
- Return another user's data, or confirm that it exists
- Build SQL by string concatenation
- Put business logic in a handler, or HTTP concerns in a repository
- Add a dependency, table, or service without an ADR
- Commit a `.env`, a real secret, or a failing test

## Commands

```powershell
make up            # docker compose: Postgres + MinIO
make migrate-up    # apply migrations
make sqlc          # regenerate typed queries
make run           # start the API
make test          # go test ./...
make lint          # go vet + golangci-lint
```

## Before finishing any task

1. `make test` and `make lint` are green
2. The authorization suite covers any new endpoint
3. The Obsidian feature note is written or updated (see the documentation contract in `../AGENTS.md`)
4. `NC_Progress_Log.md` has an entry
