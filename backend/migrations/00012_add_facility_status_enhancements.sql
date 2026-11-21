-- +goose Up
-- Step 12: Add 'reserved' status and time-bound status metadata (status_until, status_note, reserved_for).
ALTER TABLE giki_sports.facilities
    DROP CONSTRAINT IF EXISTS chk_facilities_status_valid;

ALTER TABLE giki_sports.facilities
    ADD CONSTRAINT chk_facilities_status_valid CHECK (status IN ('active', 'maintenance', 'closed', 'reserved'));

ALTER TABLE giki_sports.facilities
    ADD COLUMN IF NOT EXISTS status_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS status_note TEXT,
    ADD COLUMN IF NOT EXISTS reserved_for VARCHAR(255);

-- +goose Down
ALTER TABLE giki_sports.facilities
    DROP COLUMN IF EXISTS status_until,
    DROP COLUMN IF EXISTS status_note,
    DROP COLUMN IF EXISTS reserved_for;

ALTER TABLE giki_sports.facilities
    DROP CONSTRAINT IF EXISTS chk_facilities_status_valid;

ALTER TABLE giki_sports.facilities
    ADD CONSTRAINT chk_facilities_status_valid CHECK (status IN ('active', 'maintenance', 'closed'));
