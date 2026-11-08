-- +goose Up
-- Step 4B: Create core facilities and recurring weekly schedules tables.
-- Interpretation timezone for schedules: Asia/Karachi (PKT, UTC+5).

CREATE TABLE IF NOT EXISTS giki_sports.facilities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(100) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    location VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_facilities_slug UNIQUE (slug),
    CONSTRAINT chk_facilities_slug_nonempty CHECK (length(trim(slug)) > 0),
    CONSTRAINT chk_facilities_name_nonempty CHECK (length(trim(name)) > 0),
    CONSTRAINT chk_facilities_status_valid CHECK (status IN ('active', 'maintenance', 'closed'))
);

CREATE TABLE IF NOT EXISTS giki_sports.schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    facility_id UUID NOT NULL,
    day_of_week SMALLINT NOT NULL,
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    audience VARCHAR(50) NOT NULL DEFAULT 'general',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_schedules_facility FOREIGN KEY (facility_id) REFERENCES giki_sports.facilities(id) ON DELETE RESTRICT,
    CONSTRAINT chk_schedules_day_valid CHECK (day_of_week BETWEEN 1 AND 7),
    CONSTRAINT chk_schedules_time_order CHECK (start_time < end_time),
    CONSTRAINT chk_schedules_audience_valid CHECK (audience IN ('general', 'student', 'faculty', 'women_only', 'staff')),
    CONSTRAINT uq_schedules_window UNIQUE (facility_id, day_of_week, audience, start_time, end_time)
);

CREATE INDEX IF NOT EXISTS idx_schedules_facility_id ON giki_sports.schedules(facility_id);

-- +goose Down
-- Reverse migration for Step 4B: drop tables in reverse dependency order using RESTRICT.
DROP TABLE IF EXISTS giki_sports.schedules RESTRICT;
DROP TABLE IF EXISTS giki_sports.facilities RESTRICT;
