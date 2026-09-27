-- +goose Up
-- An archived import must not block importing the same page again.
DROP INDEX recipes_source_url;
CREATE UNIQUE INDEX recipes_source_url ON recipes (source_url)
    WHERE source_url IS NOT NULL AND archived_at IS NULL;

-- +goose Down
DROP INDEX recipes_source_url;
CREATE UNIQUE INDEX recipes_source_url ON recipes (source_url) WHERE source_url IS NOT NULL;
