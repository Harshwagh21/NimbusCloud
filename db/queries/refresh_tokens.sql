-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, family_id, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetRefreshTokenByHash :one
SELECT *
FROM refresh_tokens
WHERE token_hash = $1;

-- name: GetRefreshTokenByHashForUpdate :one
-- Rotation locks this row so two refreshes of the same token cannot both succeed.
-- The unique token_hash index is the lookup; FOR UPDATE is the lock, not a new index.
SELECT *
FROM refresh_tokens
WHERE token_hash = $1
FOR UPDATE;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET revoked_at = now()
WHERE id = $1
  AND revoked_at IS NULL;

-- name: RevokeRefreshTokenFamily :exec
UPDATE refresh_tokens
SET revoked_at = now()
WHERE family_id = $1
  AND revoked_at IS NULL;
