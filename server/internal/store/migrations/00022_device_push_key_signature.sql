-- +goose Up
-- The account's signature over the key a phone's push messages are encrypted to: a Mac checks it before it encrypts an
-- approval's preview to that key, so the server cannot hand it a key of its own.
ALTER TABLE device_push
    ADD COLUMN key_signature bytea CHECK (length(key_signature) = 64),
    ADD CONSTRAINT device_push_key_signature_with_key CHECK (key_signature IS NULL OR web_push_p256dh IS NOT NULL OR payload_p256dh IS NOT NULL);

-- +goose Down
ALTER TABLE device_push
    DROP CONSTRAINT device_push_key_signature_with_key,
    DROP COLUMN key_signature;
