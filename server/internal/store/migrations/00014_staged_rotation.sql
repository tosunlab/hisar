-- +goose Up
ALTER TABLE vaults ADD COLUMN metadata_revision bigint NOT NULL DEFAULT 1;

-- +goose StatementBegin
CREATE FUNCTION bump_vault_metadata_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.enc_name IS DISTINCT FROM OLD.enc_name THEN
        NEW.metadata_revision = OLD.metadata_revision + 1;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER vault_metadata_revision BEFORE UPDATE OF enc_name ON vaults
    FOR EACH ROW EXECUTE FUNCTION bump_vault_metadata_revision();

CREATE TRIGGER bump_team_vault_access_revision AFTER INSERT OR UPDATE OR DELETE ON team_vaults
    FOR EACH ROW EXECUTE FUNCTION bump_vault_access_revision();
-- +goose StatementBegin
CREATE FUNCTION bump_team_member_access_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE vaults SET access_revision = access_revision + 1
    WHERE id IN (SELECT vault_id FROM team_vaults WHERE team_id IN (NEW.team_id, OLD.team_id));
    RETURN COALESCE(NEW, OLD);
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER bump_team_member_access_revision AFTER INSERT OR UPDATE OR DELETE ON team_members
    FOR EACH ROW EXECUTE FUNCTION bump_team_member_access_revision();

CREATE TABLE vault_transitions (
    id uuid PRIMARY KEY,
    vault_id uuid NOT NULL,
    actor_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('rotate', 'delete-move')),
    generation bigint NOT NULL,
    item_seq bigint NOT NULL,
    access_revision bigint NOT NULL,
    metadata_revision bigint NOT NULL,
    source_name bytea NOT NULL,
    target_id uuid,
    target_generation bigint,
    target_seq bigint,
    target_access_revision bigint,
    target_metadata_revision bigint,
    state text NOT NULL DEFAULT 'staging' CHECK (state IN ('staging', 'committed', 'aborted')),
    enc_name bytea,
    item_count bigint NOT NULL DEFAULT 0,
    byte_budget bigint NOT NULL,
    staged_bytes bigint NOT NULL DEFAULT 0 CHECK (staged_bytes >= 0 AND staged_bytes <= byte_budget),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now() + interval '1 hour'
);
CREATE INDEX vault_transitions_vault_idx ON vault_transitions(vault_id, state, expires_at);
CREATE TABLE vault_transition_items (
    transition_id uuid NOT NULL REFERENCES vault_transitions(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    base_revision uuid NOT NULL,
    revision uuid NOT NULL,
    deleted boolean NOT NULL,
    overview bytea NOT NULL,
    details bytea NOT NULL,
    PRIMARY KEY(transition_id, id)
);
CREATE TABLE vault_transition_access (
    transition_id uuid NOT NULL REFERENCES vault_transitions(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    role text NOT NULL,
    wrapped_key bytea NOT NULL,
    PRIMARY KEY(transition_id, account_id)
);

-- +goose Down
DROP TABLE vault_transition_access;
DROP TABLE vault_transition_items;
DROP TABLE vault_transitions;
DROP TRIGGER bump_team_member_access_revision ON team_members;
DROP FUNCTION bump_team_member_access_revision();
DROP TRIGGER bump_team_vault_access_revision ON team_vaults;
DROP TRIGGER vault_metadata_revision ON vaults;
DROP FUNCTION bump_vault_metadata_revision();
ALTER TABLE vaults DROP COLUMN metadata_revision;
