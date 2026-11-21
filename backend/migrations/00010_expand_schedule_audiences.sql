-- +goose Up
-- Expand schedule audiences to support official GIKI varsity sports teams and dedicated cohorts.
ALTER TABLE giki_sports.schedules
    DROP CONSTRAINT IF EXISTS chk_schedules_audience_valid;

ALTER TABLE giki_sports.schedules
    ALTER COLUMN audience TYPE VARCHAR(100);

ALTER TABLE giki_sports.schedules
    ADD CONSTRAINT chk_schedules_audience_nonempty CHECK (length(trim(audience)) > 0);

-- +goose Down
-- Revert audience check constraint to initial standard set.
ALTER TABLE giki_sports.schedules
    DROP CONSTRAINT IF EXISTS chk_schedules_audience_nonempty;

ALTER TABLE giki_sports.schedules
    ADD CONSTRAINT chk_schedules_audience_valid CHECK (audience IN ('general', 'student', 'faculty', 'women_only', 'staff'));
