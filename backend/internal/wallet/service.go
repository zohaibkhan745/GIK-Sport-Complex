package wallet

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// queryable abstracts query operations for either *pgxpool.Pool or pgx.Tx.
type queryable interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Service provides domain operations for managing Sports digital wallets and atomic double-entry ledger postings.
//
// SCHEMA BOUNDARY & ENFORCEMENT NOTE:
// The PostgreSQL database schema defines tables, foreign keys, non-zero amount checks, and append-only triggers.
// However, ordinary database row constraints do not prove that a transaction has both entries, sums to zero, or
// cannot acquire extra entries later. Direct SQL inserts from outside this service could insert single or unbalanced entries.
// It is the PostTransaction domain operation that strictly enforces:
// 1. Two-entry paired balancing: exactly one debit leg (-amount) and one credit leg (+amount).
// 2. Strict zero-sum invariant: (-amount) + (+amount) = 0.
// 3. Atomicity: transaction header and both ledger entries succeed or fail together inside a single pgx.Tx.
// 4. Overdraft prevention: pessimistic row locking (FOR UPDATE) in stable UUID order verifying sender balance >= amount.
// 5. Idempotent replay protection: distinguishing identical transfer replays from parameter conflicts before funds checks.
type Service struct {
	pool                   *pgxpool.Pool
	failSecondEntryForTest bool
	beforeCommitHook       func(ctx context.Context)
}

// NewService constructs a new Service instance backed by the provided PostgreSQL connection pool.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool: pool,
	}
}

// SetFailSecondEntryForTest enables or disables simulated failure on the second ledger entry.
// Used exclusively in automated tests to verify atomic rollback behavior.
func (s *Service) SetFailSecondEntryForTest(fail bool) {
	s.failSecondEntryForTest = fail
}

// SetBeforeCommitHook registers a callback invoked right before tx.Commit.
// Used in automated tests to synchronize concurrent race condition verifications without sleeps.
func (s *Service) SetBeforeCommitHook(fn func(ctx context.Context)) {
	s.beforeCommitHook = fn
}

// PostTransaction executes an atomic double-entry transfer between two active wallets within a single PostgreSQL transaction.
//
// Rules enforced:
// PostTransaction executes a two-entry transfer between two wallets within a single PostgreSQL transaction.
// It is a backward-compatible wrapper around PostTransactionWithResult.
func (s *Service) PostTransaction(ctx context.Context, params TransferParams) (uuid.UUID, error) {
	res, err := s.PostTransactionWithResult(ctx, params)
	if err != nil {
		return uuid.Nil, err
	}
	return res.TransactionID, nil
}

// PostTransactionWithResult executes a two-entry transfer between two wallets within a single PostgreSQL transaction,
// returning a PostResult with the transaction ID and an AlreadyPosted boolean indicating whether the transfer
// was freshly created or matched an existing idempotent transaction header.
//
// Invariants enforced:
// 1. Validates inputs: amount must be positive integer paisa; sender != receiver; type and reference non-empty.
// 2. Replay check: pre-flight check on committed references allows sequential safe replays to return immediately.
// 3. Row locking: locks both wallets with SELECT ... FOR UPDATE in deterministic lexicographical UUID order to
//    eliminate opposite-order wallet lock cycles (AB/BA deadlocks on the wallets table).
// 4. Account verification: both wallets must exist, have status = 'ACTIVE', and share the same currency.
// 5. Unique reference resolution before funds check: executes INSERT ... ON CONFLICT (type, reference_id) DO NOTHING.
//    If another concurrent transaction has already claimed this reference, PostgreSQL blocks on the unique index until
//    that transaction completes. Upon commit, the losing request receives 0 rows without aborting the transaction block,
//    reads the winner's committed transfer, and returns either the original transaction ID (if identical) or
//    ErrTransactionConflict (if parameters differ), even if the sender currently has zero balance.
// 6. Balance derivation: for new references, sender's balance is dynamically derived inside the locked transaction
//    as SUM(amount) across historical ledger entries. If a personal wallet lacks funds, tx is rolled back, leaving
//    no header or entries in the database.
// 7. Paired entries: inserts one negative debit entry for the sender (-amount) and one positive credit entry for receiver (+amount).
// 8. Atomic commit: rolls back the entire transaction if any query fails. Returns transaction ID only after commit.
func (s *Service) PostTransactionWithResult(ctx context.Context, params TransferParams) (*PostResult, error) {
	// 1. Validate transfer parameters
	if params.SenderWalletID == uuid.Nil || params.ReceiverWalletID == uuid.Nil {
		return nil, ErrWalletNotFound
	}
	if params.SenderWalletID == params.ReceiverWalletID {
		return nil, ErrSelfTransfer
	}
	if params.Amount <= 0 {
		return nil, ErrInvalidAmount
	}
	params.Type = strings.TrimSpace(params.Type)
	if params.Type == "" {
		return nil, ErrEmptyType
	}
	params.ReferenceID = strings.TrimSpace(params.ReferenceID)
	if params.ReferenceID == "" {
		return nil, ErrEmptyReference
	}

	// 2. Pre-flight replay check: allows sequential replays to return early outside a transaction
	existingID, isMatch, err := s.checkExistingTransaction(ctx, s.pool, params)
	if err != nil {
		return nil, err
	}
	if existingID != uuid.Nil {
		if isMatch {
			return &PostResult{TransactionID: existingID, AlreadyPosted: true}, nil // Safe idempotent replay: return original transaction ID
		}
		return nil, ErrTransactionConflict // Conflicting transfer parameters for same reference
	}

	// 3. Begin single atomic PostgreSQL transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin database transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// 4. Determine deterministic locking order to eliminate opposite-order wallet lock cycles (AB/BA deadlocks on the wallets table)
	firstID, secondID := params.SenderWalletID, params.ReceiverWalletID
	if strings.Compare(firstID.String(), secondID.String()) > 0 {
		firstID, secondID = secondID, firstID
	}

	// 5. Lock both wallet rows in stable UUID order
	firstWallet, err := s.lockWallet(ctx, tx, firstID)
	if err != nil {
		return nil, err
	}
	secondWallet, err := s.lockWallet(ctx, tx, secondID)
	if err != nil {
		return nil, err
	}

	var senderWallet, receiverWallet *Wallet
	if firstWallet.ID == params.SenderWalletID {
		senderWallet = firstWallet
		receiverWallet = secondWallet
	} else {
		senderWallet = secondWallet
		receiverWallet = firstWallet
	}

	// 6. Verify wallet lifecycle status and currency compatibility
	if senderWallet.Status != WalletStatusActive || receiverWallet.Status != WalletStatusActive {
		return nil, ErrWalletInactive
	}
	if senderWallet.Currency != receiverWallet.Currency {
		return nil, ErrCurrencyMismatch
	}

	// 7. Resolve the transaction reference inside the transaction BEFORE checking funds.
	// Using INSERT ... ON CONFLICT (type, reference_id) DO NOTHING RETURNING id ensures that:
	// a) If the reference is brand new: it inserts the row and returns the new txID without committing yet.
	// b) If the reference is in-flight in another concurrent transaction: PostgreSQL automatically blocks
	//    on the unique index until that competing transaction commits or rolls back.
	// c) If the reference is already committed (or just committed while we waited): it does nothing, does NOT
	//    abort the transaction block, and returns 0 rows (pgx.ErrNoRows).
	var txID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO giki_sports.wallet_transactions (type, reference_id, description)
		VALUES ($1, $2, $3)
		ON CONFLICT (type, reference_id) DO NOTHING
		RETURNING id;
	`, params.Type, params.ReferenceID, params.Description).Scan(&txID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// A transaction with this (type, reference_id) has already been committed.
			// Inspect the winner's committed transaction and ledger entries inside this valid transaction.
			existingID, isMatch, checkErr := s.checkExistingTransaction(ctx, tx, params)
			if checkErr != nil {
				return nil, checkErr
			}
			if existingID != uuid.Nil {
				if isMatch {
					// Identical transfer: safe idempotent replay!
					// Returns original ID without another debit, even if the sender's balance is now zero.
					return &PostResult{TransactionID: existingID, AlreadyPosted: true}, nil
				}
				// Same reference with different amount or wallet: distinct conflict error.
				return nil, ErrTransactionConflict
			}
			return nil, ErrTransactionConflict
		}
		return nil, fmt.Errorf("failed to insert transaction header: %w", err)
	}

	// 8. Derive sender balance dynamically from ledger entries inside the locked transaction.
	// If the sender is personal and has insufficient funds, tx is rolled back, cleanly leaving
	// no header or entries in the database.
	if senderWallet.Type == WalletTypePersonal {
		var senderBalance int64
		err := tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(amount), 0)
			FROM giki_sports.wallet_ledger_entries
			WHERE wallet_id = $1;
		`, senderWallet.ID).Scan(&senderBalance)
		if err != nil {
			return nil, fmt.Errorf("failed to derive sender balance: %w", err)
		}

		if senderBalance < params.Amount {
			return nil, ErrInsufficientFunds
		}
	}

	// 9. Insert exactly two paired entries: debit (-amount) for sender, credit (+amount) for receiver
	// Sender Entry (Debit)
	_, err = tx.Exec(ctx, `
		INSERT INTO giki_sports.wallet_ledger_entries (wallet_id, transaction_id, amount)
		VALUES ($1, $2, $3);
	`, params.SenderWalletID, txID, -params.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to insert sender ledger entry: %w", err)
	}

	// Fault injection hook for automated atomicity test
	if s.failSecondEntryForTest {
		// Force failure by inserting an invalid entry with amount = 0, violating chk_wallet_ledger_entries_amount_nonzero
		_, err = tx.Exec(ctx, `
			INSERT INTO giki_sports.wallet_ledger_entries (wallet_id, transaction_id, amount)
			VALUES ($1, $2, 0);
		`, params.ReceiverWalletID, txID)
		if err != nil {
			return nil, fmt.Errorf("simulated failure on second entry: %w", err)
		}
	}

	// Receiver Entry (Credit)
	_, err = tx.Exec(ctx, `
		INSERT INTO giki_sports.wallet_ledger_entries (wallet_id, transaction_id, amount)
		VALUES ($1, $2, $3);
	`, params.ReceiverWalletID, txID, params.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to insert receiver ledger entry: %w", err)
	}

	// Synchronization hook for deterministic concurrent test verification
	if s.beforeCommitHook != nil {
		s.beforeCommitHook(ctx)
	}

	// 10. Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Return transaction ID only after commit
	return &PostResult{TransactionID: txID, AlreadyPosted: false}, nil
}

// lockWallet locks a single wallet row with FOR UPDATE and returns its current model attributes.
func (s *Service) lockWallet(ctx context.Context, tx pgx.Tx, walletID uuid.UUID) (*Wallet, error) {
	var w Wallet
	var uid *uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT id, user_id, name, type, status, currency, created_at, updated_at
		FROM giki_sports.wallets
		WHERE id = $1
		FOR UPDATE;
	`, walletID).Scan(&w.ID, &uid, &w.Name, &w.Type, &w.Status, &w.Currency, &w.CreatedAt, &w.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWalletNotFound
		}
		return nil, fmt.Errorf("failed to lock wallet %s: %w", walletID, err)
	}
	w.UserID = uid
	return &w, nil
}

// checkExistingTransaction queries committed state via the provided queryable (pool or tx) to determine
// whether a transaction with the same (type, reference_id) exists, and whether its recorded entries
// match the supplied TransferParams.
func (s *Service) checkExistingTransaction(ctx context.Context, q queryable, params TransferParams) (uuid.UUID, bool, error) {
	var txID uuid.UUID
	err := q.QueryRow(ctx, `
		SELECT id
		FROM giki_sports.wallet_transactions
		WHERE type = $1 AND reference_id = $2;
	`, params.Type, params.ReferenceID).Scan(&txID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, fmt.Errorf("failed to query existing transaction: %w", err)
	}

	// Query entries recorded for this transaction header
	rows, err := q.Query(ctx, `
		SELECT wallet_id, amount
		FROM giki_sports.wallet_ledger_entries
		WHERE transaction_id = $1;
	`, txID)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("failed to query existing ledger entries: %w", err)
	}
	defer rows.Close()

	var entryCount int
	var senderMatched, receiverMatched bool
	for rows.Next() {
		entryCount++
		var wID uuid.UUID
		var amt int64
		if err := rows.Scan(&wID, &amt); err != nil {
			return uuid.Nil, false, fmt.Errorf("failed to scan ledger entry: %w", err)
		}
		if wID == params.SenderWalletID && amt == -params.Amount {
			senderMatched = true
		} else if wID == params.ReceiverWalletID && amt == params.Amount {
			receiverMatched = true
		}
	}
	if err := rows.Err(); err != nil {
		return uuid.Nil, false, fmt.Errorf("rows iteration error: %w", err)
	}

	// An identical transfer requires exactly 2 entries matching sender (-amount) and receiver (+amount)
	isMatch := (entryCount == 2 && senderMatched && receiverMatched)
	return txID, isMatch, nil
}

// GetBalance derives the aggregate financial balance of a wallet in paisa by summing all historical ledger entries.
func (s *Service) GetBalance(ctx context.Context, walletID uuid.UUID) (int64, error) {
	var balance int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0)
		FROM giki_sports.wallet_ledger_entries
		WHERE wallet_id = $1;
	`, walletID).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("failed to query derived wallet balance: %w", err)
	}
	return balance, nil
}

// GetWallet retrieves wallet metadata by ID.
func (s *Service) GetWallet(ctx context.Context, walletID uuid.UUID) (*Wallet, error) {
	var w Wallet
	var uid *uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, type, status, currency, created_at, updated_at
		FROM giki_sports.wallets
		WHERE id = $1;
	`, walletID).Scan(&w.ID, &uid, &w.Name, &w.Type, &w.Status, &w.Currency, &w.CreatedAt, &w.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWalletNotFound
		}
		return nil, fmt.Errorf("failed to query wallet %s: %w", walletID, err)
	}
	w.UserID = uid
	return &w, nil
}

// CreateWallet creates a new personal or system wallet.
func (s *Service) CreateWallet(ctx context.Context, userID *uuid.UUID, name string, wType WalletType, currency string) (*Wallet, error) {
	if currency == "" {
		currency = "PKR"
	}
	var w Wallet
	var uid *uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO giki_sports.wallets (user_id, name, type, status, currency)
		VALUES ($1, $2, $3, 'ACTIVE', $4)
		RETURNING id, user_id, name, type, status, currency, created_at, updated_at;
	`, userID, name, string(wType), currency).Scan(&w.ID, &uid, &w.Name, &w.Type, &w.Status, &w.Currency, &w.CreatedAt, &w.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}
	w.UserID = uid
	return &w, nil
}

// GetUserWalletBalance finds only the personal wallet owned by the given Sports user ID
// and calculates its balance dynamically by summing all historical ledger entries in integer paisa.
//
// CONCURRENCY SNAPSHOT DOCUMENTATION:
// Note that GetUserWalletBalance and GetUserWalletHistory execute independent read queries against
// PostgreSQL (using the default Read Committed transaction isolation level). Under concurrent financial
// writes, if a transfer commits between a call to GetUserWalletBalance and a call to GetUserWalletHistory,
// each call will observe a slightly different snapshot of the ledger. Callers requiring an identical
// snapshot across both reads should coordinate them inside a single read-only transaction (pgx.Tx).
//
// Read-only invariants:
// - Distinguishes "no wallet exists" (returns ErrWalletNotFound) from "wallet exists with zero balance" (returns balance = 0).
// - Strictly read-only: does NOT create wallets on read (unlike legacy GIKI Wallet).
// - Excludes system accounts (type = 'SYS_REVENUE', 'SYS_LIABILITY') by enforcing type = 'PERSONAL'.
// - Evaluates all ledger entries with 64-bit integer paisa without rounding or floating-point truncation.
func (s *Service) GetUserWalletBalance(ctx context.Context, userID uuid.UUID) (*UserWalletBalance, error) {
	if userID == uuid.Nil {
		return nil, ErrWalletNotFound
	}

	var walletID uuid.UUID
	var status WalletStatus
	var currency string

	err := s.pool.QueryRow(ctx, `
		SELECT id, status, currency
		FROM giki_sports.wallets
		WHERE user_id = $1 AND type = 'PERSONAL';
	`, userID).Scan(&walletID, &status, &currency)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWalletNotFound
		}
		return nil, fmt.Errorf("failed to query personal wallet for user %s: %w", userID, err)
	}

	var balance int64
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0)
		FROM giki_sports.wallet_ledger_entries
		WHERE wallet_id = $1;
	`, walletID).Scan(&balance)

	if err != nil {
		return nil, fmt.Errorf("failed to derive balance for wallet %s: %w", walletID, err)
	}

	return &UserWalletBalance{
		WalletID: walletID,
		UserID:   userID,
		Status:   status,
		Currency: currency,
		Balance:  balance,
	}, nil
}

// GetUserWalletHistory retrieves the latest 50 ledger entries for the personal wallet owned by the specified user ID.
//
// Ordering & Scope:
// - Returns at most 50 entries ordered newest first (created_at DESC), with a deterministic id ASC tie-breaker.
// - Returns an empty slice (not nil) when the wallet has no transaction entries.
// - Scoped strictly to the authenticated user's wallet: does not expose another user's balance, details, or entries.
// - Strictly read-only: does not create or alter database rows.
func (s *Service) GetUserWalletHistory(ctx context.Context, userID uuid.UUID) ([]WalletActivityItem, error) {
	if userID == uuid.Nil {
		return nil, ErrWalletNotFound
	}

	var walletID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id
		FROM giki_sports.wallets
		WHERE user_id = $1 AND type = 'PERSONAL';
	`, userID).Scan(&walletID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWalletNotFound
		}
		return nil, fmt.Errorf("failed to query personal wallet for user %s: %w", userID, err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			e.id,
			e.transaction_id,
			t.type,
			e.amount,
			e.created_at
		FROM giki_sports.wallet_ledger_entries e
		JOIN giki_sports.wallet_transactions t ON e.transaction_id = t.id
		WHERE e.wallet_id = $1
		ORDER BY e.created_at DESC, e.id ASC
		LIMIT 50;
	`, walletID)

	if err != nil {
		return nil, fmt.Errorf("failed to query ledger activity for wallet %s: %w", walletID, err)
	}
	defer rows.Close()

	items := make([]WalletActivityItem, 0, 50)
	for rows.Next() {
		var item WalletActivityItem
		if err := rows.Scan(&item.EntryID, &item.TransactionID, &item.Type, &item.Amount, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan activity item: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading activity entries: %w", err)
	}

	return items, nil
}
