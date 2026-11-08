-- +goose Up
-- Step 7C: Create administrative audit trail table for system and facility mutations (FR-09).
-- Interpretation timezone for timestamps: Asia/Karachi (PKT, UTC+5).

CREATE TABLE IF NOT EXISTS giki_sports.audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id UUID NOT NULL,
    action VARCHAR(50) NOT NULL,
    target_type VARCHAR(50) NOT NULL,
    target_id UUID NOT NULL,
    details JSONB NOT NULL DEFAULT '{}',
    ip_address VARCHAR(45),
    user_agent TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'SUCCESS',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_audit_logs_actor FOREIGN KEY (actor_id) REFERENCES giki_sports.users(id) ON DELETE RESTRICT,
    CONSTRAINT chk_audit_logs_action_nonempty CHECK (length(trim(action)) > 0),
    CONSTRAINT chk_audit_logs_target_type_nonempty CHECK (length(trim(target_type)) > 0),
    CONSTRAINT chk_audit_logs_status_valid CHECK (status IN ('SUCCESS', 'FAILURE'))
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_id ON giki_sports.audit_logs(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_target_id ON giki_sports.audit_logs(target_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON giki_sports.audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON giki_sports.audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_target_action ON giki_sports.audit_logs(target_id, action, created_at DESC);

-- +goose Down
-- Reverse migration for Step 7C: drop audit logs table using RESTRICT.
DROP TABLE IF EXISTS giki_sports.audit_logs RESTRICT;

