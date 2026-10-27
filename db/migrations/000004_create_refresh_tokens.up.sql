CREATE TABLE refresh_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash  text NOT NULL,
    family_id   uuid NOT NULL,
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT refresh_tokens_token_hash_unique UNIQUE (token_hash),
    CONSTRAINT refresh_tokens_token_hash_not_blank CHECK (length(btrim(token_hash)) > 0)
);

-- Every refresh looks up the opaque token by its SHA-256 hash.
CREATE INDEX refresh_tokens_token_hash_idx
    ON refresh_tokens (token_hash);

CREATE INDEX refresh_tokens_family_id_idx
    ON refresh_tokens (family_id);
