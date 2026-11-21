-- +goose Up
-- Step 13: Create configurable gym membership plans table and relax hardcoded plan_type constraint.

CREATE TABLE IF NOT EXISTS giki_sports.gym_membership_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_code VARCHAR(32) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    duration_days INT NOT NULL CHECK (duration_days > 0),
    price_paisa BIGINT NOT NULL CHECK (price_paisa >= 0),
    is_active BOOLEAN NOT NULL DEFAULT true,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by UUID REFERENCES giki_sports.users(id) ON DELETE SET NULL
);

-- Seed default standard plan
INSERT INTO giki_sports.gym_membership_plans (plan_code, name, duration_days, price_paisa, is_active, description)
VALUES (
    'STANDARD_30_DAY',
    'Standard Gym Membership',
    30,
    300000,
    true,
    'Olympic racks, specialized free weights, cardio machines, and functional rigs.'
)
ON CONFLICT (plan_code) DO NOTHING;

-- Relax static check constraint on gym_memberships table
ALTER TABLE giki_sports.gym_memberships
    DROP CONSTRAINT IF EXISTS chk_gym_memberships_plan;

-- Link plan_type to gym_membership_plans
ALTER TABLE giki_sports.gym_memberships
    ADD CONSTRAINT fk_gym_memberships_plan
    FOREIGN KEY (plan_type) REFERENCES giki_sports.gym_membership_plans(plan_code)
    ON UPDATE CASCADE ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE giki_sports.gym_memberships
    DROP CONSTRAINT IF EXISTS fk_gym_memberships_plan;

ALTER TABLE giki_sports.gym_memberships
    ADD CONSTRAINT chk_gym_memberships_plan CHECK (plan_type IN ('STANDARD_30_DAY'));

DROP TABLE IF EXISTS giki_sports.gym_membership_plans CASCADE;
