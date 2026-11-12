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

-- name: UpdateFolder :one
UPDATE folders
SET name = $3,
    parent_id = $4,
    updated_at = now()
WHERE id = $1
  AND owner_id = $2
  AND deleted_at IS NULL
RETURNING *;

-- name: CountLiveChildren :one
SELECT COUNT(*)::bigint AS count
FROM folders
WHERE owner_id = $1
  AND parent_id = $2
  AND deleted_at IS NULL;

-- Walk from the folder up to the root. Ordered root → leaf for breadcrumbs.
-- name: ListFolderPath :many
WITH RECURSIVE ancestors AS (
    SELECT
        f.id,
        f.owner_id,
        f.parent_id,
        f.name,
        f.created_at,
        f.updated_at,
        f.deleted_at,
        0 AS lvl
    FROM folders f
    WHERE f.id = $1
      AND f.owner_id = $2
      AND f.deleted_at IS NULL

    UNION ALL

    SELECT
        p.id,
        p.owner_id,
        p.parent_id,
        p.name,
        p.created_at,
        p.updated_at,
        p.deleted_at,
        a.lvl + 1
    FROM folders p
    INNER JOIN ancestors a ON p.id = a.parent_id
    WHERE p.owner_id = $2
      AND p.deleted_at IS NULL
)
SELECT
    ancestors.id,
    ancestors.owner_id,
    ancestors.parent_id,
    ancestors.name,
    ancestors.created_at,
    ancestors.updated_at,
    ancestors.deleted_at
FROM ancestors
ORDER BY ancestors.lvl DESC;

