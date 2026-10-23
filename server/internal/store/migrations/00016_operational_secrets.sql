-- +goose Up
CREATE TABLE operational_secrets (
    id smallint PRIMARY KEY CHECK (id = 1),
    fingerprint bytea,
    CHECK (fingerprint IS NULL OR octet_length(fingerprint) = 32)
);
INSERT INTO operational_secrets (id) VALUES (1);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM operational_secrets WHERE fingerprint IS NOT NULL) THEN
        RAISE EXCEPTION 'Operational secrets are enrolled. Restore the matching pre-upgrade database and secrets backup instead of rolling back this migration.';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE operational_secrets;
