//go:build integration

package devwallet_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"giki-sports-backend/internal/auth"
	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/database"
	"giki-sports-backend/internal/devuser"
	"giki-sports-backend/internal/devwallet"
	"giki-sports-backend/internal/user"
	"giki-sports-backend/internal/wallet"
)

func dropDB(db *sql.DB, dbName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = db.ExecContext(ctx, fmt.Sprintf(`
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = '%s' AND pid <> pg_backend_pid();
	`, dbName))

	_, err := db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE);", dbName))
	return err
}

func setupDisposableDB(t *testing.T) (*pgxpool.Pool, string, func()) {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("integration test failed: unable to load configuration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	maintenanceDB, err := database.NewSQLDB(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("integration test failed: unable to connect to PostgreSQL server: %v", err)
	}

	dbName := fmt.Sprintf("giki_sports_disposable_devwallet_%d", time.Now().UnixNano())
	_, err = maintenanceDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s;", dbName))
	if err != nil {
		maintenanceDB.Close()
		t.Fatalf("failed to create disposable database %s: %v", dbName, err)
	}

	testDBCfg := cfg.DB
	testDBCfg.Name = dbName
	migSQLDB, err := database.NewSQLDB(ctx, testDBCfg)
	if err != nil {
		_ = dropDB(maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to connect to disposable database for migrations: %v", err)
	}

	migrationsDir := filepath.Join("..", "..", "migrations")
	runner, err := database.NewMigrationRunnerFromDir(migSQLDB, migrationsDir)
	if err != nil {
		_ = migSQLDB.Close()
		_ = dropDB(maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to initialize migration runner: %v", err)
	}

	if _, err := runner.Up(ctx); err != nil {
		_ = runner.Close()
		_ = migSQLDB.Close()
		_ = dropDB(maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to apply migrations to disposable database: %v", err)
	}
	_ = runner.Close()
	_ = migSQLDB.Close()

	pool, err := database.NewPool(ctx, testDBCfg)
	if err != nil {
		_ = dropDB(maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to initialize pgx connection pool: %v", err)
	}

	teardown := func() {
		t.Logf("Cleaning up disposable test database: %s", dbName)
		pool.Close()

		if err := dropDB(maintenanceDB, dbName); err != nil {
			t.Errorf("cleanup failure: failed to drop disposable test database %s: %v", dbName, err)
		}
		maintenanceDB.Close()
	}

	return pool, dbName, teardown
}

func seedTestUser(t *testing.T, pool *pgxpool.Pool, email, name, status string, role user.Role) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	userID := uuid.New()
	passHash, _ := auth.Hash("validPassword123")

	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, $2, $3, $4, $5);
	`, userID, email, name, passHash, status)
	if err != nil {
		t.Fatalf("failed to seed user %s: %v", email, err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.user_roles (user_id, role)
		VALUES ($1, $2);
	`, userID, role)
	if err != nil {
		t.Fatalf("failed to seed role for %s: %v", email, err)
	}

	return userID
}

// TestFundDevWallet_Integration exercises the complete dev wallet funding CLI domain logic.
func TestFundDevWallet_Integration(t *testing.T) {
	pool, _, teardown := setupDisposableDB(t)
	defer teardown()

	ctx := context.Background()

	// -------------------------------------------------------------------------
	// 1. Missing user fails clearly without creating records
	// -------------------------------------------------------------------------
	t.Run("Missing user fails clearly", func(t *testing.T) {
		res, err := devwallet.FundDevWallet(ctx, pool, devuser.PresetStudent)
		if err == nil {
			t.Fatalf("expected error for missing user, got result: %+v", res)
		}
		if !strings.Contains(err.Error(), "does not exist: provision account first") {
			t.Errorf("expected missing user guidance, got error: %v", err)
		}

		// Verify 0 rows in wallet tables
		var walletCount int
		_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM giki_sports.wallets;").Scan(&walletCount)
		if walletCount != 0 {
			t.Errorf("expected 0 wallets, got %d", walletCount)
		}
	})

	// -------------------------------------------------------------------------
	// 2. Inactive user fails clearly without creating records
	// -------------------------------------------------------------------------
	t.Run("Inactive user fails clearly", func(t *testing.T) {
		seedTestUser(t, pool, "student@example.test", "Inactive Student", "inactive", user.RoleStudent)

		res, err := devwallet.FundDevWallet(ctx, pool, devuser.PresetStudent)
		if err == nil {
			t.Fatalf("expected error for inactive user, got result: %+v", res)
		}
		if !strings.Contains(err.Error(), "exists but is not active") {
			t.Errorf("expected inactive account guidance, got error: %v", err)
		}

		// Cleanup inactive student to test active flow
		_, _ = pool.Exec(ctx, "DELETE FROM giki_sports.user_roles WHERE user_id IN (SELECT id FROM giki_sports.users WHERE email = 'student@example.test');")
		_, _ = pool.Exec(ctx, "DELETE FROM giki_sports.users WHERE email = 'student@example.test';")
	})

	// -------------------------------------------------------------------------
	// 3. First run (Happy path)
	// -------------------------------------------------------------------------
	var firstResult *devwallet.FundResult
	studentID := seedTestUser(t, pool, "student@example.test", "Development Student", "active", user.RoleStudent)

	t.Run("First run provisions wallets and funds test credit", func(t *testing.T) {
		res, err := devwallet.FundDevWallet(ctx, pool, devuser.PresetStudent)
		if err != nil {
			t.Fatalf("unexpected error on first run: %v", err)
		}
		firstResult = res

		if res.AlreadyPosted {
			t.Errorf("expected AlreadyPosted to be false on first run")
		}
		if res.AmountPaisa != 50000 {
			t.Errorf("expected AmountPaisa 50000, got %d", res.AmountPaisa)
		}
		if res.CurrentBalance != 50000 {
			t.Errorf("expected CurrentBalance 50000, got %d", res.CurrentBalance)
		}
		if res.TransactionID == uuid.Nil {
			t.Errorf("expected non-nil TransactionID")
		}
		if res.UserWalletID == uuid.Nil || res.ClearingWalletID == uuid.Nil {
			t.Errorf("expected non-nil wallet IDs")
		}

		// Verify database state:
		// 1) Exactly 1 personal wallet
		var personalType, personalStatus, personalCurr string
		err = pool.QueryRow(ctx, `
			SELECT type, status, currency FROM giki_sports.wallets WHERE id = $1;
		`, res.UserWalletID).Scan(&personalType, &personalStatus, &personalCurr)
		if err != nil {
			t.Fatalf("failed to query personal wallet: %v", err)
		}
		if personalType != "PERSONAL" || personalStatus != "ACTIVE" || personalCurr != "PKR" {
			t.Errorf("unexpected personal wallet attributes: %s, %s, %s", personalType, personalStatus, personalCurr)
		}

		// 2) Exactly 1 clearing wallet (SYS_LIABILITY)
		var clearingType, clearingStatus, clearingCurr string
		err = pool.QueryRow(ctx, `
			SELECT type, status, currency FROM giki_sports.wallets WHERE id = $1;
		`, res.ClearingWalletID).Scan(&clearingType, &clearingStatus, &clearingCurr)
		if err != nil {
			t.Fatalf("failed to query clearing wallet: %v", err)
		}
		if clearingType != "SYS_LIABILITY" || clearingStatus != "ACTIVE" || clearingCurr != "PKR" {
			t.Errorf("unexpected clearing wallet attributes: %s, %s, %s", clearingType, clearingStatus, clearingCurr)
		}

		// 3) Exactly 1 transaction header
		var txType, txRef string
		err = pool.QueryRow(ctx, `
			SELECT type, reference_id FROM giki_sports.wallet_transactions WHERE id = $1;
		`, res.TransactionID).Scan(&txType, &txRef)
		if err != nil {
			t.Fatalf("failed to query transaction: %v", err)
		}
		if txType != devwallet.DevTestCreditType {
			t.Errorf("expected tx type %s, got %s", devwallet.DevTestCreditType, txType)
		}
		expectedRef := fmt.Sprintf("DEV_CREDIT_%s", studentID)
		if txRef != expectedRef {
			t.Errorf("expected tx reference %s, got %s", expectedRef, txRef)
		}

		// 4) Exactly 2 ledger entries summing to zero
		var sumAmount int64
		var countEntries int
		err = pool.QueryRow(ctx, `
			SELECT COUNT(*), COALESCE(SUM(amount), 0)
			FROM giki_sports.wallet_ledger_entries
			WHERE transaction_id = $1;
		`, res.TransactionID).Scan(&countEntries, &sumAmount)
		if err != nil {
			t.Fatalf("failed to query ledger entries: %v", err)
		}
		if countEntries != 2 {
			t.Errorf("expected exactly 2 ledger entries, got %d", countEntries)
		}
		if sumAmount != 0 {
			t.Errorf("expected sum of ledger entries to be 0 (zero-sum), got %d", sumAmount)
		}

		// 5) Verify owner-scoped wallet balance and history via wallet.Service
		walletSvc := wallet.NewService(pool)
		bal, err := walletSvc.GetUserWalletBalance(ctx, studentID)
		if err != nil {
			t.Fatalf("failed to get user wallet balance: %v", err)
		}
		if bal.Balance != 50000 {
			t.Errorf("expected derived balance 50000, got %d", bal.Balance)
		}

		history, err := walletSvc.GetUserWalletHistory(ctx, studentID)
		if err != nil {
			t.Fatalf("failed to get user wallet history: %v", err)
		}
		if len(history) != 1 {
			t.Fatalf("expected 1 history entry, got %d", len(history))
		}
		if history[0].Amount != 50000 {
			t.Errorf("expected history amount 50000, got %d", history[0].Amount)
		}
		if history[0].Type != devwallet.DevTestCreditType {
			t.Errorf("expected history type %s, got %s", devwallet.DevTestCreditType, history[0].Type)
		}
	})

	// -------------------------------------------------------------------------
	// 4. Rerun is strictly idempotent and does not add duplicate funds
	// -------------------------------------------------------------------------
	t.Run("Rerun returns original transaction without adding funds", func(t *testing.T) {
		res2, err := devwallet.FundDevWallet(ctx, pool, devuser.PresetStudent)
		if err != nil {
			t.Fatalf("unexpected error on rerun: %v", err)
		}

		if !res2.AlreadyPosted {
			t.Errorf("expected AlreadyPosted to be true on rerun")
		}
		if res2.TransactionID != firstResult.TransactionID {
			t.Errorf("expected identical TransactionID %s, got %s", firstResult.TransactionID, res2.TransactionID)
		}
		if res2.CurrentBalance != 50000 {
			t.Errorf("expected balance to remain 50000, got %d", res2.CurrentBalance)
		}

		// Row counts must be identical: 2 wallets, 1 transaction, 2 ledger entries
		var txCount, entryCount int
		_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM giki_sports.wallet_transactions;").Scan(&txCount)
		_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM giki_sports.wallet_ledger_entries;").Scan(&entryCount)
		if txCount != 1 {
			t.Errorf("expected 1 transaction header, got %d", txCount)
		}
		if entryCount != 2 {
			t.Errorf("expected 2 ledger entries, got %d", entryCount)
		}
	})

	// -------------------------------------------------------------------------
	// 5. Rerun after user has spent part of the credit preserves spent state
	// -------------------------------------------------------------------------
	t.Run("Rerun after partial spend does not refill balance", func(t *testing.T) {
		walletSvc := wallet.NewService(pool)

		// Create a system revenue wallet
		sysRevenue, err := walletSvc.CreateWallet(ctx, nil, "System Sports Revenue", wallet.WalletTypeRevenue, "PKR")
		if err != nil {
			t.Fatalf("failed to create revenue wallet: %v", err)
		}

		// User spends 20,000 paisa (Rs. 200) on padel booking
		_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
			SenderWalletID:   firstResult.UserWalletID,
			ReceiverWalletID: sysRevenue.ID,
			Amount:           20000,
			Type:             "PADEL_BOOKING",
			ReferenceID:      "BK-TEST-SPEND-001",
			Description:      "Court booking debit",
		})
		if err != nil {
			t.Fatalf("failed to post booking spend: %v", err)
		}

		// Verify balance dropped to 30,000 paisa
		bal, err := walletSvc.GetUserWalletBalance(ctx, studentID)
		if err != nil || bal.Balance != 30000 {
			t.Fatalf("expected balance 30000, got %d (err: %v)", bal.Balance, err)
		}

		// Now rerun fund-dev-wallet
		res3, err := devwallet.FundDevWallet(ctx, pool, devuser.PresetStudent)
		if err != nil {
			t.Fatalf("unexpected error on rerun after spend: %v", err)
		}

		if !res3.AlreadyPosted {
			t.Errorf("expected AlreadyPosted to be true")
		}
		if res3.TransactionID != firstResult.TransactionID {
			t.Errorf("expected original TransactionID %s, got %s", firstResult.TransactionID, res3.TransactionID)
		}
		// Balance MUST be 30,000 paisa — NOT refilled to 50,000 and NOT doubled to 80,000!
		if res3.CurrentBalance != 30000 {
			t.Errorf("expected balance to remain spent at 30000 paisa, got %d", res3.CurrentBalance)
		}

		// History must have exactly 2 entries (booking debit, then test credit)
		history, err := walletSvc.GetUserWalletHistory(ctx, studentID)
		if err != nil || len(history) != 2 {
			t.Fatalf("expected 2 history entries, got %d (err: %v)", len(history), err)
		}
	})

	// -------------------------------------------------------------------------
	// 6. Concurrent runs are safe and race-free
	// -------------------------------------------------------------------------
	t.Run("Concurrent runs for another preset are race-free", func(t *testing.T) {
		directorID := seedTestUser(t, pool, "director@example.test", "Development Sports Director", "active", user.RoleSportsDirector)

		const workers = 4
		var wg sync.WaitGroup
		results := make([]*devwallet.FundResult, workers)
		errorsList := make([]error, workers)

		startGate := make(chan struct{})

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-startGate
				res, err := devwallet.FundDevWallet(ctx, pool, devuser.PresetDirector)
				results[idx] = res
				errorsList[idx] = err
			}(i)
		}

		// Release all workers simultaneously
		close(startGate)
		wg.Wait()

		var commonTxID uuid.UUID
		firstTimePostCount := 0
		alreadyPostedCount := 0

		for i := 0; i < workers; i++ {
			if errorsList[i] != nil {
				t.Fatalf("worker %d failed with error: %v", i, errorsList[i])
			}
			r := results[i]
			if commonTxID == uuid.Nil {
				commonTxID = r.TransactionID
			} else if r.TransactionID != commonTxID {
				t.Errorf("worker %d got different transaction ID: %s vs %s", i, r.TransactionID, commonTxID)
			}

			if r.AlreadyPosted {
				alreadyPostedCount++
			} else {
				firstTimePostCount++
			}
		}

		// Exactly one worker must have performed the initial post
		if firstTimePostCount != 1 {
			t.Errorf("expected exactly 1 worker to perform initial post, got %d", firstTimePostCount)
		}
		if alreadyPostedCount != workers-1 {
			t.Errorf("expected %d workers to observe already posted, got %d", workers-1, alreadyPostedCount)
		}

		// Final balance for director must be exactly 50,000 paisa
		walletSvc := wallet.NewService(pool)
		bal, err := walletSvc.GetUserWalletBalance(ctx, directorID)
		if err != nil {
			t.Fatalf("failed to query director balance: %v", err)
		}
		if bal.Balance != 50000 {
			t.Errorf("expected director balance 50000, got %d", bal.Balance)
		}
	})

	// -------------------------------------------------------------------------
	// 7. Failed credit leaves no ledger entries
	// -------------------------------------------------------------------------
	t.Run("Failed credit leaves no ledger entries", func(t *testing.T) {
		// Use wallet service directly with simulated failure on second entry
		walletSvc := wallet.NewService(pool)
		walletSvc.SetFailSecondEntryForTest(true)
		defer walletSvc.SetFailSecondEntryForTest(false)

		// Create dummy user and wallet
		dummyID := seedTestUser(t, pool, "dummy@example.test", "Dummy User", "active", user.RoleStudent)
		dummyWallet, err := devwallet.FindOrCreatePersonalWallet(ctx, pool, dummyID, "Dummy Personal")
		if err != nil {
			t.Fatalf("failed to create dummy wallet: %v", err)
		}
		clearingWallet, err := devwallet.FindOrCreateClearingWallet(ctx, pool, devwallet.DevTestClearingWalletName)
		if err != nil {
			t.Fatalf("failed to get clearing wallet: %v", err)
		}

		_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
			SenderWalletID:   clearingWallet,
			ReceiverWalletID: dummyWallet,
			Amount:           50000,
			Type:             devwallet.DevTestCreditType,
			ReferenceID:      fmt.Sprintf("DEV_CREDIT_%s", dummyID),
			Description:      "Simulated failure transfer",
		})
		if err == nil {
			t.Fatal("expected failure on forced second entry error, got nil")
		}

		// Assert that transaction header was rolled back and 0 ledger entries exist for dummy wallet
		var dummyEntriesCount int
		_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM giki_sports.wallet_ledger_entries WHERE wallet_id = $1;", dummyWallet).Scan(&dummyEntriesCount)
		if dummyEntriesCount != 0 {
			t.Errorf("atomicity breach: expected 0 ledger entries on rollback, found %d", dummyEntriesCount)
		}

		var dummyTxCount int
		_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM giki_sports.wallet_transactions WHERE reference_id = $1;", fmt.Sprintf("DEV_CREDIT_%s", dummyID)).Scan(&dummyTxCount)
		if dummyTxCount != 0 {
			t.Errorf("atomicity breach: expected 0 transaction headers on rollback, found %d", dummyTxCount)
		}
	})
}
