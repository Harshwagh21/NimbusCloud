-- The caller generates id so the storage key users/{owner}/files/{id}/... exists before the row.
-- name: CreateFile :one
INSERT INTO files (
    id,
    owner_id,
    folder_id,
    name,
    storage_key,
    mime_type,
    size,
    checksum,
    status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetFileByID :one
SELECT *
FROM files
WHERE id = $1
  AND owner_id = $2
  AND deleted_at IS NULL;

-- name: ListFilesByFolder :many
SELECT *
FROM files
WHERE owner_id = $1
  AND deleted_at IS NULL
  AND status = 'READY'
  AND folder_id IS NOT DISTINCT FROM $2
ORDER BY name;

-- name: SoftDeleteFile :one
UPDATE files
SET deleted_at = now(),
    updated_at = now()
WHERE id = $1
  AND owner_id = $2
  AND deleted_at IS NULL
RETURNING *;

-- Only UPLOADING -> READY. A second completion, or another owner, matches no row.
-- name: MarkFileReady :one
UPDATE files
SET status = 'READY',
    mime_type = sqlc.arg(mime_type),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND owner_id = sqlc.arg(owner_id)
  AND status = 'UPLOADING'
  AND deleted_at IS NULL
RETURNING *;
