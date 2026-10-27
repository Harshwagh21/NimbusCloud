-- name: CreateFile :one
INSERT INTO files (
    owner_id,
    folder_id,
    name,
    storage_key,
    mime_type,
    size,
    checksum,
    status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
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
