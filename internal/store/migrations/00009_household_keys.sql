-- +goose Up
-- matlistan_slug is keys.Slug in SQL: lowercase, accents folded, words joined by one
-- hyphen, at most 52 characters. TestSQLSlugMatchesGo keeps the two in step.
-- +goose StatementBegin
CREATE FUNCTION matlistan_slug(s text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
  SELECT rtrim(left(trim(both '-' from regexp_replace(
    translate(lower(s), 'åäàáâöøóòôéèêëüúûíîçñ', 'aaaaaoooooeeeeuuuiicn'),
    '[^a-z0-9]+', '-', 'g')), 52), '-')
$$;
-- +goose StatementEnd

ALTER TABLE members ADD COLUMN key text,
    ADD COLUMN managed text NOT NULL DEFAULT '' CHECK (managed IN ('', 'inline'));
ALTER TABLE recipes ADD COLUMN key text,
    ADD COLUMN managed text NOT NULL DEFAULT '' CHECK (managed IN ('', 'inline', 'url'));

-- Existing rows get a key from their name or title; a repeated one gets its id appended,
-- which cannot collide with another backfilled key.
WITH k AS (
    SELECT id, coalesce(nullif(matlistan_slug(name), ''), 'member') AS base FROM members
), n AS (
    SELECT id, base, row_number() OVER (PARTITION BY base ORDER BY id) AS rn FROM k
)
UPDATE members m SET key = CASE WHEN n.rn = 1 THEN n.base ELSE n.base || '-' || m.id END
FROM n WHERE m.id = n.id;

WITH k AS (
    SELECT id, coalesce(nullif(matlistan_slug(title), ''), 'recipe') AS base FROM recipes
), n AS (
    SELECT id, base, row_number() OVER (PARTITION BY base ORDER BY id) AS rn FROM k
)
UPDATE recipes r SET key = CASE WHEN n.rn = 1 THEN n.base ELSE n.base || '-' || r.id END
FROM n WHERE r.id = n.id;

ALTER TABLE members ALTER COLUMN key SET NOT NULL, ADD CONSTRAINT members_key UNIQUE (key);
ALTER TABLE recipes ALTER COLUMN key SET NOT NULL, ADD CONSTRAINT recipes_key UNIQUE (key);

-- +goose Down
ALTER TABLE recipes DROP COLUMN managed, DROP COLUMN key;
ALTER TABLE members DROP COLUMN managed, DROP COLUMN key;
DROP FUNCTION matlistan_slug(text);
