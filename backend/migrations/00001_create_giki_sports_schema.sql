-- +goose Up
-- Step 4A: Initialize empty giki_sports PostgreSQL schema.
-- Business tables (users, facilities, bookings, etc.) will be added in subsequent migration steps.
CREATE SCHEMA IF NOT EXISTS giki_sports;

-- +goose Down
-- Reverse migration: remove empty giki_sports schema if rolled back.
-- RESTRICT explicitly refuses to remove the schema if it contains any tables or objects,
-- preventing catastrophic accidental data loss.
DROP SCHEMA IF EXISTS giki_sports RESTRICT;

