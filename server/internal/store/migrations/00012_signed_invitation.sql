-- +goose Up
ALTER TABLE share_invitations ADD COLUMN intent jsonb;
ALTER TABLE vaults ADD COLUMN access_revision bigint NOT NULL DEFAULT 1;
CREATE TABLE retired_share_invitations (id uuid PRIMARY KEY, expires_at timestamptz NOT NULL);

-- +goose StatementBegin
CREATE FUNCTION retire_share_invitation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO retired_share_invitations(id, expires_at) VALUES (OLD.id, OLD.expires_at) ON CONFLICT DO NOTHING;
 RETURN OLD;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER retire_share_invitation BEFORE DELETE ON share_invitations FOR EACH ROW EXECUTE FUNCTION retire_share_invitation();

-- +goose StatementBegin
CREATE FUNCTION bump_vault_access_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE vaults SET access_revision = access_revision + 1 WHERE id = COALESCE(NEW.vault_id, OLD.vault_id);
 RETURN COALESCE(NEW, OLD);
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER bump_vault_access_revision AFTER INSERT OR UPDATE OR DELETE ON vault_access FOR EACH ROW EXECUTE FUNCTION bump_vault_access_revision();

-- +goose StatementBegin
CREATE FUNCTION revoke_owner_share_invitations() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.role = 'owner' AND (TG_OP = 'DELETE' OR NEW.role <> 'owner') THEN
  DELETE FROM share_invitations WHERE vault_id = OLD.vault_id AND invited_by = OLD.account_id;
 END IF;
 RETURN COALESCE(NEW, OLD);
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER revoke_owner_share_invitations BEFORE DELETE OR UPDATE OF role ON vault_access FOR EACH ROW EXECUTE FUNCTION revoke_owner_share_invitations();

-- +goose Down
DROP TRIGGER revoke_owner_share_invitations ON vault_access;
DROP FUNCTION revoke_owner_share_invitations();
DROP TRIGGER bump_vault_access_revision ON vault_access;
DROP FUNCTION bump_vault_access_revision();
DROP TRIGGER retire_share_invitation ON share_invitations;
DROP FUNCTION retire_share_invitation();
DROP TABLE retired_share_invitations;
ALTER TABLE share_invitations DROP COLUMN intent;
ALTER TABLE vaults DROP COLUMN access_revision;
