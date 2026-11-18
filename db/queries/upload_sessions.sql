-- name: CreateUploadSession :one
INSERT INTO upload_sessions (
    user_id,
    file_id,
    upload_id,
    declared_size,
    reserved_bytes,
    parts_uploaded,
    status,
    expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetUploadSessionByID :one
SELECT *
FROM upload_sessions
WHERE id = $1
  AND user_id = $2;

-- Only ACTIVE -> COMPLETED, owner-scoped like every other lookup.
-- name: CompleteUploadSession :one
UPDATE upload_sessions
SET status = 'COMPLETED',
    updated_at = now()
WHERE id = $1
  AND user_id = $2
  AND status = 'ACTIVE'
RETURNING *;
