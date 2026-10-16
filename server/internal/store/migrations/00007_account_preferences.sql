-- One record per account, sealed on the account's devices with K_user; the server keeps only the ciphertext.

-- +goose Up
CREATE TABLE account_preferences (
    account_id  text PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    revision    bigint NOT NULL CHECK (revision > 0),
    ciphertext  bytea NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE account_preferences;
