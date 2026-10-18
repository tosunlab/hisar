-- The admin dashboard's audit log reads newest first, by account and by type (DESIGN §8.3).

-- +goose Up
CREATE INDEX audit_events_created_idx ON audit_events (created_at DESC, id DESC);
CREATE INDEX audit_events_account_idx ON audit_events (account_id, created_at DESC);
CREATE INDEX audit_events_type_idx ON audit_events (type, created_at DESC);

-- +goose Down
DROP INDEX audit_events_type_idx;
DROP INDEX audit_events_account_idx;
DROP INDEX audit_events_created_idx;
