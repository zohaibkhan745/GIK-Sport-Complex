-- +goose Up
-- Phase 7 / Step 7A: Create 30-day gym membership management schema (FR-04, QR-06, CR-06).
-- Tables:
-- 1. giki_sports.gym_memberships: Models active and historical 30-day gym membership passes.
-- Invariants:
-- - Strict 30-day term: [T_start, T_start + 30 days) (QR-06).
-- - Activation strictly linked to successful double-entry wallet debit (FR-04).
-- - Replay protection: UNIQUE (user_id, idempotency_key) guarantees safe checkout replay without duplicate debits (QR-04).
-- - Foreign key ON DELETE RESTRICT guards users and wallet transactions from orphan deletion.

CREATE TABLE IF NOT EXISTS giki_sports.gym_memberships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    plan_type VARCHAR(32) NOT NULL DEFAULT 'STANDARD_30_DAY',
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    price_paisa BIGINT NOT NULL,
    debit_transaction_id UUID NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_gym_memberships_user FOREIGN KEY (user_id) REFERENCES giki_sports.users(id) ON DELETE RESTRICT,
    CONSTRAINT fk_gym_memberships_debit_tx FOREIGN KEY (debit_transaction_id) REFERENCES giki_sports.wallet_transactions(id) ON DELETE RESTRICT,
    CONSTRAINT chk_gym_memberships_period CHECK (start_time < end_time),
    CONSTRAINT chk_gym_memberships_price CHECK (price_paisa >= 0),
    CONSTRAINT chk_gym_memberships_status CHECK (status IN ('ACTIVE', 'EXPIRED', 'CANCELLED')),
    CONSTRAINT chk_gym_memberships_plan CHECK (plan_type IN ('STANDARD_30_DAY')),
    CONSTRAINT chk_gym_memberships_updated_after_created CHECK (updated_at >= created_at),
    CONSTRAINT chk_gym_memberships_idempotency_nonempty CHECK (length(trim(idempotency_key)) > 0),
    CONSTRAINT uq_gym_memberships_user_idempotency UNIQUE (user_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_gym_memberships_user_status_end
    ON giki_sports.gym_memberships (user_id, status, end_time DESC);

CREATE INDEX IF NOT EXISTS idx_gym_memberships_status_end
    ON giki_sports.gym_memberships (status, end_time);

CREATE INDEX IF NOT EXISTS idx_gym_memberships_debit_tx
    ON giki_sports.gym_memberships (debit_transaction_id);

-- +goose Down
DROP TABLE IF EXISTS giki_sports.gym_memberships CASCADE;
