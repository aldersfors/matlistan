-- +goose Up
-- A dinner the family keeps while the rest of the draft week is planned again.
ALTER TABLE plan_entries ADD COLUMN locked boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE plan_entries DROP COLUMN locked;
