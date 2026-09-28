-- +goose Up
-- matlistan_slug is keys.Slug in SQL: lowercase, accents folded, words joined by one
-- hyphen, at most 52 characters. TestSQLSlugMatchesGo keeps the two in step. Capitals are
-- folded by translate before lower(): under the C locale (CNPG's default) lower() changes
-- only ASCII letters.
-- +goose StatementBegin
CREATE FUNCTION matlistan_slug(s text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
  SELECT rtrim(left(trim(both '-' from regexp_replace(
    lower(translate(s,
      'ÅÄÀÁÂÖØÓÒÔÉÈÊËÜÚÛÍÎÇÑåäàáâöøóòôéèêëüúûíîçñ',
      'aaaaaoooooeeeeuuuiicnaaaaaoooooeeeeuuuiicn')),
    '[^a-z0-9]+', '-', 'g')), 52), '-')
$$;
-- +goose StatementEnd

ALTER TABLE members ADD COLUMN key text,
    ADD COLUMN managed text NOT NULL DEFAULT '' CHECK (managed IN ('', 'inline'));
ALTER TABLE recipes ADD COLUMN key text,
    ADD COLUMN managed text NOT NULL DEFAULT '' CHECK (managed IN ('', 'inline', 'url'));

-- Existing rows get a key from their name or title, the way the app makes new ones: the
-- first row of each slug, by id, keeps it; the others take the next free -2, -3, ... So a
-- natural "pasta-2" (from "Pasta 2") is never taken from its row.
-- +goose StatementBegin
DO $$
DECLARE
    t   text;
    r   record;
    k   text;
    n   int;
    free boolean;
BEGIN
    FOREACH t IN ARRAY ARRAY['members', 'recipes'] LOOP
        EXECUTE format($q$
            UPDATE %1$I x SET key = b.base FROM (
                SELECT DISTINCT ON (base) id, base FROM (
                    SELECT id, coalesce(nullif(matlistan_slug(%2$I), ''), %3$L) AS base
                    FROM %1$I) s
                ORDER BY base, id) b
            WHERE x.id = b.id$q$, t,
            CASE t WHEN 'members' THEN 'name' ELSE 'title' END,
            CASE t WHEN 'members' THEN 'member' ELSE 'recipe' END);
        FOR r IN EXECUTE format($q$
            SELECT id, coalesce(nullif(matlistan_slug(%2$I), ''), %3$L) AS base
            FROM %1$I WHERE key IS NULL ORDER BY id$q$, t,
            CASE t WHEN 'members' THEN 'name' ELSE 'title' END,
            CASE t WHEN 'members' THEN 'member' ELSE 'recipe' END)
        LOOP
            n := 2;
            LOOP
                k := r.base || '-' || n;
                EXECUTE format('SELECT NOT EXISTS (SELECT 1 FROM %I WHERE key = $1)', t)
                    INTO free USING k;
                EXIT WHEN free;
                n := n + 1;
            END LOOP;
            EXECUTE format('UPDATE %I SET key = $1 WHERE id = $2', t) USING k, r.id;
        END LOOP;
    END LOOP;
END
$$;
-- +goose StatementEnd

ALTER TABLE members ALTER COLUMN key SET NOT NULL, ADD CONSTRAINT members_key UNIQUE (key);
ALTER TABLE recipes ALTER COLUMN key SET NOT NULL, ADD CONSTRAINT recipes_key UNIQUE (key);

-- +goose Down
ALTER TABLE recipes DROP COLUMN managed, DROP COLUMN key;
ALTER TABLE members DROP COLUMN managed, DROP COLUMN key;
DROP FUNCTION matlistan_slug(text);
