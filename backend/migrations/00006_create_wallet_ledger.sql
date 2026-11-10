-- +goose Up
-- Step 11B: Create digital wallet accounts, transaction headers, and append-only ledger entries (FR-05, QR-03, QR-04).
-- Monetary unit: signed BIGINT paisa (1 PKR = 100 paisa). Balances are derived dynamically from ledger entries.
-- Replay protection: UNIQUE(type, reference_id) on transaction headers.
-- Append-only integrity: Database triggers reject all UPDATE, DELETE, and TRUNCATE operations on transactions and ledger entries.

CREATE TABLE IF NOT EXISTS giki_sports.wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(30) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    currency VARCHAR(10) NOT NULL DEFAULT 'PKR',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_wallets_user FOREIGN KEY (user_id) REFERENCES giki_sports.users(id) ON DELETE RESTRICT,
    CONSTRAINT chk_wallets_type_valid CHECK (type IN ('PERSONAL', 'SYS_REVENUE', 'SYS_LIABILITY')),
    CONSTRAINT chk_wallets_status_valid CHECK (status IN ('ACTIVE', 'FROZEN', 'SUSPENDED', 'CLOSED')),
    CONSTRAINT chk_wallets_ownership CHECK (
        (type = 'PERSONAL' AND user_id IS NOT NULL) OR
        (type IN ('SYS_REVENUE', 'SYS_LIABILITY') AND user_id IS NULL)
    ),
    CONSTRAINT chk_wallets_name_nonempty CHECK (length(trim(name)) > 0),
    CONSTRAINT chk_wallets_updated_after_created CHECK (updated_at >= created_at)
);

-- One personal wallet per Sports user
CREATE UNIQUE INDEX IF NOT EXISTS idx_wallets_user_personal
    ON giki_sports.wallets(user_id)
    WHERE type = 'PERSONAL';

-- Distinct system accounts identified by name and type
CREATE UNIQUE INDEX IF NOT EXISTS idx_wallets_system_name_type
    ON giki_sports.wallets(name, type)
    WHERE user_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_wallets_type_status
    ON giki_sports.wallets(type, status);

CREATE TABLE IF NOT EXISTS giki_sports.wallet_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(50) NOT NULL,
    reference_id VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_wallet_transactions_type_nonempty CHECK (length(trim(type)) > 0),
    CONSTRAINT chk_wallet_transactions_reference_nonempty CHECK (length(trim(reference_id)) > 0),
    CONSTRAINT uq_wallet_transactions_type_reference UNIQUE (type, reference_id)
);

CREATE INDEX IF NOT EXISTS idx_wallet_transactions_created_at
    ON giki_sports.wallet_transactions(created_at DESC);

CREATE TABLE IF NOT EXISTS giki_sports.wallet_ledger_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id UUID NOT NULL,
    transaction_id UUID NOT NULL,
    amount BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT fk_wallet_ledger_entries_wallet FOREIGN KEY (wallet_id) REFERENCES giki_sports.wallets(id) ON DELETE RESTRICT,
    CONSTRAINT fk_wallet_ledger_entries_transaction FOREIGN KEY (transaction_id) REFERENCES giki_sports.wallet_transactions(id) ON DELETE RESTRICT,
    CONSTRAINT chk_wallet_ledger_entries_amount_nonzero CHECK (amount <> 0)
);

CREATE INDEX IF NOT EXISTS idx_wallet_ledger_entries_wallet_created
    ON giki_sports.wallet_ledger_entries(wallet_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_wallet_ledger_entries_transaction_id
    ON giki_sports.wallet_ledger_entries(transaction_id);

-- Append-only trigger function blocking modifications on transaction headers and ledger entries
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION giki_sports.prevent_ledger_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Table % is append-only: UPDATE, DELETE, and TRUNCATE operations are strictly prohibited.', TG_TABLE_NAME
        USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_wallet_transactions_append_only
    BEFORE UPDATE OR DELETE ON giki_sports.wallet_transactions
    FOR EACH ROW EXECUTE FUNCTION giki_sports.prevent_ledger_modification();

CREATE TRIGGER trg_wallet_transactions_truncate_guard
    BEFORE TRUNCATE ON giki_sports.wallet_transactions
    FOR EACH STATEMENT EXECUTE FUNCTION giki_sports.prevent_ledger_modification();

CREATE TRIGGER trg_wallet_ledger_entries_append_only
    BEFORE UPDATE OR DELETE ON giki_sports.wallet_ledger_entries
    FOR EACH ROW EXECUTE FUNCTION giki_sports.prevent_ledger_modification();

CREATE TRIGGER trg_wallet_ledger_entries_truncate_guard
    BEFORE TRUNCATE ON giki_sports.wallet_ledger_entries
    FOR EACH STATEMENT EXECUTE FUNCTION giki_sports.prevent_ledger_modification();

-- +goose Down
-- Reverse migration for Step 11B: drop triggers, trigger function, and tables in reverse dependency order using RESTRICT.
DROP TRIGGER IF EXISTS trg_wallet_ledger_entries_truncate_guard ON giki_sports.wallet_ledger_entries;
DROP TRIGGER IF EXISTS trg_wallet_ledger_entries_append_only ON giki_sports.wallet_ledger_entries;
DROP TRIGGER IF EXISTS trg_wallet_transactions_truncate_guard ON giki_sports.wallet_transactions;
DROP TRIGGER IF EXISTS trg_wallet_transactions_append_only ON giki_sports.wallet_transactions;
DROP FUNCTION IF EXISTS giki_sports.prevent_ledger_modification();

DROP TABLE IF EXISTS giki_sports.wallet_ledger_entries RESTRICT;
DROP TABLE IF EXISTS giki_sports.wallet_transactions RESTRICT;
DROP TABLE IF EXISTS giki_sports.wallets RESTRICT;
