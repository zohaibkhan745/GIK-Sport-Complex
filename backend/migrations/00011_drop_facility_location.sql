-- +goose Up
-- Step 11: Drop redundant location column from facilities table.
-- All campus sports facilities are housed within or directly adjacent to the single-story GIKI Sports Complex building.
ALTER TABLE giki_sports.facilities
    DROP COLUMN IF EXISTS location;

-- +goose Down
-- Reverse migration: re-add nullable location column.
ALTER TABLE giki_sports.facilities
    ADD COLUMN IF NOT EXISTS location VARCHAR(255);
