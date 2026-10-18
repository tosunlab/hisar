-- The settings an admin saves from the dashboard, one JSON document in one row; HISAR_* variables override it (DESIGN §8.3).

-- +goose Up
CREATE TABLE instance_settings (
    id          smallint PRIMARY KEY CHECK (id = 1),
    settings    jsonb NOT NULL,
    updated_at  timestamptz,
    updated_by  text
);

-- +goose Down
DROP TABLE instance_settings;
