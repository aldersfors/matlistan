-- +goose Up
CREATE TABLE auth_events (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    at           timestamptz NOT NULL,
    subject      text        NOT NULL DEFAULT '',
    email        text        NOT NULL DEFAULT '',
    outcome      text        NOT NULL CHECK (outcome IN ('login', 'denied', 'error')),
    claim_values text[]      NOT NULL DEFAULT '{}',
    detail       text        NOT NULL DEFAULT ''
);
CREATE INDEX auth_events_at ON auth_events (at);

-- +goose Down
DROP TABLE auth_events;
