CREATE TABLE accounts (
    id              text PRIMARY KEY,
    email           text NOT NULL UNIQUE,
    name            text NOT NULL,
    role            text NOT NULL CHECK (role IN ('admin', 'member')),
    argon2_params   jsonb NOT NULL,
    salt            bytea NOT NULL,
    opaque_record   bytea NOT NULL,
    enc_keyset      bytea NOT NULL,
    kem_pub         bytea NOT NULL,
    sign_pub        bytea NOT NULL,
    kem_sig         bytea NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE devices (
    id              uuid PRIMARY KEY,
    account_id      text NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    name            text NOT NULL,
    platform        text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    revoked_at      timestamptz
);

CREATE TABLE sessions (
    token_hash      bytea PRIMARY KEY,
    account_id      text NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    device_id       uuid NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    expires_at      timestamptz NOT NULL
);

CREATE INDEX sessions_device_idx ON sessions (device_id);

CREATE TABLE invites (
    token_hash      bytea PRIMARY KEY,
    email           text NOT NULL,
    role            text NOT NULL CHECK (role IN ('admin', 'member')),
    created_by      text REFERENCES accounts (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    used_at         timestamptz
);

CREATE TABLE vaults (
    id              uuid PRIMARY KEY,
    type            text NOT NULL CHECK (type IN ('personal', 'shared')),
    enc_name        bytea NOT NULL,
    seq             bigint NOT NULL DEFAULT 0,
    created_by      text NOT NULL REFERENCES accounts (id),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE vault_access (
    vault_id        uuid NOT NULL REFERENCES vaults (id) ON DELETE CASCADE,
    account_id      text NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    wrapped_key     bytea NOT NULL,
    role            text NOT NULL CHECK (role IN ('owner', 'editor', 'reader')),
    granted_by      text NOT NULL REFERENCES accounts (id),
    granted_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (vault_id, account_id)
);

CREATE INDEX vault_access_account_idx ON vault_access (account_id);

CREATE TABLE items (
    vault_id        uuid NOT NULL REFERENCES vaults (id) ON DELETE CASCADE,
    id              uuid NOT NULL,
    revision        uuid NOT NULL,
    seq             bigint NOT NULL,
    deleted         boolean NOT NULL DEFAULT false,
    overview        bytea NOT NULL,
    details         bytea NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    updated_by      text NOT NULL REFERENCES accounts (id),
    PRIMARY KEY (vault_id, id)
);

CREATE INDEX items_seq_idx ON items (vault_id, seq);

CREATE TABLE item_history (
    vault_id        uuid NOT NULL,
    id              uuid NOT NULL,
    revision        uuid NOT NULL,
    deleted         boolean NOT NULL,
    overview        bytea NOT NULL,
    details         bytea NOT NULL,
    updated_at      timestamptz NOT NULL,
    updated_by      text NOT NULL,
    PRIMARY KEY (vault_id, id, revision),
    FOREIGN KEY (vault_id, id) REFERENCES items (vault_id, id) ON DELETE CASCADE
);

CREATE INDEX item_history_updated_idx ON item_history (updated_at);

CREATE TABLE audit_events (
    id              bigserial PRIMARY KEY,
    account_id      text,
    device_id       uuid,
    type            text NOT NULL,
    detail          jsonb,
    ip              text,
    created_at      timestamptz NOT NULL DEFAULT now()
);
