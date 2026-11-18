-- name: CreateUser :one
INSERT INTO users (email, password_hash, name, storage_limit)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1;

-- Check and increment in one statement, so two concurrent completions cannot both pass
-- a limit that has room for only one. No row returned means over the limit.
-- name: AddStorageUsed :one
UPDATE users
SET storage_used = storage_used + sqlc.arg(bytes),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND storage_used + sqlc.arg(bytes) <= storage_limit
RETURNING storage_used;
