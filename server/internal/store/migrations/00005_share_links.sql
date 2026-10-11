-- +goose Up
CREATE TABLE share_links (
    id                      text PRIMARY KEY,
    account_id              text NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    ciphertext              bytea NOT NULL,
    encrypted_label         bytea NOT NULL,
    passphrase_salt         bytea,
    passphrase_iterations   integer,
    max_views               integer NOT NULL CHECK (max_views BETWEEN 0 AND 100),
    view_count              integer NOT NULL DEFAULT 0 CHECK (view_count >= 0),
    created_at              timestamptz NOT NULL DEFAULT now(),
    expires_at              timestamptz NOT NULL,
    CHECK ((passphrase_salt IS NULL) = (passphrase_iterations IS NULL))
);

CREATE INDEX share_links_account_idx ON share_links (account_id, created_at);
CREATE INDEX share_links_expires_idx ON share_links (expires_at);

-- +goose Down
DROP TABLE share_links;
