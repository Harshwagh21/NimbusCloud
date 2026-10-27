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
