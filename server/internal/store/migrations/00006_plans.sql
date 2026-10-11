-- +goose Up
CREATE TABLE founding_members (
    account_id      text PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    granted_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE billing_references (
    account_id      text PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    reference       text NOT NULL UNIQUE,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE subscriptions (
    id                  text PRIMARY KEY,
    account_id          text NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    customer_id         text NOT NULL,
    variant_id          text NOT NULL,
    billing_interval    text NOT NULL CHECK (billing_interval IN ('monthly', 'yearly')),
    status              text NOT NULL CHECK (status IN ('on_trial', 'active', 'paused', 'past_due', 'unpaid', 'cancelled', 'expired')),
    renews_at           timestamptz,
    ends_at             timestamptz,
    trial_ends_at       timestamptz,
    provider_updated_at timestamptz NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX subscriptions_account_idx ON subscriptions (account_id);

CREATE TABLE billing_webhook_events (
    body_hash       bytea PRIMARY KEY,
    event_name      text NOT NULL,
    received_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX billing_webhook_events_received_idx ON billing_webhook_events (received_at);

-- +goose Down
DROP TABLE billing_webhook_events;
DROP TABLE subscriptions;
DROP TABLE billing_references;
DROP TABLE founding_members;
