-- +goose Up
CREATE TABLE share_invitations (
    id              uuid PRIMARY KEY,
    vault_id        uuid NOT NULL REFERENCES vaults (id) ON DELETE CASCADE,
    email           text NOT NULL,
    role            text NOT NULL CHECK (role IN ('owner', 'editor', 'reader')),
    invited_by      text NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    state           text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'accepted')),
    accepted_by     text REFERENCES accounts (id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    accepted_at     timestamptz,
    UNIQUE (vault_id, email),
    CHECK ((state = 'accepted') = (accepted_by IS NOT NULL))
);

CREATE INDEX share_invitations_email_idx ON share_invitations (email);

-- +goose Down
DROP TABLE share_invitations;
