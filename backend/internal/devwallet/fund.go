package devwallet

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"giki-sports-backend/internal/devuser"
	"giki-sports-backend/internal/wallet"
)

// DevTestCreditAmountPaisa defines the fixed synthetic test credit: 50,000 paisa (PKR 500.00).
const DevTestCreditAmountPaisa int64 = 50000

// DevTestCreditType defines the dedicated transaction type for developer wallet funding.
const DevTestCreditType = "DEV_TEST_CREDIT"

// DevTestClearingWalletName identifies the dedicated system clearing account.
const DevTestClearingWalletName = "Dev Test Credit Clearing"

// TargetUser models the minimal user record retrieved from giki_sports.users.
type TargetUser struct {
	ID          uuid.UUID
	Email       string
	DisplayName string
	Status      string
}

// FundResult summarizes the outcome of funding a local development wallet.
type FundResult struct {
	User             TargetUser
	UserWalletID     uuid.UUID
	ClearingWalletID uuid.UUID
	TransactionID    uuid.UUID
	AmountPaisa      int64
	AlreadyPosted    bool
	CurrentBalance   int64
}

// FundDevWallet provisions and funds one existing demo user's Sports wallet with synthetic test credit.
//
// Operational Lifecycle & Guarantees:
// 1. Preset Verification: Maps preset ('student' or 'director') to its designated synthetic example.test email.
// 2. User Existence Guard: Queries giki_sports.users. Fails clearly if the user is missing or inactive. Never creates users.
// 3. Wallet Idempotency: Finds or creates the user's personal wallet and the dedicated test clearing wallet.
//    Reuses existing accounts without changing their status, ownership, or currency.
//    Handles concurrent creation safely through ON CONFLICT DO NOTHING.
// 4. Double-Entry Posting: Posts fixed synthetic credit (50,000 paisa = PKR 500.00) via wallet.Service.PostTransaction.
//    Sender: dedicated test clearing account (SYS_LIABILITY).
//    Receiver: user's personal wallet (PERSONAL).
//    Type: DEV_TEST_CREDIT.
//    ReferenceID: stable reference tied to the user ID (DEV_CREDIT_<user.ID>).
//    Never inserts ledger entries directly.
// 5. Replay Protection: Rerunning returns the original transaction ID without adding duplicate funds,
//    even if the user has already spent some of the credit.
// 6. Decoupled Transaction Isolation: Account provisioning and PostTransaction are intentionally NOT wrapped
//    in an outer database transaction. PostTransaction manages its own atomic pgx.Tx with row locks.
//    If wallet creation succeeds but posting fails, wallets remain safely intact and ready for retry.
func FundDevWallet(ctx context.Context, pool *pgxpool.Pool, preset devuser.Preset) (*FundResult, error) {
	presetDef, err := devuser.GetPreset(string(preset))
	if err != nil {
		return nil, err
	}

	// 1. Look up existing demo account
	user, err := LookupUserByEmail(ctx, pool, presetDef.Email)
	if err != nil {
		return nil, err
	}

	// 2. Find or create user's personal wallet
	userWalletID, err := FindOrCreatePersonalWallet(ctx, pool, user.ID, fmt.Sprintf("%s Personal Wallet", user.DisplayName))
	if err != nil {
		return nil, fmt.Errorf("failed to find or create personal wallet: %w", err)
	}

	// 3. Find or create dedicated test clearing account
	clearingWalletID, err := FindOrCreateClearingWallet(ctx, pool, DevTestClearingWalletName)
	if err != nil {
		return nil, fmt.Errorf("failed to find or create test clearing wallet: %w", err)
	}

	// 4. Determine stable reference tied to the user ID
	refID := fmt.Sprintf("DEV_CREDIT_%s", user.ID)

	// 5. Post synthetic credit via existing PostTransactionWithResult domain service.
	// Atomic ON CONFLICT resolution inside PostTransactionWithResult guarantees race-free replay detection:
	// exactly one concurrent worker posts the initial credit (AlreadyPosted = false), while all concurrent
	// or subsequent runs detect the existing committed reference (AlreadyPosted = true).
	walletSvc := wallet.NewService(pool)
	postRes, err := walletSvc.PostTransactionWithResult(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWalletID,
		ReceiverWalletID: userWalletID,
		Amount:           DevTestCreditAmountPaisa,
		Type:             DevTestCreditType,
		ReferenceID:      refID,
		Description:      fmt.Sprintf("Local dev synthetic test credit of 500 PKR for %s", user.Email),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to post dev credit transaction: %w", err)
	}

	// 6. Query current derived balance
	balInfo, err := walletSvc.GetUserWalletBalance(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to derive wallet balance: %w", err)
	}

	return &FundResult{
		User:             *user,
		UserWalletID:     userWalletID,
		ClearingWalletID: clearingWalletID,
		TransactionID:    postRes.TransactionID,
		AmountPaisa:      DevTestCreditAmountPaisa,
		AlreadyPosted:    postRes.AlreadyPosted,
		CurrentBalance:   balInfo.Balance,
	}, nil
}

// LookupUserByEmail finds an existing user record and verifies it is active.
func LookupUserByEmail(ctx context.Context, pool *pgxpool.Pool, email string) (*TargetUser, error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	var u TargetUser
	err := pool.QueryRow(ctx, `
		SELECT id, email, display_name, status
		FROM giki_sports.users
		WHERE LOWER(email) = $1;
	`, normEmail).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Status)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("demo user account %q does not exist: provision account first via cmd/provision-dev-user", email)
		}
		return nil, fmt.Errorf("failed to query user account %q: %w", email, err)
	}

	if strings.ToLower(u.Status) != "active" {
		return nil, fmt.Errorf("demo user account %q exists but is not active (status: %s)", email, u.Status)
	}

	return &u, nil
}

// FindOrCreatePersonalWallet finds or creates the personal wallet for a given user.
func FindOrCreatePersonalWallet(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, walletName string) (uuid.UUID, error) {
	var id uuid.UUID
	var status wallet.WalletStatus
	var currency string

	// 1. Try finding existing wallet
	err := pool.QueryRow(ctx, `
		SELECT id, status, currency
		FROM giki_sports.wallets
		WHERE user_id = $1 AND type = 'PERSONAL';
	`, userID).Scan(&id, &status, &currency)

	if err == nil {
		if status != wallet.WalletStatusActive {
			return uuid.Nil, fmt.Errorf("personal wallet for user %s is not active (status: %s)", userID, status)
		}
		if currency != "PKR" {
			return uuid.Nil, fmt.Errorf("personal wallet for user %s has unsupported currency %q (expected PKR)", userID, currency)
		}
		return id, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("failed to check personal wallet: %w", err)
	}

	// 2. Insert with ON CONFLICT DO NOTHING for safe concurrency
	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.wallets (user_id, name, type, status, currency)
		VALUES ($1, $2, 'PERSONAL', 'ACTIVE', 'PKR')
		ON CONFLICT (user_id) WHERE type = 'PERSONAL'
		DO NOTHING
		RETURNING id, status, currency;
	`, userID, walletName).Scan(&id, &status, &currency)

	if err == nil {
		return id, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		// A concurrent worker inserted the wallet; read again
		err = pool.QueryRow(ctx, `
			SELECT id, status, currency
			FROM giki_sports.wallets
			WHERE user_id = $1 AND type = 'PERSONAL';
		`, userID).Scan(&id, &status, &currency)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to retrieve concurrent personal wallet: %w", err)
		}
		return id, nil
	}

	return uuid.Nil, fmt.Errorf("failed to insert personal wallet: %w", err)
}

// FindOrCreateClearingWallet finds or creates the dedicated system clearing account.
func FindOrCreateClearingWallet(ctx context.Context, pool *pgxpool.Pool, clearingName string) (uuid.UUID, error) {
	var id uuid.UUID
	var status wallet.WalletStatus
	var currency string

	// 1. Try finding existing clearing wallet
	err := pool.QueryRow(ctx, `
		SELECT id, status, currency
		FROM giki_sports.wallets
		WHERE name = $1 AND type = 'SYS_LIABILITY' AND user_id IS NULL;
	`, clearingName).Scan(&id, &status, &currency)

	if err == nil {
		if status != wallet.WalletStatusActive {
			return uuid.Nil, fmt.Errorf("clearing wallet %q is not active (status: %s)", clearingName, status)
		}
		if currency != "PKR" {
			return uuid.Nil, fmt.Errorf("clearing wallet %q has unsupported currency %q (expected PKR)", clearingName, currency)
		}
		return id, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("failed to check clearing wallet: %w", err)
	}

	// 2. Insert with ON CONFLICT DO NOTHING for safe concurrency
	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.wallets (user_id, name, type, status, currency)
		VALUES (NULL, $1, 'SYS_LIABILITY', 'ACTIVE', 'PKR')
		ON CONFLICT (name, type) WHERE user_id IS NULL
		DO NOTHING
		RETURNING id, status, currency;
	`, clearingName).Scan(&id, &status, &currency)

	if err == nil {
		return id, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		// A concurrent worker inserted the clearing wallet; read again
		err = pool.QueryRow(ctx, `
			SELECT id, status, currency
			FROM giki_sports.wallets
			WHERE name = $1 AND type = 'SYS_LIABILITY' AND user_id IS NULL;
		`, clearingName).Scan(&id, &status, &currency)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to retrieve concurrent clearing wallet: %w", err)
		}
		return id, nil
	}

	return uuid.Nil, fmt.Errorf("failed to insert clearing wallet: %w", err)
}
