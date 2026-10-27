CREATE TABLE folders (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    parent_id   uuid REFERENCES folders (id) ON DELETE CASCADE,
    name        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz,

    CONSTRAINT folders_name_not_blank CHECK (length(btrim(name)) > 0)
);

-- Root folders (parent_id IS NULL) need their own partial unique index because
-- Postgres treats NULLs as distinct in unique indexes.
CREATE UNIQUE INDEX folders_unique_name_root
    ON folders (owner_id, name)
    WHERE parent_id IS NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX folders_unique_name_in_parent
    ON folders (owner_id, parent_id, name)
    WHERE parent_id IS NOT NULL AND deleted_at IS NULL;

-- Sidebar tree and folder navigation: list children of a parent for an owner.
CREATE INDEX folders_owner_parent_idx
    ON folders (owner_id, parent_id);
