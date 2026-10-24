-- +goose Up
-- device_push holds a phone's push relay handle and the push kinds it wants. The handle is the relay's sealed push
-- token: this server cannot read the token behind it.
CREATE TABLE device_push (
    device_id uuid PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    handle text NOT NULL CHECK (length(handle) BETWEEN 1 AND 8192),
    kinds text[] NOT NULL CHECK (kinds <@ ARRAY['approval', 'security', 'sharing']::text[]),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE device_push;
