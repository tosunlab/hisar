-- +goose Up
ALTER TABLE operational_secrets ADD COLUMN settings_version smallint NOT NULL DEFAULT 0 CHECK (settings_version IN (0, 1));

ALTER TABLE instance_settings ADD CONSTRAINT smtp_password_encrypted CHECK (
    settings #> '{smtp,password}' IS NULL
    OR settings #> '{smtp,password}' = '""'::jsonb
    OR (
        jsonb_typeof(settings #> '{smtp,password}') = 'object'
        AND settings #> '{smtp,password,version}' = '1'::jsonb
        AND jsonb_typeof(settings #> '{smtp,password,ciphertext}') = 'string'
        AND length(settings #>> '{smtp,password,ciphertext}') > 0
    ) IS TRUE
) NOT VALID;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM operational_secrets WHERE settings_version <> 0) THEN
        RAISE EXCEPTION 'Stored SMTP passwords are encrypted. Restore the matching pre-upgrade database and secrets backup instead of rolling back this migration.';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE instance_settings DROP CONSTRAINT smtp_password_encrypted;
ALTER TABLE operational_secrets DROP COLUMN settings_version;
