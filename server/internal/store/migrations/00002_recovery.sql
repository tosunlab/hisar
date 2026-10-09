-- +goose Up
CREATE TABLE recovery_keyset (
    singleton       boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    kem_pub         bytea NOT NULL,
    sign_pub        bytea NOT NULL,
    kem_sig         bytea NOT NULL,
    enc_keyset      bytea NOT NULL,
    created_by      text NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE vaults ADD COLUMN recovery_opt_out boolean NOT NULL DEFAULT false;

CREATE TABLE vault_recovery (
    vault_id        uuid PRIMARY KEY REFERENCES vaults (id) ON DELETE CASCADE,
    wrapped_key     bytea NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE recovery_tokens (
    account_id      text PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    token_hash      bytea NOT NULL UNIQUE,
    created_by      text REFERENCES accounts (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    used_at         timestamptz
);

CREATE TABLE account_recovery (
    account_id      text PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    argon2_params   jsonb NOT NULL,
    salt            bytea NOT NULL,
    opaque_record   bytea NOT NULL,
    enc_keyset      bytea NOT NULL,
    kem_pub         bytea NOT NULL,
    sign_pub        bytea NOT NULL,
    kem_sig         bytea NOT NULL,
    submitted_at    timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE account_recovery;
DROP TABLE recovery_tokens;
DROP TABLE vault_recovery;
ALTER TABLE vaults DROP COLUMN recovery_opt_out;
DROP TABLE recovery_keyset;
