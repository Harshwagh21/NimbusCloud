-- Case-insensitive email uniqueness: Harsh@x.com and harsh@x.com are the same account.
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email           citext NOT NULL,
    password_hash   text NOT NULL,
    name            text NOT NULL,
    avatar_url      text,
    storage_used    bigint NOT NULL DEFAULT 0 CHECK (storage_used >= 0),
    -- Matches the free-tier default in config (10 GiB). Premium raises it in application code.
    storage_limit   bigint NOT NULL DEFAULT 10737418240 CHECK (storage_limit >= 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT users_email_unique UNIQUE (email)
);
