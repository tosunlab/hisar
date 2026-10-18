-- Organizations give vaults to teams of their members (DESIGN §5.11). created_by, added_by and granted_by may name a
-- deleted account, as vaults.created_by does since 00006. Down refuses to run while organization vaults exist (the
-- restored type check fails), so a rollback never deletes vaults: delete them first.

-- +goose Up
CREATE TABLE organizations (
    id              uuid PRIMARY KEY,
    name            text NOT NULL,
    created_by      text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE organization_members (
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    account_id      text NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    role            text NOT NULL CHECK (role IN ('admin', 'member')),
    added_by        text NOT NULL,
    added_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, account_id)
);

CREATE INDEX organization_members_account_idx ON organization_members (account_id);

CREATE TABLE organization_invitations (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email           text NOT NULL,
    role            text NOT NULL CHECK (role IN ('admin', 'member')),
    invited_by      text NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    UNIQUE (organization_id, email)
);

CREATE INDEX organization_invitations_email_idx ON organization_invitations (email);
CREATE INDEX organization_invitations_invited_by_idx ON organization_invitations (invited_by);

CREATE TABLE teams (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            text NOT NULL,
    created_by      text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, organization_id)
);

CREATE UNIQUE INDEX teams_name_idx ON teams (organization_id, lower(name));

CREATE TABLE team_members (
    team_id         uuid NOT NULL,
    organization_id uuid NOT NULL,
    account_id      text NOT NULL,
    added_by        text NOT NULL,
    added_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, account_id),
    FOREIGN KEY (team_id, organization_id) REFERENCES teams (id, organization_id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, account_id) REFERENCES organization_members (organization_id, account_id) ON DELETE CASCADE
);

CREATE INDEX team_members_account_idx ON team_members (account_id);

ALTER TABLE vaults DROP CONSTRAINT vaults_type_check;
ALTER TABLE vaults ADD CONSTRAINT vaults_type_check CHECK (type IN ('personal', 'shared', 'organization'));
ALTER TABLE vaults ADD COLUMN organization_id uuid REFERENCES organizations (id);
ALTER TABLE vaults ADD COLUMN key_generation bigint NOT NULL DEFAULT 1;
ALTER TABLE vaults ADD COLUMN needs_rotation boolean NOT NULL DEFAULT false;
ALTER TABLE vaults ADD CONSTRAINT vaults_organization_check CHECK ((type = 'organization') = (organization_id IS NOT NULL));
ALTER TABLE vaults ADD CONSTRAINT vaults_id_organization_key UNIQUE (id, organization_id);
CREATE INDEX vaults_organization_idx ON vaults (organization_id) WHERE organization_id IS NOT NULL;

CREATE TABLE team_vaults (
    team_id         uuid NOT NULL,
    vault_id        uuid NOT NULL,
    organization_id uuid NOT NULL,
    role            text NOT NULL CHECK (role IN ('owner', 'editor', 'reader')),
    granted_by      text NOT NULL,
    granted_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, vault_id),
    FOREIGN KEY (team_id, organization_id) REFERENCES teams (id, organization_id) ON DELETE CASCADE,
    FOREIGN KEY (vault_id, organization_id) REFERENCES vaults (id, organization_id) ON DELETE CASCADE
);

CREATE INDEX team_vaults_vault_idx ON team_vaults (vault_id);

-- +goose Down
ALTER TABLE vaults DROP CONSTRAINT vaults_type_check;
ALTER TABLE vaults ADD CONSTRAINT vaults_type_check CHECK (type IN ('personal', 'shared'));
DROP TABLE team_vaults;
DROP TABLE team_members;
DROP TABLE teams;
DROP TABLE organization_invitations;
DROP TABLE organization_members;
ALTER TABLE vaults DROP CONSTRAINT vaults_id_organization_key;
ALTER TABLE vaults DROP CONSTRAINT vaults_organization_check;
ALTER TABLE vaults DROP COLUMN needs_rotation;
ALTER TABLE vaults DROP COLUMN key_generation;
ALTER TABLE vaults DROP COLUMN organization_id;
DROP TABLE organizations;
