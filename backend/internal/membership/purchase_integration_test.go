//go:build integration

package membership

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"giki-sports-backend/internal/auth"
	"giki-sports-backend/internal/authentication"
	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/database"
	"giki-sports-backend/internal/session"
	"giki-sports-backend/internal/user"
	"giki-sports-backend/internal/wallet"
)

func createDisposableDB(ctx context.Context, maintenanceDB *sql.DB, prefix string) (string, error) {
	dbName := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	_, err := maintenanceDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s;", dbName))
	if err != nil {
		return "", fmt.Errorf("failed to create disposable database %s: %w", dbName, err)
	}
	return dbName, nil
}

func dropDisposableDB(ctx context.Context, maintenanceDB *sql.DB, dbName string) error {
	if !strings.HasPrefix(dbName, "giki_sports_disposable_") {
		return fmt.Errorf("safety check: refusing to drop db '%s' without disposable prefix", dbName)
	}
	_, _ = maintenanceDB.ExecContext(ctx, fmt.Sprintf(`
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = '%s' AND pid <> pg_backend_pid();`, dbName))
	_, err := maintenanceDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE);", dbName))
	return err
}

func setupMembershipIntegrationTest(t *testing.T) (*PostgresRepository, *pgxpool.Pool, *wallet.Service, func()) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	maintenanceDB, err := database.NewSQLDB(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("failed to connect maintenance db: %v", err)
	}

	dbName, err := createDisposableDB(ctx, maintenanceDB, "giki_sports_disposable_membership")
	if err != nil {
		maintenanceDB.Close()
		t.Fatalf("failed to create disposable db: %v", err)
	}

	testDBCfg := cfg.DB
	testDBCfg.Name = dbName
	testDBCfg.MaxConns = 30

	migDB, err := database.NewSQLDB(ctx, testDBCfg)
	if err != nil {
		_ = dropDisposableDB(ctx, maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to connect migDB: %v", err)
	}

	migrationsDir := filepath.Join("..", "..", "migrations")
	runner, err := database.NewMigrationRunnerFromDir(migDB, migrationsDir)
	if err != nil {
		migDB.Close()
		_ = dropDisposableDB(ctx, maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to create migration runner: %v", err)
	}

	if _, err := runner.Up(ctx); err != nil {
		runner.Close()
		migDB.Close()
		_ = dropDisposableDB(ctx, maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}
	runner.Close()
	migDB.Close()

	pool, err := database.NewPool(ctx, testDBCfg)
	if err != nil {
		_ = dropDisposableDB(ctx, maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to connect pgxpool: %v", err)
	}

	repo := NewRepository(pool)
	walletSvc := wallet.NewService(pool)

	cleanup := func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = dropDisposableDB(cleanupCtx, maintenanceDB, dbName)
		maintenanceDB.Close()
	}

	return repo, pool, walletSvc, cleanup
}

// TestPurchase_HappyPath_WalletDebitAnd30DayTerm_Integration verifies FR-04 and QR-06:
// - Atomic wallet debit of 300,000 paisa (PKR 3,000.00).
// - Strict 30-day term: [T_start, T_start + 30 days).
// - System revenue credit and double-entry transaction record.
// - Audit log in giki_sports.audit_logs.
func TestPurchase_HappyPath_WalletDebitAnd30DayTerm_Integration(t *testing.T) {
	repo, pool, walletSvc, cleanup := setupMembershipIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	clock := NewControlledClock(time.Date(2026, time.November, 15, 10, 0, 0, 0, time.UTC))
	svc := NewService(pool, repo, walletSvc, clock)

	// 1. Seed user Alice
	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000001")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice.member@giki.edu.pk', 'Alice Member', 'hash', 'active');
	`, aliceID)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	// 2. Fund Alice's personal wallet with 500,000 paisa (PKR 5,000.00)
	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           500000,
		Type:             "TOP_UP",
		ReferenceID:      "topup-alice-001",
		Description:      "Initial Top Up",
	})
	if err != nil {
		t.Fatalf("failed to fund wallet: %v", err)
	}

	// 3. Purchase gym pass
	mbr, err := svc.PurchaseOrRenewPass(ctx, aliceID, PlanTypeStandard30Day, "idem-purchase-alice-1")
	if err != nil {
		t.Fatalf("failed to purchase gym pass: %v", err)
	}

	// 4. Verify pass details
	if mbr.Status != MembershipStatusActive {
		t.Fatalf("expected status ACTIVE, got %v", mbr.Status)
	}
	if mbr.PricePaisa != 300000 {
		t.Fatalf("expected fee 300000 paisa, got %d", mbr.PricePaisa)
	}
	expectedEnd := clock.Now().Add(30 * 24 * time.Hour)
	if !mbr.EndTime.Equal(expectedEnd) {
		t.Fatalf("expected end time %v, got %v", expectedEnd, mbr.EndTime)
	}

	// 5. Verify Alice's wallet balance: debited by 300,000 -> 200,000 paisa remaining
	aliceBal, err := walletSvc.GetUserWalletBalance(ctx, aliceID)
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}
	if aliceBal.Balance != 200000 {
		t.Fatalf("expected available balance 200000 paisa, got %d", aliceBal.Balance)
	}

	// 6. Verify System Revenue wallet has received 300,000 paisa
	var revBal int64
	err = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0)
		FROM giki_sports.wallet_ledger_entries e
		JOIN giki_sports.wallets w ON e.wallet_id = w.id
		WHERE w.name = 'SYS_MEMBERSHIP_REVENUE';
	`).Scan(&revBal)
	if err != nil || revBal != 300000 {
		t.Fatalf("expected revenue balance 300000, got %d, err=%v", revBal, err)
	}

	// 7. Verify audit log entry
	var auditCount int
	err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM giki_sports.audit_logs
		WHERE actor_id = $1 AND action = 'membership.purchase' AND target_id = $2;
	`, aliceID, mbr.ID).Scan(&auditCount)
	if err != nil || auditCount != 1 {
		t.Fatalf("expected 1 audit log entry, got %d, err=%v", auditCount, err)
	}

	// 8. Verify user summary
	summary, err := svc.GetUserSummary(ctx, aliceID)
	if err != nil {
		t.Fatalf("failed to get summary: %v", err)
	}
	if !summary.IsActive {
		t.Fatalf("expected user summary to be active")
	}
	if summary.DaysRemaining != 30 {
		t.Fatalf("expected 30 days remaining, got %d", summary.DaysRemaining)
	}
}

// TestPurchase_InsufficientFunds_Rollback_Integration verifies that if the user's wallet
// has insufficient funds, the transaction fails and rolls back: zero passes and zero ledger entries.
func TestPurchase_InsufficientFunds_Rollback_Integration(t *testing.T) {
	repo, pool, walletSvc, cleanup := setupMembershipIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	svc := NewService(pool, repo, walletSvc, nil)

	// 1. Seed user Bob with only 100,000 paisa (fee is 300,000)
	bobID := uuid.MustParse("b0000000-0000-0000-0000-000000000001")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'bob.member@giki.edu.pk', 'Bob Member', 'hash', 'active');
	`, bobID)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	bobWallet, err := walletSvc.CreateWallet(ctx, &bobID, "Bob Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: bobWallet.ID,
		Amount:           100000,
		Type:             "TOP_UP",
		ReferenceID:      "topup-bob-001",
		Description:      "Low Top Up",
	})
	if err != nil {
		t.Fatalf("failed to fund wallet: %v", err)
	}

	// 2. Attempt purchase -> must fail with ErrInsufficientFunds
	_, err = svc.PurchaseOrRenewPass(ctx, bobID, PlanTypeStandard30Day, "idem-bob-fail-1")
	if err == nil || !strings.Contains(err.Error(), "insufficient") {
		t.Fatalf("expected ErrInsufficientFunds, got: %v", err)
	}

	// 3. Verify Bob's wallet balance is intact (100,000 paisa)
	bal, err := walletSvc.GetUserWalletBalance(ctx, bobID)
	if err != nil || bal.Balance != 100000 {
		t.Fatalf("expected 100000 paisa, got %d, err=%v", bal.Balance, err)
	}

	// 4. Verify 0 membership rows exist in giki_sports.gym_memberships
	var count int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM giki_sports.gym_memberships WHERE user_id = $1;`, bobID).Scan(&count)
	if err != nil || count != 0 {
		t.Fatalf("expected 0 memberships created, got %d, err=%v", count, err)
	}

	// 5. Verify 0 GYM_MEMBERSHIP transactions in ledger
	err = pool.QueryRow(ctx, `SELECT count(*) FROM giki_sports.wallet_transactions WHERE type = 'GYM_MEMBERSHIP';`).Scan(&count)
	if err != nil || count != 0 {
		t.Fatalf("expected 0 gym membership transactions, got %d, err=%v", count, err)
	}
}

// TestRenewal_EarlyRenewal_ContiguousTerm_Integration verifies early renewal rule (QR-06):
// Renewing early stacks 30 days onto existing pass end time, creating contiguous coverage.
func TestRenewal_EarlyRenewal_ContiguousTerm_Integration(t *testing.T) {
	repo, pool, walletSvc, cleanup := setupMembershipIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	initialNow := time.Date(2026, time.November, 1, 12, 0, 0, 0, time.UTC)
	clock := NewControlledClock(initialNow)
	svc := NewService(pool, repo, walletSvc, clock)

	// Seed user Charlie with 1,000,000 paisa (enough for 2 passes: 600,000 paisa)
	charlieID := uuid.MustParse("c0000000-0000-0000-0000-000000000001")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'charlie.member@giki.edu.pk', 'Charlie Member', 'hash', 'active');
	`, charlieID)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	charlieWallet, err := walletSvc.CreateWallet(ctx, &charlieID, "Charlie Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: charlieWallet.ID,
		Amount:           1000000,
		Type:             "TOP_UP",
		ReferenceID:      "topup-charlie-001",
		Description:      "Sufficient Top Up",
	})
	if err != nil {
		t.Fatalf("failed to fund wallet: %v", err)
	}

	// 1. Initial purchase on Nov 1 -> valid until Dec 1
	pass1, err := svc.PurchaseOrRenewPass(ctx, charlieID, PlanTypeStandard30Day, "idem-charlie-pass1")
	if err != nil {
		t.Fatalf("pass 1 purchase failed: %v", err)
	}
	expectedPass1End := initialNow.Add(30 * 24 * time.Hour) // Dec 1
	if !pass1.EndTime.Equal(expectedPass1End) {
		t.Fatalf("expected pass 1 end %v, got %v", expectedPass1End, pass1.EndTime)
	}

	// 2. Advance clock 10 days to Nov 11 (20 days remaining on pass 1)
	clock.Advance(10 * 24 * time.Hour)

	// 3. Early renewal on Nov 11
	pass2, err := svc.PurchaseOrRenewPass(ctx, charlieID, PlanTypeStandard30Day, "idem-charlie-pass2")
	if err != nil {
		t.Fatalf("pass 2 renewal failed: %v", err)
	}

	// Invariant verification (QR-06):
	// pass2 start MUST equal pass1 end (Dec 1)
	if !pass2.StartTime.Equal(pass1.EndTime) {
		t.Fatalf("expected pass2 start to equal pass1 end (%v), got %v", pass1.EndTime, pass2.StartTime)
	}
	// pass2 end MUST equal pass1 end + 30 days (Dec 31)
	expectedPass2End := pass1.EndTime.Add(30 * 24 * time.Hour)
	if !pass2.EndTime.Equal(expectedPass2End) {
		t.Fatalf("expected pass2 end %v, got %v", expectedPass2End, pass2.EndTime)
	}

	// Charlie's wallet balance: debited twice by 300,000 -> 400,000 paisa remaining
	bal, err := walletSvc.GetUserWalletBalance(ctx, charlieID)
	if err != nil || bal.Balance != 400000 {
		t.Fatalf("expected balance 400000, got %d, err=%v", bal.Balance, err)
	}

	// Summary on Nov 11: EffectiveExpiry is Dec 31 (50 days remaining from Nov 11)
	summary, err := svc.GetUserSummary(ctx, charlieID)
	if err != nil {
		t.Fatalf("failed to get summary: %v", err)
	}
	if !summary.IsActive {
		t.Fatalf("expected active")
	}
	if summary.EffectiveExpiry == nil || !summary.EffectiveExpiry.Equal(expectedPass2End) {
		t.Fatalf("expected effective expiry %v, got %v", expectedPass2End, summary.EffectiveExpiry)
	}
	if summary.DaysRemaining < 49 {
		t.Fatalf("expected around 50 days remaining, got %d", summary.DaysRemaining)
	}
}

// TestPurchase_IdempotencyReplay_Integration verifies safe retry protection (QR-04):
// Submitting the identical purchase request returns the original membership without a duplicate wallet debit.
func TestPurchase_IdempotencyReplay_Integration(t *testing.T) {
	repo, pool, walletSvc, cleanup := setupMembershipIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	svc := NewService(pool, repo, walletSvc, nil)

	daveID := uuid.MustParse("d0000000-0000-0000-0000-000000000001")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'dave.member@giki.edu.pk', 'Dave Member', 'hash', 'active');
	`, daveID)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	daveWallet, err := walletSvc.CreateWallet(ctx, &daveID, "Dave Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: daveWallet.ID,
		Amount:           800000,
		Type:             "TOP_UP",
		ReferenceID:      "topup-dave-001",
		Description:      "Idempotency Top Up",
	})
	if err != nil {
		t.Fatalf("failed to fund wallet: %v", err)
	}

	// 1. Initial purchase with idempotency key
	m1, err := svc.PurchaseOrRenewPass(ctx, daveID, PlanTypeStandard30Day, "idem-dave-safe-retry")
	if err != nil {
		t.Fatalf("initial purchase failed: %v", err)
	}

	// 2. Replay identical purchase 5 times
	for i := 0; i < 5; i++ {
		mRetry, err := svc.PurchaseOrRenewPass(ctx, daveID, PlanTypeStandard30Day, "idem-dave-safe-retry")
		if err != nil {
			t.Fatalf("retry %d failed: %v", i+1, err)
		}
		if mRetry.ID != m1.ID {
			t.Fatalf("expected identical membership ID %s, got %s", m1.ID, mRetry.ID)
		}
	}

	// 3. Verify wallet was debited EXACTLY ONCE (800,000 - 300,000 = 500,000 paisa)
	bal, err := walletSvc.GetUserWalletBalance(ctx, daveID)
	if err != nil || bal.Balance != 500000 {
		t.Fatalf("expected balance 500000 after replays, got %d, err=%v", bal.Balance, err)
	}
}

// TestHTTP_MembershipEndpoints_Integration verifies REST contracts and role authorization.
func TestHTTP_MembershipEndpoints_Integration(t *testing.T) {
	repo, pool, walletSvc, cleanup := setupMembershipIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	svc := NewService(pool, repo, walletSvc, nil)
	handler := NewHandler(svc, 5*time.Second)

	// Seed student & sports director
	studentID := uuid.MustParse("e0000000-0000-0000-0000-000000000001")
	directorID := uuid.MustParse("e0000000-0000-0000-0000-000000000002")

	userRepo := user.NewRepository(pool)
	sessionRepo := session.NewRepository(pool)
	authService, err := authentication.NewService(userRepo, sessionRepo)
	if err != nil {
		t.Fatalf("failed to initialize auth service: %v", err)
	}

	authCfg := config.AuthConfig{
		CookieName:     "giki_session",
		CookieSecure:   false,
		CookiePath:     "/api",
		CookieSameSite: "Lax",
		TrustedOrigins: []string{"http://127.0.0.1:5173", "http://localhost:5173"},
		RequestTimeout: 5 * time.Second,
	}
	authHandler := authentication.NewHandler(authService, authCfg)

	app := fiber.New()
	api := app.Group("/api")
	authHandler.RegisterRoutes(api)
	handler.RegisterRoutes(
		api,
		authHandler.RequireTrustedOrigin(),
		authHandler.RequireAuth(),
		authentication.RequireAnyRole(user.RoleSportsDirector, user.RoleSuperAdmin),
	)

	seedAndLogin := func(id uuid.UUID, email, name string, role user.Role) string {
		hash, err := auth.Hash("password123")
		if err != nil {
			t.Fatalf("failed to hash password: %v", err)
		}
		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
			VALUES ($1, $2, $3, $4, 'active');
		`, id, email, name, hash)
		if err != nil {
			t.Fatalf("failed to insert user %s: %v", email, err)
		}
		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.user_roles (user_id, role)
			VALUES ($1, $2);
		`, id, role)
		if err != nil {
			t.Fatalf("failed to insert role %s: %v", role, err)
		}

		loginBody := fmt.Sprintf(`{"email":%q,"password":"password123"}`, email)
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(loginBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		res, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("login failed: %v", err)
		}
		defer res.Body.Close()
		for _, c := range res.Cookies() {
			if c.Name == "giki_session" {
				return c.Value
			}
		}
		t.Fatalf("missing giki_session cookie for %s", email)
		return ""
	}

	studentToken := seedAndLogin(studentID, "student.http@giki.edu.pk", "Student HTTP", user.RoleStudent)
	directorToken := seedAndLogin(directorID, "director.http@giki.edu.pk", "Director HTTP", user.RoleSportsDirector)

	// Fund student wallet
	stuWallet, err := walletSvc.CreateWallet(ctx, &studentID, "Student Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create student wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: stuWallet.ID,
		Amount:           500000,
		Type:             "TOP_UP",
		ReferenceID:      "topup-http-student",
		Description:      "HTTP Student Top Up",
	})
	if err != nil {
		t.Fatalf("failed to fund student wallet: %v", err)
	}

	// 1. GET /api/memberships/me without session -> 401
	reqUnauth := httptest.NewRequest(http.MethodGet, "/api/memberships/me", nil)
	reqUnauth.Header.Set("Origin", "http://127.0.0.1:5173")
	resp, err := app.Test(reqUnauth)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated GET /me, got %d", resp.StatusCode)
	}

	// 2. GET /api/memberships/me with student session -> 200, isActive=false initially
	reqAuth := httptest.NewRequest(http.MethodGet, "/api/memberships/me", nil)
	reqAuth.Header.Set("Origin", "http://127.0.0.1:5173")
	reqAuth.AddCookie(&http.Cookie{Name: "giki_session", Value: studentToken})
	resp, err = app.Test(reqAuth)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for authenticated GET /me, got %d", resp.StatusCode)
	}
	var sumResp SummaryResponse
	_ = json.NewDecoder(resp.Body).Decode(&sumResp)
	if sumResp.IsActive {
		t.Fatalf("expected initially inactive")
	}

	// 3. POST /api/memberships/purchase with student session -> 200
	purchasePayload := `{"planType":"STANDARD_30_DAY","idempotencyKey":"http-purchase-test-1"}`
	reqPurchase := httptest.NewRequest(http.MethodPost, "/api/memberships/purchase", bytes.NewBufferString(purchasePayload))
	reqPurchase.Header.Set("Content-Type", "application/json")
	reqPurchase.Header.Set("Origin", "http://127.0.0.1:5173")
	reqPurchase.AddCookie(&http.Cookie{Name: "giki_session", Value: studentToken})
	resp, err = app.Test(reqPurchase)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for purchase, got %d", resp.StatusCode)
	}

	// 4. GET /api/memberships/me after purchase -> 200, isActive=true, daysRemaining=30
	reqAuth2 := httptest.NewRequest(http.MethodGet, "/api/memberships/me", nil)
	reqAuth2.Header.Set("Origin", "http://127.0.0.1:5173")
	reqAuth2.AddCookie(&http.Cookie{Name: "giki_session", Value: studentToken})
	resp, err = app.Test(reqAuth2)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for authenticated GET /me, got %d", resp.StatusCode)
	}
	var sumResp2 SummaryResponse
	_ = json.NewDecoder(resp.Body).Decode(&sumResp2)
	if !sumResp2.IsActive || sumResp2.DaysRemaining != 30 {
		t.Fatalf("expected active with 30 days remaining, got active=%v, days=%d", sumResp2.IsActive, sumResp2.DaysRemaining)
	}

	// 5. GET /api/admin/memberships with student session -> 403 Forbidden
	reqAdminStudent := httptest.NewRequest(http.MethodGet, "/api/admin/memberships", nil)
	reqAdminStudent.Header.Set("Origin", "http://127.0.0.1:5173")
	reqAdminStudent.AddCookie(&http.Cookie{Name: "giki_session", Value: studentToken})
	resp, err = app.Test(reqAdminStudent)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for student accessing admin endpoint, got %d", resp.StatusCode)
	}

	// 6. GET /api/admin/memberships with director session -> 200 OK
	reqAdminDirector := httptest.NewRequest(http.MethodGet, "/api/admin/memberships", nil)
	reqAdminDirector.Header.Set("Origin", "http://127.0.0.1:5173")
	reqAdminDirector.AddCookie(&http.Cookie{Name: "giki_session", Value: directorToken})
	resp, err = app.Test(reqAdminDirector)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for director accessing admin endpoint, got %d", resp.StatusCode)
	}
	var adminList []*MembershipResponse
	_ = json.NewDecoder(resp.Body).Decode(&adminList)
	if len(adminList) != 1 {
		t.Fatalf("expected 1 membership in admin list, got %d", len(adminList))
	}
}
