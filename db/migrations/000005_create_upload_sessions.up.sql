CREATE TYPE upload_session_status AS ENUM (
    'ACTIVE',
    'COMPLETED',
    'ABANDONED',
    'FAILED'
);

CREATE TABLE upload_sessions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    file_id         uuid NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    upload_id       text,
    declared_size   bigint NOT NULL CHECK (declared_size >= 0),
    reserved_bytes  bigint NOT NULL CHECK (reserved_bytes >= 0),
    parts_uploaded  integer NOT NULL DEFAULT 0 CHECK (parts_uploaded >= 0),
    status          upload_session_status NOT NULL DEFAULT 'ACTIVE',
    expires_at      timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX upload_sessions_user_id_idx
    ON upload_sessions (user_id);

CREATE INDEX upload_sessions_expires_at_idx
    ON upload_sessions (expires_at)
    WHERE status = 'ACTIVE';
