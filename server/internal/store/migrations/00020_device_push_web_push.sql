-- +goose Up
-- An Android phone gets its pushes by Web Push at its UnifiedPush distributor, which this server sends to directly,
-- instead of through the push relay; a row of device_push now has either a relay handle or a Web Push subscription.
ALTER TABLE device_push
    ALTER COLUMN handle DROP NOT NULL,
    ADD COLUMN web_push_endpoint text CHECK (length(web_push_endpoint) BETWEEN 1 AND 2048),
    ADD COLUMN web_push_p256dh bytea CHECK (length(web_push_p256dh) = 65),
    ADD COLUMN web_push_auth bytea CHECK (length(web_push_auth) = 16),
    ADD CONSTRAINT device_push_web_push_complete CHECK ((web_push_endpoint IS NULL) = (web_push_p256dh IS NULL) AND (web_push_endpoint IS NULL) = (web_push_auth IS NULL)),
    ADD CONSTRAINT device_push_one_target CHECK ((handle IS NULL) <> (web_push_endpoint IS NULL));

-- +goose Down
DELETE FROM device_push WHERE handle IS NULL;
ALTER TABLE device_push
    DROP CONSTRAINT device_push_one_target,
    DROP CONSTRAINT device_push_web_push_complete,
    DROP COLUMN web_push_auth,
    DROP COLUMN web_push_p256dh,
    DROP COLUMN web_push_endpoint,
    ALTER COLUMN handle SET NOT NULL;
