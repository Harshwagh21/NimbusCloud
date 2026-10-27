-- name: CreateFolder :one
INSERT INTO folders (owner_id, parent_id, name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetFolderByID :one
SELECT *
FROM folders
WHERE id = $1
  AND owner_id = $2
  AND deleted_at IS NULL;

-- name: ListFoldersByParent :many
SELECT *
FROM folders
WHERE owner_id = $1
  AND deleted_at IS NULL
  AND parent_id IS NOT DISTINCT FROM $2
ORDER BY name;

-- name: SoftDeleteFolder :one
UPDATE folders
SET deleted_at = now(),
    updated_at = now()
WHERE id = $1
  AND owner_id = $2
  AND deleted_at IS NULL
RETURNING *;

-- name: HardDeleteFolder :exec
DELETE FROM folders
WHERE id = $1
  AND owner_id = $2;
