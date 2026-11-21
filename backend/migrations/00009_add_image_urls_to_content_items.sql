-- +goose Up
-- Add optional image_urls array to content_items for sports achievements and announcements media.
ALTER TABLE giki_sports.content_items
    ADD COLUMN IF NOT EXISTS image_urls TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
-- Remove image_urls column from content_items.
ALTER TABLE giki_sports.content_items
    DROP COLUMN IF EXISTS image_urls;
