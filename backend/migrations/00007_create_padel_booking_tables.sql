-- +goose Up
-- Step 12B: Create Padel court slots inventory and booking reservations schema (FR-03, FR-06, QR-01, QR-03, QR-04).
-- Tables:
-- 1. giki_sports.padel_slots: Inventory table modeling court time slots, supporting default 60-min slots
--    or custom admin durations. Includes temporary 5-minute hold fields (holder_id, hold_expires_at) and price in integer paisa.
-- 2. giki_sports.padel_bookings: Confirmed booking reservations tied to slots, users, and wallet ledger transactions.
-- Concurrency & Integrity Invariants:
-- - Partial unique index uq_active_slot_booking on padel_bookings (slot_id) WHERE status IN ('CONFIRMED', 'CHECKED_IN')
--   physically prevents duplicate active bookings for the same slot (QR-01), while permitting historical CANCELLED records.
-- - Unique constraint uq_padel_bookings_user_idempotency on (user_id, idempotency_key) guarantees safe replay without duplicate debits (QR-04).
-- - Restrictive foreign keys ensure slots cannot be deleted while referenced, and wallets/users cannot be dropped under active bookings.

CREATE TABLE IF NOT EXISTS giki_sports.padel_slots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    facility_id UUID NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'AVAILABLE',
    holder_id UUID,
    hold_expires_at TIMESTAMPTZ,
    price_paisa BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_padel_slots_facility FOREIGN KEY (facility_id) REFERENCES giki_sports.facilities(id) ON DELETE RESTRICT,
    CONSTRAINT fk_padel_slots_holder FOREIGN KEY (holder_id) REFERENCES giki_sports.users(id) ON DELETE SET NULL,
    CONSTRAINT chk_padel_slots_time_valid CHECK (start_time < end_time),
    CONSTRAINT chk_padel_slots_price_non_negative CHECK (price_paisa >= 0),
    CONSTRAINT chk_padel_slots_status CHECK (status IN ('AVAILABLE', 'HELD', 'BOOKED', 'BLOCKED')),
    CONSTRAINT chk_padel_slots_hold_consistency CHECK (
        (status = 'HELD' AND holder_id IS NOT NULL AND hold_expires_at IS NOT NULL) OR
        (status != 'HELD')
    ),
    CONSTRAINT chk_padel_slots_updated_after_created CHECK (updated_at >= created_at),
    CONSTRAINT uq_padel_slots_facility_start_time UNIQUE (facility_id, start_time)
);

CREATE INDEX IF NOT EXISTS idx_padel_slots_facility_start
    ON giki_sports.padel_slots (facility_id, start_time);

CREATE INDEX IF NOT EXISTS idx_padel_slots_held_expiry
    ON giki_sports.padel_slots (status, hold_expires_at)
    WHERE status = 'HELD';

CREATE INDEX IF NOT EXISTS idx_padel_slots_status
    ON giki_sports.padel_slots (status);

CREATE TABLE IF NOT EXISTS giki_sports.padel_bookings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slot_id UUID NOT NULL,
    user_id UUID NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'CONFIRMED',
    fee_paisa BIGINT NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    check_in_code VARCHAR(16) NOT NULL,
    checked_in_at TIMESTAMPTZ,
    checked_in_by UUID,
    cancelled_at TIMESTAMPTZ,
    cancelled_by UUID,
    cancellation_reason TEXT,
    debit_transaction_id UUID,
    refund_transaction_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_padel_bookings_slot FOREIGN KEY (slot_id) REFERENCES giki_sports.padel_slots(id) ON DELETE RESTRICT,
    CONSTRAINT fk_padel_bookings_user FOREIGN KEY (user_id) REFERENCES giki_sports.users(id) ON DELETE RESTRICT,
    CONSTRAINT fk_padel_bookings_checked_in_by FOREIGN KEY (checked_in_by) REFERENCES giki_sports.users(id) ON DELETE SET NULL,
    CONSTRAINT fk_padel_bookings_cancelled_by FOREIGN KEY (cancelled_by) REFERENCES giki_sports.users(id) ON DELETE SET NULL,
    CONSTRAINT fk_padel_bookings_debit_tx FOREIGN KEY (debit_transaction_id) REFERENCES giki_sports.wallet_transactions(id) ON DELETE RESTRICT,
    CONSTRAINT fk_padel_bookings_refund_tx FOREIGN KEY (refund_transaction_id) REFERENCES giki_sports.wallet_transactions(id) ON DELETE RESTRICT,
    CONSTRAINT chk_padel_bookings_fee_positive CHECK (fee_paisa > 0),
    CONSTRAINT chk_padel_bookings_status CHECK (status IN ('CONFIRMED', 'CHECKED_IN', 'CANCELLED', 'COMPLETED')),
    CONSTRAINT chk_padel_bookings_checkin_consistency CHECK (
        (status = 'CHECKED_IN' AND checked_in_at IS NOT NULL) OR
        (status != 'CHECKED_IN')
    ),
    CONSTRAINT chk_padel_bookings_cancel_consistency CHECK (
        (status = 'CANCELLED' AND cancelled_at IS NOT NULL) OR
        (status != 'CANCELLED')
    ),
    CONSTRAINT chk_padel_bookings_code_nonempty CHECK (length(trim(check_in_code)) > 0),
    CONSTRAINT chk_padel_bookings_idempotency_nonempty CHECK (length(trim(idempotency_key)) > 0),
    CONSTRAINT chk_padel_bookings_updated_after_created CHECK (updated_at >= created_at),
    CONSTRAINT uq_padel_bookings_user_idempotency UNIQUE (user_id, idempotency_key)
);

-- Partial Unique Index: Exactly one active booking per slot (QR-01, double-booking invariant)
-- Allows historical CANCELLED records while preventing concurrent or duplicate active bookings.
CREATE UNIQUE INDEX IF NOT EXISTS uq_active_slot_booking
    ON giki_sports.padel_bookings (slot_id)
    WHERE status IN ('CONFIRMED', 'CHECKED_IN');

CREATE INDEX IF NOT EXISTS idx_padel_bookings_user_status
    ON giki_sports.padel_bookings (user_id, status);

CREATE INDEX IF NOT EXISTS idx_padel_bookings_slot
    ON giki_sports.padel_bookings (slot_id);

CREATE INDEX IF NOT EXISTS idx_padel_bookings_check_in_code
    ON giki_sports.padel_bookings (check_in_code);

-- +goose Down
-- Reverse migration for Step 12B: drop tables and indexes in reverse dependency order using RESTRICT.
DROP TABLE IF EXISTS giki_sports.padel_bookings RESTRICT;
DROP TABLE IF EXISTS giki_sports.padel_slots RESTRICT;
