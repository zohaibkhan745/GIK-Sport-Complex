//go:build integration

package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/database"
)

// createDisposableDB creates an isolated disposable database for wallet service integration tests.
func createDisposableDB(ctx context.Context, maintenanceDB *sql.DB, prefix string) (string, error) {
	dbName := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	_, err := maintenanceDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s;", dbName))
	if err != nil {
		return "", fmt.Errorf("failed to create disposable database %s: %w", dbName, err)
	}
	return dbName, nil
}

// dropDisposableDB terminates sessions and drops the disposable database.
func dropDisposableDB(ctx context.Context, maintenanceDB *sql.DB, dbName string) error {
	if !strings.HasPrefix(dbName, "giki_sports_disposable_wallet_svc") {
		return fmt.Errorf("safety check violated: refusing to drop database '%s' without expected prefix", dbName)
	}

	_, _ = maintenanceDB.ExecContext(ctx, fmt.Sprintf(`
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = '%s' AND pid <> pg_backend_pid();`, dbName))

	_, err := maintenanceDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE);", dbName))
	return err
}

// setupWalletIntegrationTest provisions a disposable database, applies migrations 1-6,
// and initializes a Service connected via pgxpool.Pool.
func setupWalletIntegrationTest(t *testing.T) (*Service, func()) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load configuration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	maintenanceDB, err := database.NewSQLDB(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("failed to connect to maintenance db: %v", err)
	}

	dbName, err := createDisposableDB(ctx, maintenanceDB, "giki_sports_disposable_wallet_svc")
	if err != nil {
		maintenanceDB.Close()
		t.Fatalf("failed to create disposable db: %v", err)
	}

	testDBCfg := cfg.DB
	testDBCfg.Name = dbName

	// Apply real migrations 1 through 6
	migDB, err := database.NewSQLDB(ctx, testDBCfg)
	if err != nil {
		_ = dropDisposableDB(ctx, maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to connect migDB: %v", err)
	}

	migrationsDir := filepath.Join("..", "..", "migrations")
	runner, err := database.NewMigrationRunnerFromDir(migDB, migrationsDir)
	if err != nil {
		_ = migDB.Close()
		_ = dropDisposableDB(ctx, maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to init migration runner: %v", err)
	}

	upRes, err := runner.Up(ctx)
	if err != nil {
		_ = runner.Close()
		_ = migDB.Close()
		_ = dropDisposableDB(ctx, maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to apply migrations: %v", err)
	}
	if len(upRes) != 6 {
		t.Fatalf("expected 6 migrations applied, got %d", len(upRes))
	}
	_ = runner.Close()
	_ = migDB.Close()

	// Initialize connection pool for Service
	pool, err := database.NewPool(ctx, testDBCfg)
	if err != nil {
		_ = dropDisposableDB(ctx, maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to init connection pool: %v", err)
	}

	svc := NewService(pool)

	cleanup := func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = dropDisposableDB(cleanupCtx, maintenanceDB, dbName)
		maintenanceDB.Close()
	}

	return svc, cleanup
}

func TestWalletService_BalancedEntriesAndDerivedBalances(t *testing.T) {
	svc, cleanup := setupWalletIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Seed test user
	aliceID := uuid.New()
	_, err := svc.pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice@giki.edu.pk', 'Alice Smith', 'hash1', 'active');
	`, aliceID)
	if err != nil {
		t.Fatalf("failed to seed user Alice: %v", err)
	}

	// Create wallets
	aliceWallet, err := svc.CreateWallet(ctx, &aliceID, "Alice Personal", WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create Alice wallet: %v", err)
	}

	liabilityWallet, err := svc.CreateWallet(ctx, nil, "Cashier Clearing", WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create liability wallet: %v", err)
	}

	revenueWallet, err := svc.CreateWallet(ctx, nil, "Sports Revenue", WalletTypeRevenue, "PKR")
	if err != nil {
		t.Fatalf("failed to create revenue wallet: %v", err)
	}

	// 1. Cashier top-up: Liability -> Alice (50,000 paisa)
	topupTxID, err := svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   liabilityWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           50000,
		Type:             "CASHIER_CASH_TOPUP",
		ReferenceID:      "REC-2026-001",
		Description:      "Cashier top-up 500 PKR",
	})
	if err != nil {
		t.Fatalf("failed to post topup transaction: %v", err)
	}
	if topupTxID == uuid.Nil {
		t.Fatalf("expected valid topup transaction ID, got nil")
	}

	// Verify derived balances
	aliceBal, err := svc.GetBalance(ctx, aliceWallet.ID)
	if err != nil || aliceBal != 50000 {
		t.Fatalf("expected Alice balance 50000, got %d, err=%v", aliceBal, err)
	}
	liabBal, err := svc.GetBalance(ctx, liabilityWallet.ID)
	if err != nil || liabBal != -50000 {
		t.Fatalf("expected Liability balance -50000, got %d, err=%v", liabBal, err)
	}

	// Verify entries sum to 0
	var sumAmount int64
	err = svc.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0)
		FROM giki_sports.wallet_ledger_entries
		WHERE transaction_id = $1;
	`, topupTxID).Scan(&sumAmount)
	if err != nil || sumAmount != 0 {
		t.Fatalf("expected topup entries sum to be 0, got %d, err=%v", sumAmount, err)
	}

	// 2. Service debit: Alice -> Sports Revenue (20,000 paisa)
	bookingTxID, err := svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   aliceWallet.ID,
		ReceiverWalletID: revenueWallet.ID,
		Amount:           20000,
		Type:             "PADEL_BOOKING",
		ReferenceID:      "BK-PADEL-101",
		Description:      "Court reservation 200 PKR",
	})
	if err != nil {
		t.Fatalf("failed to post booking transaction: %v", err)
	}
	if bookingTxID == uuid.Nil {
		t.Fatalf("expected valid booking transaction ID, got nil")
	}

	// Verify derived balances
	aliceBal, err = svc.GetBalance(ctx, aliceWallet.ID)
	if err != nil || aliceBal != 30000 {
		t.Fatalf("expected Alice balance 30000, got %d, err=%v", aliceBal, err)
	}
	revBal, err := svc.GetBalance(ctx, revenueWallet.ID)
	if err != nil || revBal != 20000 {
		t.Fatalf("expected Revenue balance 20000, got %d, err=%v", revBal, err)
	}

	// Verify entries sum to 0
	err = svc.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0)
		FROM giki_sports.wallet_ledger_entries
		WHERE transaction_id = $1;
	`, bookingTxID).Scan(&sumAmount)
	if err != nil || sumAmount != 0 {
		t.Fatalf("expected booking entries sum to be 0, got %d, err=%v", sumAmount, err)
	}
}

func TestWalletService_InvalidOrInactiveAccounts(t *testing.T) {
	svc, cleanup := setupWalletIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userA := uuid.New()
	userB := uuid.New()
	_, err := svc.pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'a@giki.edu.pk', 'A', 'h1', 'active'),
		       ($2, 'b@giki.edu.pk', 'B', 'h2', 'active');
	`, userA, userB)
	if err != nil {
		t.Fatalf("failed to seed users: %v", err)
	}

	walletA, err := svc.CreateWallet(ctx, &userA, "Wallet A", WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet A: %v", err)
	}
	walletB, err := svc.CreateWallet(ctx, &userB, "Wallet B", WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet B: %v", err)
	}

	// Non-existent sender wallet
	ghostID := uuid.New()
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   ghostID,
		ReceiverWalletID: walletB.ID,
		Amount:           1000,
		Type:             "TEST",
		ReferenceID:      "REF-1",
	})
	if !errors.Is(err, ErrWalletNotFound) {
		t.Fatalf("expected ErrWalletNotFound for non-existent sender, got %v", err)
	}

	// Non-existent receiver wallet
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   walletA.ID,
		ReceiverWalletID: ghostID,
		Amount:           1000,
		Type:             "TEST",
		ReferenceID:      "REF-2",
	})
	if !errors.Is(err, ErrWalletNotFound) {
		t.Fatalf("expected ErrWalletNotFound for non-existent receiver, got %v", err)
	}

	// Inactive sender wallet (status = FROZEN)
	_, err = svc.pool.Exec(ctx, `UPDATE giki_sports.wallets SET status = 'FROZEN' WHERE id = $1;`, walletA.ID)
	if err != nil {
		t.Fatalf("failed to freeze wallet A: %v", err)
	}
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   walletA.ID,
		ReceiverWalletID: walletB.ID,
		Amount:           1000,
		Type:             "TEST",
		ReferenceID:      "REF-3",
	})
	if !errors.Is(err, ErrWalletInactive) {
		t.Fatalf("expected ErrWalletInactive for frozen sender, got %v", err)
	}

	// Restore A, freeze B (inactive receiver)
	_, _ = svc.pool.Exec(ctx, `UPDATE giki_sports.wallets SET status = 'ACTIVE' WHERE id = $1;`, walletA.ID)
	_, _ = svc.pool.Exec(ctx, `UPDATE giki_sports.wallets SET status = 'SUSPENDED' WHERE id = $1;`, walletB.ID)

	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   walletA.ID,
		ReceiverWalletID: walletB.ID,
		Amount:           1000,
		Type:             "TEST",
		ReferenceID:      "REF-4",
	})
	if !errors.Is(err, ErrWalletInactive) {
		t.Fatalf("expected ErrWalletInactive for suspended receiver, got %v", err)
	}

	// Currency mismatch
	_, _ = svc.pool.Exec(ctx, `UPDATE giki_sports.wallets SET status = 'ACTIVE' WHERE id = $1;`, walletB.ID)
	walletUSD, err := svc.CreateWallet(ctx, nil, "USD Clearing", WalletTypeLiability, "USD")
	if err != nil {
		t.Fatalf("failed to create USD wallet: %v", err)
	}

	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   walletUSD.ID,
		ReceiverWalletID: walletA.ID,
		Amount:           1000,
		Type:             "TEST",
		ReferenceID:      "REF-5",
	})
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("expected ErrCurrencyMismatch between USD and PKR, got %v", err)
	}
}

func TestWalletService_InsufficientFunds(t *testing.T) {
	svc, cleanup := setupWalletIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userAlice := uuid.New()
	_, err := svc.pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice@giki.edu.pk', 'Alice', 'h1', 'active');
	`, userAlice)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	aliceWallet, err := svc.CreateWallet(ctx, &userAlice, "Alice Personal", WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create Alice wallet: %v", err)
	}
	liabWallet, err := svc.CreateWallet(ctx, nil, "Cashier Clearing", WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create liability wallet: %v", err)
	}
	revWallet, err := svc.CreateWallet(ctx, nil, "Sports Revenue", WalletTypeRevenue, "PKR")
	if err != nil {
		t.Fatalf("failed to create revenue wallet: %v", err)
	}

	// Fund Alice with 10,000 paisa
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   liabWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           10000,
		Type:             "TOPUP",
		ReferenceID:      "TOP-1",
	})
	if err != nil {
		t.Fatalf("failed topup: %v", err)
	}

	// Attempt debit of 15,000 paisa
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   aliceWallet.ID,
		ReceiverWalletID: revWallet.ID,
		Amount:           15000,
		Type:             "BOOKING",
		ReferenceID:      "FAIL-1",
	})
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}

	// Verify balance is still 10,000
	bal, err := svc.GetBalance(ctx, aliceWallet.ID)
	if err != nil || bal != 10000 {
		t.Fatalf("expected balance to remain 10000, got %d, err=%v", bal, err)
	}

	// Verify no transaction header was persisted
	var txCount int
	_ = svc.pool.QueryRow(ctx, `SELECT count(*) FROM giki_sports.wallet_transactions WHERE reference_id = 'FAIL-1';`).Scan(&txCount)
	if txCount != 0 {
		t.Fatalf("expected 0 transaction records for failed debit, got %d", txCount)
	}
}

func TestWalletService_IdenticalAndConflictingReplays(t *testing.T) {
	svc, cleanup := setupWalletIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userAlice := uuid.New()
	userBob := uuid.New()
	_, err := svc.pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice@giki.edu.pk', 'Alice', 'h1', 'active'),
		       ($2, 'bob@giki.edu.pk', 'Bob', 'h2', 'active');
	`, userAlice, userBob)
	if err != nil {
		t.Fatalf("failed to seed users: %v", err)
	}

	aliceWallet, _ := svc.CreateWallet(ctx, &userAlice, "Alice Personal", WalletTypePersonal, "PKR")
	bobWallet, _ := svc.CreateWallet(ctx, &userBob, "Bob Personal", WalletTypePersonal, "PKR")
	liabWallet, _ := svc.CreateWallet(ctx, nil, "Cashier Clearing", WalletTypeLiability, "PKR")
	revWallet, _ := svc.CreateWallet(ctx, nil, "Sports Revenue", WalletTypeRevenue, "PKR")

	// Fund Alice with exactly 20,000 paisa
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   liabWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           20000,
		Type:             "TOPUP",
		ReferenceID:      "TOP-ALICE-1",
	})
	if err != nil {
		t.Fatalf("failed topup: %v", err)
	}

	// Post transfer using ALL 20,000 paisa (balance becomes 0)
	originalTxID, err := svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   aliceWallet.ID,
		ReceiverWalletID: revWallet.ID,
		Amount:           20000,
		Type:             "PADEL_BOOKING",
		ReferenceID:      "BK-ZERO-BAL-1",
		Description:      "Court reservation",
	})
	if err != nil {
		t.Fatalf("failed initial booking transfer: %v", err)
	}

	// Verify balance is now 0
	bal, _ := svc.GetBalance(ctx, aliceWallet.ID)
	if bal != 0 {
		t.Fatalf("expected Alice balance to be 0, got %d", bal)
	}

	// 1. Identical replay: MUST succeed and return originalTxID even though balance is 0
	replayTxID, err := svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   aliceWallet.ID,
		ReceiverWalletID: revWallet.ID,
		Amount:           20000,
		Type:             "PADEL_BOOKING",
		ReferenceID:      "BK-ZERO-BAL-1",
		Description:      "Court reservation replay",
	})
	if err != nil {
		t.Fatalf("expected identical replay to succeed with zero balance, got: %v", err)
	}
	if replayTxID != originalTxID {
		t.Fatalf("expected replayTxID %s to equal originalTxID %s", replayTxID, originalTxID)
	}

	// Verify Alice balance is still 0 (no duplicate debit)
	bal, _ = svc.GetBalance(ctx, aliceWallet.ID)
	if bal != 0 {
		t.Fatalf("expected Alice balance to remain 0, got %d", bal)
	}

	// 2. Conflicting replays:
	// 2A: Different amount (25,000 instead of 20,000)
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   aliceWallet.ID,
		ReceiverWalletID: revWallet.ID,
		Amount:           25000,
		Type:             "PADEL_BOOKING",
		ReferenceID:      "BK-ZERO-BAL-1",
	})
	if !errors.Is(err, ErrTransactionConflict) {
		t.Fatalf("expected ErrTransactionConflict for different amount, got %v", err)
	}

	// 2B: Different receiver (bobWallet instead of revWallet)
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   aliceWallet.ID,
		ReceiverWalletID: bobWallet.ID,
		Amount:           20000,
		Type:             "PADEL_BOOKING",
		ReferenceID:      "BK-ZERO-BAL-1",
	})
	if !errors.Is(err, ErrTransactionConflict) {
		t.Fatalf("expected ErrTransactionConflict for different receiver, got %v", err)
	}

	// 2C: Different sender (bobWallet instead of aliceWallet)
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   bobWallet.ID,
		ReceiverWalletID: revWallet.ID,
		Amount:           20000,
		Type:             "PADEL_BOOKING",
		ReferenceID:      "BK-ZERO-BAL-1",
	})
	if !errors.Is(err, ErrTransactionConflict) {
		t.Fatalf("expected ErrTransactionConflict for different sender, got %v", err)
	}
}

func TestWalletService_ConcurrentDebitsCannotBothBeFunded(t *testing.T) {
	svc, cleanup := setupWalletIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userCharlie := uuid.New()
	_, err := svc.pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'charlie@giki.edu.pk', 'Charlie', 'h1', 'active');
	`, userCharlie)
	if err != nil {
		t.Fatalf("failed to seed user Charlie: %v", err)
	}

	charlieWallet, _ := svc.CreateWallet(ctx, &userCharlie, "Charlie Personal", WalletTypePersonal, "PKR")
	liabWallet, _ := svc.CreateWallet(ctx, nil, "Cashier Clearing", WalletTypeLiability, "PKR")
	revWallet, _ := svc.CreateWallet(ctx, nil, "Sports Revenue", WalletTypeRevenue, "PKR")

	// Fund Charlie with exactly 10,000 paisa
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   liabWallet.ID,
		ReceiverWalletID: charlieWallet.ID,
		Amount:           10000,
		Type:             "TOPUP",
		ReferenceID:      "TOP-CHARLIE-1",
	})
	if err != nil {
		t.Fatalf("failed topup: %v", err)
	}

	// Launch 10 concurrent requests to debit 10,000 paisa with DIFFERENT reference IDs
	const concurrency = 10
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	successCount := 0
	insufficientFundsCount := 0
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		refID := fmt.Sprintf("CONCURRENT-DEBIT-%d", i)
		go func(ref string) {
			defer wg.Done()
			<-startBarrier // synchronize start
			_, postErr := svc.PostTransaction(ctx, TransferParams{
				SenderWalletID:   charlieWallet.ID,
				ReceiverWalletID: revWallet.ID,
				Amount:           10000,
				Type:             "CONCURRENT_TEST",
				ReferenceID:      ref,
			})

			mu.Lock()
			defer mu.Unlock()
			if postErr == nil {
				successCount++
			} else if errors.Is(postErr, ErrInsufficientFunds) {
				insufficientFundsCount++
			} else {
				t.Errorf("unexpected error in concurrent debit: %v", postErr)
			}
		}(refID)
	}

	close(startBarrier)
	wg.Wait()

	if successCount != 1 {
		t.Fatalf("expected exactly 1 concurrent debit to succeed, got %d", successCount)
	}
	if insufficientFundsCount != concurrency-1 {
		t.Fatalf("expected exactly %d rejections with ErrInsufficientFunds, got %d", concurrency-1, insufficientFundsCount)
	}

	// Final Charlie balance must be exactly 0
	finalBal, err := svc.GetBalance(ctx, charlieWallet.ID)
	if err != nil || finalBal != 0 {
		t.Fatalf("expected Charlie final balance to be 0, got %d, err=%v", finalBal, err)
	}
}

func TestWalletService_ConcurrentRequestsWithSameReference(t *testing.T) {
	svc, cleanup := setupWalletIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userDiana := uuid.New()
	_, err := svc.pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'diana@giki.edu.pk', 'Diana', 'h1', 'active');
	`, userDiana)
	if err != nil {
		t.Fatalf("failed to seed user Diana: %v", err)
	}

	dianaWallet, _ := svc.CreateWallet(ctx, &userDiana, "Diana Personal", WalletTypePersonal, "PKR")
	liabWallet, _ := svc.CreateWallet(ctx, nil, "Cashier Clearing", WalletTypeLiability, "PKR")
	revWallet, _ := svc.CreateWallet(ctx, nil, "Sports Revenue", WalletTypeRevenue, "PKR")

	// Fund Diana with 50,000 paisa
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   liabWallet.ID,
		ReceiverWalletID: dianaWallet.ID,
		Amount:           50000,
		Type:             "TOPUP",
		ReferenceID:      "TOP-DIANA-1",
	})
	if err != nil {
		t.Fatalf("failed topup: %v", err)
	}

	// 10 concurrent requests with the EXACT SAME (type, reference_id, sender, receiver, amount)
	const concurrency = 10
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	txIDs := make([]uuid.UUID, concurrency)
	errs := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-startBarrier
			txIDs[idx], errs[idx] = svc.PostTransaction(ctx, TransferParams{
				SenderWalletID:   dianaWallet.ID,
				ReceiverWalletID: revWallet.ID,
				Amount:           10000,
				Type:             "MEMBERSHIP",
				ReferenceID:      "DIANA-MEMBERSHIP-REF-1",
				Description:      "Concurrent idempotent replay test",
			})
		}(i)
	}

	close(startBarrier)
	wg.Wait()

	// All 10 must succeed and return the EXACT same transaction ID
	for i, e := range errs {
		if e != nil {
			t.Fatalf("concurrent request %d failed: %v", i, e)
		}
	}

	firstTxID := txIDs[0]
	for i, id := range txIDs {
		if id != firstTxID {
			t.Fatalf("request %d returned different txID: expected %s, got %s", i, firstTxID, id)
		}
	}

	// Diana must be debited ONLY ONCE: 50,000 - 10,000 = 40,000 paisa
	dianaBal, err := svc.GetBalance(ctx, dianaWallet.ID)
	if err != nil || dianaBal != 40000 {
		t.Fatalf("expected Diana balance 40,000 paisa, got %d, err=%v", dianaBal, err)
	}

	// Database must contain exactly 1 transaction header and 2 ledger entries
	var headerCount int
	_ = svc.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM giki_sports.wallet_transactions
		WHERE reference_id = 'DIANA-MEMBERSHIP-REF-1';
	`).Scan(&headerCount)
	if headerCount != 1 {
		t.Fatalf("expected exactly 1 transaction header, got %d", headerCount)
	}

	var entryCount int
	_ = svc.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM giki_sports.wallet_ledger_entries
		WHERE transaction_id = $1;
	`, firstTxID).Scan(&entryCount)
	if entryCount != 2 {
		t.Fatalf("expected exactly 2 ledger entries, got %d", entryCount)
	}
}

func TestWalletService_ForcedFailureOnSecondEntryRollsBackAtomically(t *testing.T) {
	svc, cleanup := setupWalletIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userAlice := uuid.New()
	_, err := svc.pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice@giki.edu.pk', 'Alice', 'h1', 'active');
	`, userAlice)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	aliceWallet, _ := svc.CreateWallet(ctx, &userAlice, "Alice Personal", WalletTypePersonal, "PKR")
	revWallet, _ := svc.CreateWallet(ctx, nil, "Sports Revenue", WalletTypeRevenue, "PKR")
	liabWallet, _ := svc.CreateWallet(ctx, nil, "Cashier Clearing", WalletTypeLiability, "PKR")

	// Fund Alice with 20,000 paisa
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   liabWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           20000,
		Type:             "TOPUP",
		ReferenceID:      "TOP-FORCE-1",
	})
	if err != nil {
		t.Fatalf("failed topup: %v", err)
	}

	// Enable test hook to force failure on second entry
	svc.SetFailSecondEntryForTest(true)
	defer svc.SetFailSecondEntryForTest(false)

	targetRef := "FORCED-FAIL-REF-99"
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   aliceWallet.ID,
		ReceiverWalletID: revWallet.ID,
		Amount:           5000,
		Type:             "PADEL_BOOKING",
		ReferenceID:      targetRef,
	})
	if err == nil {
		t.Fatalf("expected PostTransaction to fail due to forced second entry failure, got nil")
	}

	// Verify atomic rollback: neither the transaction header nor either entry exists!
	var txCount int
	_ = svc.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM giki_sports.wallet_transactions
		WHERE reference_id = $1;
	`, targetRef).Scan(&txCount)
	if txCount != 0 {
		t.Fatalf("expected 0 transaction headers after rollback, got %d", txCount)
	}

	var entryCount int
	_ = svc.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM giki_sports.wallet_ledger_entries
		WHERE amount IN (-5000, 5000);
	`).Scan(&entryCount)
	if entryCount != 0 {
		t.Fatalf("expected 0 ledger entries after rollback, got %d", entryCount)
	}

	// Alice balance must remain completely unchanged at 20,000
	aliceBal, err := svc.GetBalance(ctx, aliceWallet.ID)
	if err != nil || aliceBal != 20000 {
		t.Fatalf("expected Alice balance to remain 20000, got %d, err=%v", aliceBal, err)
	}
}

func TestWalletService_ConcurrentReplayExhaustedBalance_ExplicitSync(t *testing.T) {
	svc, cleanup := setupWalletIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userAlice := uuid.New()
	_, err := svc.pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice_exhaust@giki.edu.pk', 'Alice Exhaust', 'h1', 'active');
	`, userAlice)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	aliceWallet, _ := svc.CreateWallet(ctx, &userAlice, "Alice Personal", WalletTypePersonal, "PKR")
	revWallet, _ := svc.CreateWallet(ctx, nil, "Sports Revenue", WalletTypeRevenue, "PKR")
	liabWallet, _ := svc.CreateWallet(ctx, nil, "Cashier Clearing", WalletTypeLiability, "PKR")

	// Fund Alice with EXACTLY one transfer's amount: 10,000 paisa
	const transferAmount int64 = 10000
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   liabWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           transferAmount,
		Type:             "TOPUP",
		ReferenceID:      "TOP-EXHAUST-1",
	})
	if err != nil {
		t.Fatalf("failed topup: %v", err)
	}

	// Verify Alice balance is exactly 10,000 paisa
	initialBal, err := svc.GetBalance(ctx, aliceWallet.ID)
	if err != nil || initialBal != transferAmount {
		t.Fatalf("expected initial balance %d, got %d", transferAmount, initialBal)
	}

	params := TransferParams{
		SenderWalletID:   aliceWallet.ID,
		ReceiverWalletID: revWallet.ID,
		Amount:           transferAmount,
		Type:             "PADEL_BOOKING",
		ReferenceID:      "RACE-EXHAUST-REF-1",
		Description:      "Concurrent race test exhausting balance",
	}

	// Explicit synchronization channels: ZERO sleeps
	req1Entered := make(chan struct{})
	req1Proceed := make(chan struct{})

	svc.SetBeforeCommitHook(func(hookCtx context.Context) {
		close(req1Entered) // Signal: Request 1 has locked wallets, inserted header & entries, but has NOT committed
		select {
		case <-req1Proceed:
		case <-hookCtx.Done():
		}
	})
	defer svc.SetBeforeCommitHook(nil)

	// Launch Request 1
	var txID1 uuid.UUID
	var err1 error
	req1Done := make(chan struct{})
	go func() {
		defer close(req1Done)
		txID1, err1 = svc.PostTransaction(ctx, params)
	}()

	// Wait until Request 1 is paused right before commit, actively holding the wallet lock
	select {
	case <-req1Entered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Request 1 to enter beforeCommitHook")
	}

	// Launch Request 2 with IDENTICAL parameters while Request 1 holds the lock
	var txID2 uuid.UUID
	var err2 error
	req2Started := make(chan struct{})
	req2Done := make(chan struct{})
	go func() {
		close(req2Started) // Signal: Request 2 has invoked PostTransaction
		defer close(req2Done)
		txID2, err2 = svc.PostTransaction(ctx, params)
	}()

	// Wait until Request 2 starts PostTransaction
	<-req2Started

	// Verify explicit overlap: Request 2 must be blocked waiting on the wallet lock while Request 1 holds it
	select {
	case <-req2Done:
		t.Fatal("Request 2 completed prematurely before Request 1 committed - concurrency overlap failed")
	default:
		// Confirmed: Request 2 is actively overlapping and blocked while Request 1 holds the lock
	}

	// Release Request 1 to commit its transaction
	close(req1Proceed)

	// Wait for both requests to complete
	<-req1Done
	<-req2Done

	// Verify outcomes
	if err1 != nil {
		t.Fatalf("Request 1 failed: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("Request 2 failed (expected safe replay, got: %v)", err2)
	}
	if txID1 == uuid.Nil {
		t.Fatalf("Request 1 returned nil transaction ID")
	}
	if txID2 != txID1 {
		t.Fatalf("Request 2 returned different transaction ID: expected %s, got %s", txID1, txID2)
	}

	// Verify database state: exactly 1 transaction header, exactly 2 ledger entries
	var headerCount int
	_ = svc.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM giki_sports.wallet_transactions
		WHERE reference_id = $1;
	`, params.ReferenceID).Scan(&headerCount)
	if headerCount != 1 {
		t.Fatalf("expected exactly 1 transaction header, got %d", headerCount)
	}

	var entryCount int
	_ = svc.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM giki_sports.wallet_ledger_entries
		WHERE transaction_id = $1;
	`, txID1).Scan(&entryCount)
	if entryCount != 2 {
		t.Fatalf("expected exactly 2 ledger entries, got %d", entryCount)
	}

	// Verify Alice's balance ends at exactly zero
	finalBal, err := svc.GetBalance(ctx, aliceWallet.ID)
	if err != nil || finalBal != 0 {
		t.Fatalf("expected Alice final balance to be 0, got %d, err=%v", finalBal, err)
	}
}

func TestWalletService_ConcurrentConflictingReplay_ExplicitSync(t *testing.T) {
	svc, cleanup := setupWalletIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userAlice := uuid.New()
	_, err := svc.pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice_conflict@giki.edu.pk', 'Alice Conflict', 'h1', 'active');
	`, userAlice)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	aliceWallet, _ := svc.CreateWallet(ctx, &userAlice, "Alice Personal", WalletTypePersonal, "PKR")
	revWallet, _ := svc.CreateWallet(ctx, nil, "Sports Revenue", WalletTypeRevenue, "PKR")
	liabWallet, _ := svc.CreateWallet(ctx, nil, "Cashier Clearing", WalletTypeLiability, "PKR")

	// Fund Alice with exactly 10,000 paisa
	const transferAmount int64 = 10000
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   liabWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           transferAmount,
		Type:             "TOPUP",
		ReferenceID:      "TOP-CONFLICT-RACE-1",
	})
	if err != nil {
		t.Fatalf("failed topup: %v", err)
	}

	const sharedRef = "RACE-CONFLICT-REF-1"

	// Request 1: Amount = 10,000
	params1 := TransferParams{
		SenderWalletID:   aliceWallet.ID,
		ReceiverWalletID: revWallet.ID,
		Amount:           10000,
		Type:             "PADEL_BOOKING",
		ReferenceID:      sharedRef,
		Description:      "Original booking",
	}

	// Request 2: Conflicting Amount = 20,000 with SAME reference
	params2 := TransferParams{
		SenderWalletID:   aliceWallet.ID,
		ReceiverWalletID: revWallet.ID,
		Amount:           20000,
		Type:             "PADEL_BOOKING",
		ReferenceID:      sharedRef,
		Description:      "Conflicting booking attempt",
	}

	req1Entered := make(chan struct{})
	req1Proceed := make(chan struct{})

	svc.SetBeforeCommitHook(func(hookCtx context.Context) {
		close(req1Entered)
		select {
		case <-req1Proceed:
		case <-hookCtx.Done():
		}
	})
	defer svc.SetBeforeCommitHook(nil)

	// Launch Request 1
	var txID1 uuid.UUID
	var err1 error
	req1Done := make(chan struct{})
	go func() {
		defer close(req1Done)
		txID1, err1 = svc.PostTransaction(ctx, params1)
	}()

	// Wait until Request 1 is paused right before commit
	select {
	case <-req1Entered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Request 1 to enter beforeCommitHook")
	}

	// Launch Request 2 with conflicting amount
	var txID2 uuid.UUID
	var err2 error
	req2Started := make(chan struct{})
	req2Done := make(chan struct{})
	go func() {
		close(req2Started)
		defer close(req2Done)
		txID2, err2 = svc.PostTransaction(ctx, params2)
	}()

	<-req2Started

	// Verify overlap
	select {
	case <-req2Done:
		t.Fatal("Request 2 completed prematurely before Request 1 committed")
	default:
	}

	// Release Request 1 to commit
	close(req1Proceed)

	<-req1Done
	<-req2Done

	// Verify outcomes
	if err1 != nil {
		t.Fatalf("Request 1 failed: %v", err1)
	}
	if txID1 == uuid.Nil {
		t.Fatalf("Request 1 returned nil txID")
	}

	// Request 2 MUST return ErrTransactionConflict, NOT ErrInsufficientFunds, even though Alice now has 0 balance!
	if !errors.Is(err2, ErrTransactionConflict) {
		t.Fatalf("expected Request 2 to fail with ErrTransactionConflict, got: %v", err2)
	}
	if txID2 != uuid.Nil {
		t.Fatalf("expected Request 2 to return nil txID on conflict, got %s", txID2)
	}

	// Alice final balance must be exactly 0
	finalBal, err := svc.GetBalance(ctx, aliceWallet.ID)
	if err != nil || finalBal != 0 {
		t.Fatalf("expected Alice final balance to be 0, got %d, err=%v", finalBal, err)
	}
}

func TestWalletService_OwnerScopedBalanceAndHistory(t *testing.T) {
	svc, cleanup := setupWalletIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. Test "No wallet exists"
	t.Log("Testing read for non-existent wallet...")
	ghostUserID := uuid.New()
	_, err := svc.GetUserWalletBalance(ctx, ghostUserID)
	if !errors.Is(err, ErrWalletNotFound) {
		t.Fatalf("expected ErrWalletNotFound for non-existent user balance, got %v", err)
	}
	_, err = svc.GetUserWalletHistory(ctx, ghostUserID)
	if !errors.Is(err, ErrWalletNotFound) {
		t.Fatalf("expected ErrWalletNotFound for non-existent user history, got %v", err)
	}

	// 2. Test "Zero balance and empty history"
	t.Log("Testing zero balance and empty history for fresh personal wallet...")
	aliceID := uuid.New()
	_, err = svc.pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice_owner@giki.edu.pk', 'Alice Owner', 'h1', 'active');
	`, aliceID)
	if err != nil {
		t.Fatalf("failed to seed Alice: %v", err)
	}

	aliceWallet, err := svc.CreateWallet(ctx, &aliceID, "Alice Personal", WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create Alice wallet: %v", err)
	}

	aliceBal, err := svc.GetUserWalletBalance(ctx, aliceID)
	if err != nil {
		t.Fatalf("failed to get Alice wallet balance: %v", err)
	}
	if aliceBal.Balance != 0 {
		t.Fatalf("expected initial balance 0 paisa, got %d", aliceBal.Balance)
	}
	if aliceBal.Status != WalletStatusActive || aliceBal.Currency != "PKR" || aliceBal.WalletID != aliceWallet.ID {
		t.Fatalf("mismatched wallet metadata: %+v", aliceBal)
	}

	aliceHistory, err := svc.GetUserWalletHistory(ctx, aliceID)
	if err != nil {
		t.Fatalf("failed to get Alice wallet history: %v", err)
	}
	if aliceHistory == nil || len(aliceHistory) != 0 {
		t.Fatalf("expected empty non-nil history slice for fresh wallet, got: %+v", aliceHistory)
	}

	// 3. Test "Balance after credits and debits"
	t.Log("Testing balance and history after multiple credits and debits...")
	liabWallet, _ := svc.CreateWallet(ctx, nil, "Cashier Clearing", WalletTypeLiability, "PKR")
	revWallet, _ := svc.CreateWallet(ctx, nil, "Sports Revenue", WalletTypeRevenue, "PKR")

	// Credit 1: +50,000 paisa
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   liabWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           50000,
		Type:             "CASHIER_CASH_TOPUP",
		ReferenceID:      "REC-ALICE-1",
	})
	if err != nil {
		t.Fatalf("credit 1 failed: %v", err)
	}

	// Debit 1: -20,000 paisa
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   aliceWallet.ID,
		ReceiverWalletID: revWallet.ID,
		Amount:           20000,
		Type:             "PADEL_BOOKING",
		ReferenceID:      "BK-ALICE-1",
	})
	if err != nil {
		t.Fatalf("debit 1 failed: %v", err)
	}

	// Credit 2: +15,000 paisa
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   liabWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           15000,
		Type:             "CASHIER_CASH_TOPUP",
		ReferenceID:      "REC-ALICE-2",
	})
	if err != nil {
		t.Fatalf("credit 2 failed: %v", err)
	}

	// Debit 2: -5,000 paisa
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   aliceWallet.ID,
		ReceiverWalletID: revWallet.ID,
		Amount:           5000,
		Type:             "PADEL_BOOKING",
		ReferenceID:      "BK-ALICE-2",
	})
	if err != nil {
		t.Fatalf("debit 2 failed: %v", err)
	}

	// Derived balance should be: 50,000 - 20,000 + 15,000 - 5,000 = 40,000 paisa
	aliceBal, err = svc.GetUserWalletBalance(ctx, aliceID)
	if err != nil || aliceBal.Balance != 40000 {
		t.Fatalf("expected derived balance 40,000 paisa, got %d, err=%v", aliceBal.Balance, err)
	}

	// History should contain 4 items, ordered newest first
	aliceHistory, err = svc.GetUserWalletHistory(ctx, aliceID)
	if err != nil || len(aliceHistory) != 4 {
		t.Fatalf("expected 4 history items, got %d, err=%v", len(aliceHistory), err)
	}

	// Verify entries order: index 0 is newest (-5000), index 3 is oldest (+50000)
	if aliceHistory[0].Amount != -5000 || aliceHistory[0].Type != "PADEL_BOOKING" {
		t.Fatalf("expected item 0 to be newest debit -5000, got: %+v", aliceHistory[0])
	}
	if aliceHistory[1].Amount != 15000 || aliceHistory[1].Type != "CASHIER_CASH_TOPUP" {
		t.Fatalf("expected item 1 to be credit +15000, got: %+v", aliceHistory[1])
	}
	if aliceHistory[2].Amount != -20000 || aliceHistory[2].Type != "PADEL_BOOKING" {
		t.Fatalf("expected item 2 to be debit -20000, got: %+v", aliceHistory[2])
	}
	if aliceHistory[3].Amount != 50000 || aliceHistory[3].Type != "CASHIER_CASH_TOPUP" {
		t.Fatalf("expected item 3 to be oldest credit +50000, got: %+v", aliceHistory[3])
	}

	// 4. Test "Separation between two users"
	t.Log("Testing privacy separation between two users...")
	bobID := uuid.New()
	_, err = svc.pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'bob_owner@giki.edu.pk', 'Bob Owner', 'h2', 'active');
	`, bobID)
	if err != nil {
		t.Fatalf("failed to seed Bob: %v", err)
	}

	bobWallet, _ := svc.CreateWallet(ctx, &bobID, "Bob Personal", WalletTypePersonal, "PKR")
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   liabWallet.ID,
		ReceiverWalletID: bobWallet.ID,
		Amount:           25000,
		Type:             "CASHIER_CASH_TOPUP",
		ReferenceID:      "REC-BOB-1",
	})
	if err != nil {
		t.Fatalf("Bob topup failed: %v", err)
	}

	// Bob's balance must be 25,000; Alice's must remain 40,000
	bobBal, err := svc.GetUserWalletBalance(ctx, bobID)
	if err != nil || bobBal.Balance != 25000 {
		t.Fatalf("expected Bob balance 25000, got %d, err=%v", bobBal.Balance, err)
	}
	aliceBal, err = svc.GetUserWalletBalance(ctx, aliceID)
	if err != nil || aliceBal.Balance != 40000 {
		t.Fatalf("expected Alice balance 40000, got %d, err=%v", aliceBal.Balance, err)
	}

	// Bob's history must contain only Bob's 1 entry
	bobHistory, err := svc.GetUserWalletHistory(ctx, bobID)
	if err != nil || len(bobHistory) != 1 || bobHistory[0].Amount != 25000 {
		t.Fatalf("expected Bob history to have 1 item with amount 25000, got: %+v", bobHistory)
	}

	// Alice's history must still contain only Alice's 4 entries (none of Bob's entries)
	aliceHistory, err = svc.GetUserWalletHistory(ctx, aliceID)
	if err != nil || len(aliceHistory) != 4 {
		t.Fatalf("expected Alice history to have 4 items, got: %d", len(aliceHistory))
	}
	for _, it := range aliceHistory {
		if it.Amount == 25000 {
			t.Fatalf("privacy violation: Bob's 25,000 paisa entry appeared in Alice's history!")
		}
	}

	// 5. Test "50-entry limit & stable ordering"
	t.Log("Testing 50-entry limit on history and full-balance derivation...")
	charlieID := uuid.New()
	_, err = svc.pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'charlie_owner@giki.edu.pk', 'Charlie Owner', 'h3', 'active');
	`, charlieID)
	if err != nil {
		t.Fatalf("failed to seed Charlie: %v", err)
	}

	charlieWallet, _ := svc.CreateWallet(ctx, &charlieID, "Charlie Personal", WalletTypePersonal, "PKR")

	// Post 55 separate deposits of 1,000 paisa each to Charlie
	const totalDeposits = 55
	const depositAmount int64 = 1000
	for i := 1; i <= totalDeposits; i++ {
		_, err := svc.PostTransaction(ctx, TransferParams{
			SenderWalletID:   liabWallet.ID,
			ReceiverWalletID: charlieWallet.ID,
			Amount:           depositAmount,
			Type:             "TEST_DEPOSIT",
			ReferenceID:      fmt.Sprintf("CHARLIE-DEP-%03d", i),
		})
		if err != nil {
			t.Fatalf("deposit %d failed: %v", i, err)
		}
	}

	// Balance must derive from ALL 55 entries: 55 * 1000 = 55,000 paisa
	charlieBal, err := svc.GetUserWalletBalance(ctx, charlieID)
	if err != nil || charlieBal.Balance != 55000 {
		t.Fatalf("expected Charlie derived balance 55,000 paisa, got %d, err=%v", charlieBal.Balance, err)
	}

	// History must return EXACTLY the latest 50 entries (capped at 50)
	charlieHistory, err := svc.GetUserWalletHistory(ctx, charlieID)
	if err != nil {
		t.Fatalf("failed to query Charlie history: %v", err)
	}
	if len(charlieHistory) != 50 {
		t.Fatalf("expected exactly 50 history entries, got %d", len(charlieHistory))
	}

	// Verify reverse-chronological ordering
	for i := 0; i < len(charlieHistory)-1; i++ {
		curr := charlieHistory[i]
		next := charlieHistory[i+1]
		if curr.CreatedAt.Before(next.CreatedAt) {
			t.Fatalf("history ordering failure at index %d: entry timestamp %v is before next %v",
				i, curr.CreatedAt, next.CreatedAt)
		}
	}

	// 6. Test "Canceled requests"
	t.Log("Testing context cancellation...")
	canceledCtx, cancelFn := context.WithCancel(ctx)
	cancelFn() // cancel immediately

	_, err = svc.GetUserWalletBalance(canceledCtx, aliceID)
	if err == nil {
		t.Fatalf("expected error on canceled context for balance, got nil")
	}

	_, err = svc.GetUserWalletHistory(canceledCtx, aliceID)
	if err == nil {
		t.Fatalf("expected error on canceled context for history, got nil")
	}

	// 7. Test "Reads do not create or change records"
	t.Log("Testing that read operations produce zero database mutations...")
	var wCount1, txCount1, entryCount1 int
	_ = svc.pool.QueryRow(ctx, `SELECT count(*) FROM giki_sports.wallets;`).Scan(&wCount1)
	_ = svc.pool.QueryRow(ctx, `SELECT count(*) FROM giki_sports.wallet_transactions;`).Scan(&txCount1)
	_ = svc.pool.QueryRow(ctx, `SELECT count(*) FROM giki_sports.wallet_ledger_entries;`).Scan(&entryCount1)

	// Perform 10 read operations
	for i := 0; i < 5; i++ {
		_, _ = svc.GetUserWalletBalance(ctx, aliceID)
		_, _ = svc.GetUserWalletHistory(ctx, aliceID)
		_, _ = svc.GetUserWalletBalance(ctx, ghostUserID)
		_, _ = svc.GetUserWalletHistory(ctx, ghostUserID)
	}

	var wCount2, txCount2, entryCount2 int
	_ = svc.pool.QueryRow(ctx, `SELECT count(*) FROM giki_sports.wallets;`).Scan(&wCount2)
	_ = svc.pool.QueryRow(ctx, `SELECT count(*) FROM giki_sports.wallet_transactions;`).Scan(&txCount2)
	_ = svc.pool.QueryRow(ctx, `SELECT count(*) FROM giki_sports.wallet_ledger_entries;`).Scan(&entryCount2)

	if wCount1 != wCount2 || txCount1 != txCount2 || entryCount1 != entryCount2 {
		t.Fatalf("read operations mutated database rows! Wallets: %d->%d, Transactions: %d->%d, Entries: %d->%d",
			wCount1, wCount2, txCount1, txCount2, entryCount1, entryCount2)
	}
	t.Log("Verified: read operations are strictly side-effect free.")
}

