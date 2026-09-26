-- +goose Up
CREATE TABLE week_plans (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    iso_year     int         NOT NULL,
    iso_week     int         NOT NULL CHECK (iso_week BETWEEN 1 AND 53),
    status       text        NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'approved')),
    context      jsonb       NOT NULL,
    error        text        NOT NULL DEFAULT '',
    generated_at timestamptz,
    approved_at  timestamptz,
    approved_by  text        NOT NULL DEFAULT '',
    UNIQUE (iso_year, iso_week)
);

CREATE TABLE plan_entries (
    plan_id   bigint NOT NULL REFERENCES week_plans (id) ON DELETE CASCADE,
    day       int    NOT NULL CHECK (day BETWEEN 1 AND 7),
    recipe_id bigint NOT NULL REFERENCES recipes (id),
    servings  int    NOT NULL CHECK (servings BETWEEN 1 AND 20),
    why       text   NOT NULL DEFAULT '' CHECK (char_length(why) <= 300),
    PRIMARY KEY (plan_id, day)
);

-- +goose Down
DROP TABLE plan_entries;
DROP TABLE week_plans;
