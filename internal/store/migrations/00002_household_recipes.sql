-- +goose Up
CREATE TABLE members (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    birth_year   int         NOT NULL,
    oidc_subject text        UNIQUE,
    diets        text[]      NOT NULL DEFAULT '{}',
    allergens    text[]      NOT NULL DEFAULT '{}',
    likes        text        NOT NULL DEFAULT '' CHECK (char_length(likes) <= 500),
    dislikes     text        NOT NULL DEFAULT '' CHECK (char_length(dislikes) <= 500),
    created_at   timestamptz NOT NULL DEFAULT now(),
    archived_at  timestamptz
);

CREATE TABLE household_settings (
    id                  int PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    dinners_per_week    int NOT NULL DEFAULT 7  CHECK (dinners_per_week BETWEEN 1 AND 7),
    weeknight_minutes   int NOT NULL DEFAULT 45 CHECK (weeknight_minutes BETWEEN 10 AND 240),
    repeat_window_weeks int NOT NULL DEFAULT 6  CHECK (repeat_window_weeks BETWEEN 1 AND 26),
    library_share       int NOT NULL DEFAULT 50 CHECK (library_share BETWEEN 0 AND 100)
);
INSERT INTO household_settings (id) VALUES (1);

CREATE TABLE staples (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name     text   NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    name_key text   NOT NULL UNIQUE
);

CREATE TABLE recipes (
    id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title          text        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
    title_key      text        NOT NULL,
    description    text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 1000),
    lang           text        NOT NULL CHECK (lang IN ('en', 'sv')),
    servings       int         NOT NULL CHECK (servings BETWEEN 1 AND 20),
    active_minutes int         NOT NULL CHECK (active_minutes >= 0),
    total_minutes  int         NOT NULL CHECK (total_minutes BETWEEN 1 AND 1440),
    tags           text[]      NOT NULL DEFAULT '{}',
    steps          text[]      NOT NULL,
    diets          text[]      NOT NULL DEFAULT '{}',
    allergens      text[]      NOT NULL DEFAULT '{}',
    source         text        NOT NULL CHECK (source IN ('manual', 'generated', 'imported')),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    archived_at    timestamptz
);
CREATE INDEX recipes_lang_title ON recipes (lang, title_key) WHERE archived_at IS NULL;

CREATE TABLE recipe_ingredients (
    recipe_id bigint           NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    position  int              NOT NULL,
    name      text             NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    quantity  double precision NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    unit      text             NOT NULL DEFAULT '',
    section   text             NOT NULL,
    optional  boolean          NOT NULL DEFAULT false,
    PRIMARY KEY (recipe_id, position)
);

-- +goose Down
DROP TABLE recipe_ingredients;
DROP TABLE recipes;
DROP TABLE staples;
DROP TABLE household_settings;
DROP TABLE members;
