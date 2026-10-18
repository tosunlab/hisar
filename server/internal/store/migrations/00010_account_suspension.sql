-- An admin suspends an account from the dashboard: it keeps its data but cannot sign in until reactivated (DESIGN §5.1).
-- The dashboard's account list counts each account's devices, which the index serves.

-- +goose Up
ALTER TABLE accounts ADD COLUMN suspended_at timestamptz;
CREATE INDEX devices_account_idx ON devices (account_id);

-- +goose Down
DROP INDEX devices_account_idx;
ALTER TABLE accounts DROP COLUMN suspended_at;
