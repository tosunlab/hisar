-- +goose Up
DELETE FROM invites WHERE used_at IS NULL;

ALTER TABLE invites ADD COLUMN share_invitation_id uuid REFERENCES share_invitations (id) ON DELETE SET NULL;
ALTER TABLE invites ADD COLUMN organization_invitation_id uuid REFERENCES organization_invitations (id) ON DELETE SET NULL;
ALTER TABLE invites ADD CONSTRAINT invites_one_source CHECK (num_nonnulls(share_invitation_id, organization_invitation_id) <= 1);
CREATE INDEX invites_share_source_idx ON invites (share_invitation_id) WHERE share_invitation_id IS NOT NULL;
CREATE INDEX invites_organization_source_idx ON invites (organization_invitation_id) WHERE organization_invitation_id IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION invalidate_source_signup_invites() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_TABLE_NAME = 'share_invitations' THEN
        DELETE FROM invites WHERE share_invitation_id = OLD.id AND used_at IS NULL;
    ELSE
        DELETE FROM invites WHERE organization_invitation_id = OLD.id AND used_at IS NULL;
    END IF;
    RETURN OLD;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER share_signup_invites_before_delete BEFORE DELETE ON share_invitations
    FOR EACH ROW EXECUTE FUNCTION invalidate_source_signup_invites();
CREATE TRIGGER organization_signup_invites_before_delete BEFORE DELETE ON organization_invitations
    FOR EACH ROW EXECUTE FUNCTION invalidate_source_signup_invites();

-- +goose Down
DROP TRIGGER organization_signup_invites_before_delete ON organization_invitations;
DROP TRIGGER share_signup_invites_before_delete ON share_invitations;
DROP FUNCTION invalidate_source_signup_invites();
ALTER TABLE invites DROP CONSTRAINT invites_one_source;
ALTER TABLE invites DROP COLUMN organization_invitation_id;
ALTER TABLE invites DROP COLUMN share_invitation_id;
