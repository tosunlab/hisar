-- +goose Up
-- An iPhone that gets its pushes through the relay may also give the key its notification service extension opens a
-- push's details with, so the server can say who invited the account to what without the relay or APNs reading it.
ALTER TABLE device_push
    ADD COLUMN payload_p256dh bytea CHECK (length(payload_p256dh) = 65),
    ADD COLUMN payload_auth bytea CHECK (length(payload_auth) = 16),
    ADD CONSTRAINT device_push_payload_key_complete CHECK ((payload_p256dh IS NULL) = (payload_auth IS NULL)),
    ADD CONSTRAINT device_push_payload_key_with_handle CHECK (payload_p256dh IS NULL OR handle IS NOT NULL);

-- +goose Down
ALTER TABLE device_push
    DROP CONSTRAINT device_push_payload_key_with_handle,
    DROP CONSTRAINT device_push_payload_key_complete,
    DROP COLUMN payload_auth,
    DROP COLUMN payload_p256dh;
