-- +goose Up
CREATE TABLE push_subscriptions (
    id            bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_subject text        NOT NULL,
    endpoint      text        NOT NULL UNIQUE CHECK (char_length(endpoint) <= 1000),
    p256dh        bytea       NOT NULL CHECK (octet_length(p256dh) = 65),
    auth          bytea       NOT NULL CHECK (octet_length(auth) = 16),
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_sent_at  timestamptz
);

-- +goose Down
DROP TABLE push_subscriptions;
