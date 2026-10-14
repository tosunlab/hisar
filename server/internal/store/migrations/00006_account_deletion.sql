-- Deleting an account keeps the vaults it shared with another owner, so these columns may name a deleted account,
-- as item_history.updated_by already does. Down adds the constraints back without checking existing rows.

-- +goose Up
ALTER TABLE vaults DROP CONSTRAINT vaults_created_by_fkey;
ALTER TABLE vault_access DROP CONSTRAINT vault_access_granted_by_fkey;
ALTER TABLE items DROP CONSTRAINT items_updated_by_fkey;
CREATE INDEX invites_created_by_idx ON invites (created_by);
CREATE INDEX invites_email_idx ON invites (email);

-- +goose Down
DROP INDEX invites_email_idx;
DROP INDEX invites_created_by_idx;
ALTER TABLE items ADD CONSTRAINT items_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES accounts (id) NOT VALID;
ALTER TABLE vault_access ADD CONSTRAINT vault_access_granted_by_fkey FOREIGN KEY (granted_by) REFERENCES accounts (id) NOT VALID;
ALTER TABLE vaults ADD CONSTRAINT vaults_created_by_fkey FOREIGN KEY (created_by) REFERENCES accounts (id) NOT VALID;
