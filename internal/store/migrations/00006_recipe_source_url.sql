-- +goose Up
ALTER TABLE recipes ADD COLUMN source_url text
    CHECK (source_url IS NULL OR (length(source_url) <= 2000 AND source_url LIKE 'https://%'));
CREATE UNIQUE INDEX recipes_source_url ON recipes (source_url) WHERE source_url IS NOT NULL;

-- +goose Down
DROP INDEX recipes_source_url;
ALTER TABLE recipes DROP COLUMN source_url;
