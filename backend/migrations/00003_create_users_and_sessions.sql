-- +goose Up
-- Step 6A: Create users, user_roles, and server-side sessions tables.
-- Requirements: QR-13 (Argon2id password hash, server-side sessions, 30m idle / 8h absolute expiry, immediate logout invalidation)

CREATE TABLE IF NOT EXISTS giki_sports.users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_users_email_nonempty CHECK (length(trim(email)) > 0),
    CONSTRAINT chk_users_display_name_nonempty CHECK (length(trim(display_name)) > 0),
    CONSTRAINT chk_users_password_hash_nonempty CHECK (length(trim(password_hash)) > 0),
    CONSTRAINT chk_users_status_valid CHECK (status IN ('active', 'inactive'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_email_lower ON giki_sports.users (LOWER(TRIM(email)));

CREATE TABLE IF NOT EXISTS giki_sports.user_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    role VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_user_roles_user FOREIGN KEY (user_id) REFERENCES giki_sports.users(id) ON DELETE RESTRICT,
    CONSTRAINT uq_user_roles_user_role UNIQUE (user_id, role),
    CONSTRAINT chk_user_roles_role_valid CHECK (role IN ('student', 'faculty', 'sports_director', 'super_admin'))
);

CREATE TABLE IF NOT EXISTS giki_sports.sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    token_hash VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES giki_sports.users(id) ON DELETE RESTRICT,
    CONSTRAINT uq_sessions_token_hash UNIQUE (token_hash),
    CONSTRAINT chk_sessions_token_hash_nonempty CHECK (length(trim(token_hash)) > 0),
    CONSTRAINT chk_sessions_activity_after_created CHECK (last_activity_at >= created_at),
    CONSTRAINT chk_sessions_expiry_after_created CHECK (absolute_expires_at > created_at),
    CONSTRAINT chk_sessions_revoked_after_created CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON giki_sports.sessions(user_id);

-- +goose Down
-- Reverse migration for Step 6A: drop tables in reverse dependency order using RESTRICT.
DROP TABLE IF EXISTS giki_sports.sessions RESTRICT;
DROP TABLE IF EXISTS giki_sports.user_roles RESTRICT;
DROP TABLE IF EXISTS giki_sports.users RESTRICT;
