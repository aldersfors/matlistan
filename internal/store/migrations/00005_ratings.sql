-- +goose Up
CREATE TABLE ratings (
    plan_id   bigint      NOT NULL REFERENCES week_plans (id) ON DELETE CASCADE,
    day       int         NOT NULL CHECK (day BETWEEN 1 AND 7),
    member_id bigint      NOT NULL REFERENCES members (id),
    recipe_id bigint      NOT NULL REFERENCES recipes (id),
    score     int         NOT NULL CHECK (score IN (1, 3, 5)),
    rated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (plan_id, day, member_id)
);
CREATE INDEX ratings_recipe ON ratings (recipe_id);

-- +goose Down
DROP TABLE ratings;
