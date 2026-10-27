CREATE TYPE file_status AS ENUM (
    'UPLOADING',
    'READY',
    'FAILED',
    'PROCESSING'
);

CREATE TABLE files (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    folder_id    uuid REFERENCES folders (id) ON DELETE RESTRICT,
    name         text NOT NULL,
    storage_key  text NOT NULL,
    mime_type    text NOT NULL,
    size         bigint NOT NULL CHECK (size >= 0),
    checksum     text,
    status       file_status NOT NULL DEFAULT 'UPLOADING',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz,

    CONSTRAINT files_name_not_blank CHECK (length(btrim(name)) > 0),
    CONSTRAINT files_storage_key_not_blank CHECK (length(btrim(storage_key)) > 0)
);

CREATE UNIQUE INDEX files_unique_name_root
    ON files (owner_id, name)
    WHERE folder_id IS NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX files_unique_name_in_folder
    ON files (owner_id, folder_id, name)
    WHERE folder_id IS NOT NULL AND deleted_at IS NULL;

-- Hottest query: listing a folder's live contents for an owner.
CREATE INDEX files_owner_folder_live_idx
    ON files (owner_id, folder_id)
    WHERE deleted_at IS NULL;

-- Trash view for an owner.
CREATE INDEX files_owner_deleted_idx
    ON files (owner_id, deleted_at);

-- Recent view and keyset pagination.
CREATE INDEX files_owner_created_desc_idx
    ON files (owner_id, created_at DESC);
