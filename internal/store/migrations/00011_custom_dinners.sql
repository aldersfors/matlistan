-- +goose Up
-- A dinner is a recipe or a line of text the family typed, such as "Hämtmat".
ALTER TABLE plan_entries ALTER COLUMN recipe_id DROP NOT NULL;
ALTER TABLE plan_entries ADD COLUMN title text NOT NULL DEFAULT ''
    CHECK (char_length(title) <= 60);
ALTER TABLE plan_entries ADD CONSTRAINT plan_entries_recipe_or_title
    CHECK ((recipe_id IS NULL) <> (title = ''));

-- +goose Down
DELETE FROM plan_entries WHERE recipe_id IS NULL;
ALTER TABLE plan_entries DROP CONSTRAINT plan_entries_recipe_or_title;
ALTER TABLE plan_entries DROP COLUMN title;
ALTER TABLE plan_entries ALTER COLUMN recipe_id SET NOT NULL;
