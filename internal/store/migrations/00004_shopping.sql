-- +goose Up
CREATE TABLE shopping_lists (
    id               bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    plan_id          bigint      NOT NULL UNIQUE REFERENCES week_plans (id) ON DELETE CASCADE,
    excluded_staples int         NOT NULL DEFAULT 0,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE shopping_items (
    id       bigint           GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    list_id  bigint           NOT NULL REFERENCES shopping_lists (id) ON DELETE CASCADE,
    position int              NOT NULL,
    name     text             NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    section  text             NOT NULL,
    quantity double precision NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    unit     text             NOT NULL DEFAULT '',
    days     int[]            NOT NULL DEFAULT '{}',
    optional boolean          NOT NULL DEFAULT false,
    manual   boolean          NOT NULL DEFAULT false,
    checked  boolean          NOT NULL DEFAULT false
);
CREATE INDEX shopping_items_list ON shopping_items (list_id, position);

CREATE TABLE api_tokens (
    id            bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_subject text        NOT NULL,
    name          text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    hash          bytea       NOT NULL UNIQUE,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_used_at  timestamptz
);

-- +goose Down
DROP TABLE api_tokens;
DROP TABLE shopping_items;
DROP TABLE shopping_lists;
