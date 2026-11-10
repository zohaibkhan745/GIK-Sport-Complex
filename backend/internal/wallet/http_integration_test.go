//go:build integration

package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"giki-sports-backend/internal/auth"
	"giki-sports-backend/internal/authentication"
	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/session"
	"giki-sports-backend/internal/user"
)

// TestWalletMeHTTP_Integration verifies GET /api/wallet/me against a disposable PostgreSQL database
// with real session authentication middleware.
func TestWalletMeHTTP_Integration(t *testing.T) {
	svc, cleanup := setupWalletIntegrationTest(t)
	defer cleanup()

	pool := svc.pool
	ctx := context.Background()

	// 1. Initialize real authentication stack
	userRepo := user.NewRepository(pool)
	sessionRepo := session.NewRepository(pool)

	authService, err := authentication.NewService(userRepo, sessionRepo)
	if err != nil {
		t.Fatalf("failed to initialize auth service: %v", err)
	}

	authCfg := config.AuthConfig{
		CookieName:         "giki_session",
		CookieSecure:       false,
		CookiePath:         "/api",
		CookieSameSite:     "Lax",
		TrustedOrigins:     []string{"http://127.0.0.1:5173", "http://localhost:5173"},
		LoginMaxConcurrent: 4,
		RequestTimeout:     5 * time.Second,
		LoginRateLimit:     50,
		LoginRateWindow:    time.Minute,
	}

	authHandler := authentication.NewHandler(authService, authCfg)
	walletHandler := NewHandler(svc, 5*time.Second)

	app := fiber.New()
	api := app.Group("/api")

	authHandler.RegisterRoutes(api)
	walletHandler.RegisterRoutes(api, authHandler.RequireAuth())

	// Helper to seed a user and login to obtain session cookie
	seedAndLoginUser := func(email, name string, role user.Role) (uuid.UUID, string) {
		userID := uuid.New()
		passHash, err := auth.Hash("testpassword123")
		if err != nil {
			t.Fatalf("failed to hash password: %v", err)
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.users (id, email, display_name, password_hash)
			VALUES ($1, $2, $3, $4);
		`, userID, email, name, passHash)
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

		// Perform login through real auth endpoint
		loginPayload := fmt.Sprintf(`{"email":%q,"password":"testpassword123"}`, email)
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(loginPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")

		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("login request failed for %s: %v", email, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("login failed for %s (status %d): %s", email, resp.StatusCode, string(body))
		}

		for _, c := range resp.Cookies() {
			if c.Name == "giki_session" {
				return userID, c.Value
			}
		}
		t.Fatalf("session cookie not found after login for %s", email)
		return uuid.Nil, ""
	}

	// Helper to count rows across all wallet ledger tables
	countWalletRows := func() (wallets, txs, entries int64) {
		_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM giki_sports.wallets;").Scan(&wallets)
		_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM giki_sports.wallet_transactions;").Scan(&txs)
		_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM giki_sports.wallet_ledger_entries;").Scan(&entries)
		return
	}

	// Seed system wallets for ledger funding and collections
	sysLiability, err := svc.CreateWallet(ctx, nil, "System Cash Liability", WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create sys liability wallet: %v", err)
	}
	sysRevenue, err := svc.CreateWallet(ctx, nil, "System Sports Revenue", WalletTypeRevenue, "PKR")
	if err != nil {
		t.Fatalf("failed to create sys revenue wallet: %v", err)
	}

	// -------------------------------------------------------------------------
	// 1. Signed-out rejection (401 Unauthorized)
	// -------------------------------------------------------------------------
	t.Run("Signed-out rejection (401)", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/wallet/me", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("unexpected request error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}

		cacheControl := resp.Header.Get("Cache-Control")
		if !strings.Contains(cacheControl, "no-store") {
			t.Errorf("expected Cache-Control to contain 'no-store', got %q", cacheControl)
		}

		var errBody ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
			t.Fatalf("failed to decode error body: %v", err)
		}
		if !strings.Contains(errBody.Error, "unauthorized") {
			t.Errorf("expected unauthorized message in body, got %q", errBody.Error)
		}
	})

	// -------------------------------------------------------------------------
	// 2. User without a wallet returns 200 with wallet null and activity empty
	// -------------------------------------------------------------------------
	userNoWalletID, userNoWalletCookie := seedAndLoginUser("nowallet@giki.test", "No Wallet User", user.RoleStudent)

	t.Run("No wallet yet returns 200 with wallet null and activity empty array", func(t *testing.T) {
		initialWallets, _, _ := countWalletRows()

		req := httptest.NewRequest("GET", "/api/wallet/me", nil)
		req.AddCookie(&http.Cookie{Name: "giki_session", Value: userNoWalletCookie})

		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("unexpected request error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		cacheControl := resp.Header.Get("Cache-Control")
		if !strings.Contains(cacheControl, "no-store") {
			t.Errorf("expected Cache-Control: no-store, got %q", cacheControl)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}

		// Exact JSON structure check: "wallet":null and "activity":[]
		bodyStr := string(bodyBytes)
		if !strings.Contains(bodyStr, `"wallet":null`) {
			t.Errorf("expected body to contain '\"wallet\":null', got: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, `"activity":[]`) {
			t.Errorf("expected body to contain '\"activity\":[]', got: %s", bodyStr)
		}

		var meResp WalletMeResponse
		if err := json.Unmarshal(bodyBytes, &meResp); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}
		if meResp.Wallet != nil {
			t.Errorf("expected Wallet to be nil, got %+v", meResp.Wallet)
		}
		if meResp.Activity == nil || len(meResp.Activity) != 0 {
			t.Errorf("expected Activity to be empty non-nil slice, got %+v", meResp.Activity)
		}

		// Verify GET did NOT create a wallet row in the database
		finalWallets, _, _ := countWalletRows()
		if finalWallets != initialWallets {
			t.Errorf("expected wallet count %d unchanged, got %d", initialWallets, finalWallets)
		}

		var count int
		_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM giki_sports.wallets WHERE user_id = $1;", userNoWalletID).Scan(&count)
		if count != 0 {
			t.Errorf("expected 0 wallet rows for user %s, got %d", userNoWalletID, count)
		}
	})

	// -------------------------------------------------------------------------
	// 3. User with active wallet and zero balance
	// -------------------------------------------------------------------------
	userZeroBalID, userZeroBalCookie := seedAndLoginUser("zerobal@giki.test", "Zero Balance User", user.RoleStudent)
	_, err = svc.CreateWallet(ctx, &userZeroBalID, "Zero Balance Personal Wallet", WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet for zero bal user: %v", err)
	}

	t.Run("Existing wallet with zero balance", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/wallet/me", nil)
		req.AddCookie(&http.Cookie{Name: "giki_session", Value: userZeroBalCookie})

		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("unexpected request error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}

		bodyStr := string(bodyBytes)
		if !strings.Contains(bodyStr, `"balancePaisa":"0"`) {
			t.Errorf("expected '\"balancePaisa\":\"0\"' in raw body, got: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, `"activity":[]`) {
			t.Errorf("expected '\"activity\":[]' in raw body, got: %s", bodyStr)
		}

		var meResp WalletMeResponse
		if err := json.Unmarshal(bodyBytes, &meResp); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		if meResp.Wallet == nil {
			t.Fatal("expected non-nil Wallet in response")
		}
		if meResp.Wallet.Status != "ACTIVE" {
			t.Errorf("expected status ACTIVE, got %s", meResp.Wallet.Status)
		}
		if meResp.Wallet.Currency != "PKR" {
			t.Errorf("expected currency PKR, got %s", meResp.Wallet.Currency)
		}
		if meResp.Wallet.BalancePaisa != "0" {
			t.Errorf("expected balancePaisa '0', got %s", meResp.Wallet.BalancePaisa)
		}
		if len(meResp.Activity) != 0 {
			t.Errorf("expected empty activity, got %d items", len(meResp.Activity))
		}
	})

	// -------------------------------------------------------------------------
	// 4. Isolation between two signed-in users with transaction activity
	// -------------------------------------------------------------------------
	userAID, userACookie := seedAndLoginUser("usera@giki.test", "User A", user.RoleStudent)
	userAWallet, err := svc.CreateWallet(ctx, &userAID, "User A Personal", WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet for User A: %v", err)
	}

	userBID, userBCookie := seedAndLoginUser("userb@giki.test", "User B", user.RoleFaculty)
	userBWallet, err := svc.CreateWallet(ctx, &userBID, "User B Personal", WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet for User B: %v", err)
	}

	// Post transfers:
	// 1) Topup User A: 50,000 paisa (Rs. 500)
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   sysLiability.ID,
		ReceiverWalletID: userAWallet.ID,
		Amount:           50000,
		Type:             "TOPUP",
		ReferenceID:      "topup-user-a-isolated-001",
	})
	if err != nil {
		t.Fatalf("failed to topup User A: %v", err)
	}

	// 2) Topup User B: 100,000 paisa (Rs. 1,000)
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   sysLiability.ID,
		ReceiverWalletID: userBWallet.ID,
		Amount:           100000,
		Type:             "TOPUP",
		ReferenceID:      "topup-user-b-isolated-001",
	})
	if err != nil {
		t.Fatalf("failed to topup User B: %v", err)
	}

	// 3) Debit User B for booking: 35,000 paisa (Rs. 350)
	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   userBWallet.ID,
		ReceiverWalletID: sysRevenue.ID,
		Amount:           35000,
		Type:             "PADEL_BOOKING",
		ReferenceID:      "booking-user-b-isolated-001",
	})
	if err != nil {
		t.Fatalf("failed to debit User B: %v", err)
	}

	t.Run("Isolation between two signed-in users", func(t *testing.T) {
		// User A query
		reqA := httptest.NewRequest("GET", "/api/wallet/me", nil)
		reqA.AddCookie(&http.Cookie{Name: "giki_session", Value: userACookie})

		respA, err := app.Test(reqA, 5000)
		if err != nil {
			t.Fatalf("unexpected request error for User A: %v", err)
		}
		defer respA.Body.Close()

		if respA.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK for User A, got %d", respA.StatusCode)
		}

		var meRespA WalletMeResponse
		if err := json.NewDecoder(respA.Body).Decode(&meRespA); err != nil {
			t.Fatalf("failed to decode User A response: %v", err)
		}

		if meRespA.Wallet.BalancePaisa != "50000" {
			t.Errorf("expected User A balancePaisa '50000', got %s", meRespA.Wallet.BalancePaisa)
		}
		if len(meRespA.Activity) != 1 {
			t.Fatalf("expected User A to have 1 activity item, got %d", len(meRespA.Activity))
		}
		if meRespA.Activity[0].AmountPaisa != "50000" {
			t.Errorf("expected User A activity amount '50000', got %s", meRespA.Activity[0].AmountPaisa)
		}
		if meRespA.Activity[0].Type != "TOPUP" {
			t.Errorf("expected User A activity type TOPUP, got %s", meRespA.Activity[0].Type)
		}

		// User B query
		reqB := httptest.NewRequest("GET", "/api/wallet/me", nil)
		reqB.AddCookie(&http.Cookie{Name: "giki_session", Value: userBCookie})

		respB, err := app.Test(reqB, 5000)
		if err != nil {
			t.Fatalf("unexpected request error for User B: %v", err)
		}
		defer respB.Body.Close()

		if respB.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK for User B, got %d", respB.StatusCode)
		}

		var meRespB WalletMeResponse
		if err := json.NewDecoder(respB.Body).Decode(&meRespB); err != nil {
			t.Fatalf("failed to decode User B response: %v", err)
		}

		// Balance should be 100000 - 35000 = 65000
		if meRespB.Wallet.BalancePaisa != "65000" {
			t.Errorf("expected User B balancePaisa '65000', got %s", meRespB.Wallet.BalancePaisa)
		}
		if len(meRespB.Activity) != 2 {
			t.Fatalf("expected User B to have 2 activity items, got %d", len(meRespB.Activity))
		}
		// Ordered newest first: booking (-35000) then topup (+100000)
		if meRespB.Activity[0].AmountPaisa != "-35000" {
			t.Errorf("expected newest entry to be '-35000', got %s", meRespB.Activity[0].AmountPaisa)
		}
		if meRespB.Activity[0].Type != "PADEL_BOOKING" {
			t.Errorf("expected newest entry type PADEL_BOOKING, got %s", meRespB.Activity[0].Type)
		}
		if meRespB.Activity[1].AmountPaisa != "100000" {
			t.Errorf("expected second entry to be '100000', got %s", meRespB.Activity[1].AmountPaisa)
		}
		if meRespB.Activity[1].Type != "TOPUP" {
			t.Errorf("expected second entry type TOPUP, got %s", meRespB.Activity[1].Type)
		}

		// Verify zero cross-exposure: User A has none of User B's transactions
		for _, itA := range meRespA.Activity {
			for _, itB := range meRespB.Activity {
				if itA.EntryID == itB.EntryID || itA.TransactionID == itB.TransactionID {
					t.Errorf("data leak: User A saw User B entry/transaction ID (%s / %s)", itA.EntryID, itA.TransactionID)
				}
			}
		}
	})

	// -------------------------------------------------------------------------
	// 5. Ignored spoofed headers (X-User-ID, X-Wallet-ID, etc.)
	// -------------------------------------------------------------------------
	t.Run("Ignored spoofed headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/wallet/me", nil)
		// Authenticated as User A
		req.AddCookie(&http.Cookie{Name: "giki_session", Value: userACookie})

		// Attacker injects spoofed headers attempting to read User B's balance
		req.Header.Set("X-User-ID", userBID.String())
		req.Header.Set("X-Wallet-ID", userBWallet.ID.String())
		req.Header.Set("User-ID", userBID.String())

		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("unexpected request error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var meResp WalletMeResponse
		if err := json.NewDecoder(resp.Body).Decode(&meResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Balance MUST be User A's (50000), never User B's (65000)
		if meResp.Wallet.BalancePaisa != "50000" {
			t.Errorf("spoofed header bypass: expected User A balance '50000', got %s", meResp.Wallet.BalancePaisa)
		}
		if len(meResp.Activity) != 1 || meResp.Activity[0].AmountPaisa != "50000" {
			t.Errorf("spoofed header returned unauthorized activity: %+v", meResp.Activity)
		}
	})

	// -------------------------------------------------------------------------
	// 6. Exact decimal string serialization for amounts above JS safe integer
	// -------------------------------------------------------------------------
	userLargeBalID, userLargeBalCookie := seedAndLoginUser("largebal@giki.test", "Large Balance User", user.RoleStudent)
	userLargeWallet, err := svc.CreateWallet(ctx, &userLargeBalID, "Large Balance Wallet", WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet for large balance user: %v", err)
	}

	// Number.MAX_SAFE_INTEGER is 9007199254740991 (2^53 - 1).
	// We post an amount exceeding it: 9007199254740992 (2^53)
	const largeAmount int64 = 9007199254740992

	_, err = svc.PostTransaction(ctx, TransferParams{
		SenderWalletID:   sysLiability.ID,
		ReceiverWalletID: userLargeWallet.ID,
		Amount:           largeAmount,
		Type:             "TOPUP",
		ReferenceID:      "large-amount-transfer-001",
	})
	if err != nil {
		t.Fatalf("failed to post large amount transfer: %v", err)
	}

	t.Run("Exact string serialization of amounts above JavaScript safe integer range (> 2^53 - 1)", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/wallet/me", nil)
		req.AddCookie(&http.Cookie{Name: "giki_session", Value: userLargeBalCookie})

		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("unexpected request error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}

		rawBody := string(bodyBytes)
		expectedStr := strconv.FormatInt(largeAmount, 10) // "9007199254740992"

		// Verify exact string representation in JSON output (not numeric, not floating point)
		expectedBalanceSubstring := fmt.Sprintf(`"balancePaisa":%q`, expectedStr)
		if !strings.Contains(rawBody, expectedBalanceSubstring) {
			t.Errorf("expected raw JSON to contain %s, got: %s", expectedBalanceSubstring, rawBody)
		}

		expectedAmountSubstring := fmt.Sprintf(`"amountPaisa":%q`, expectedStr)
		if !strings.Contains(rawBody, expectedAmountSubstring) {
			t.Errorf("expected raw JSON activity to contain %s, got: %s", expectedAmountSubstring, rawBody)
		}

		var meResp WalletMeResponse
		if err := json.Unmarshal(bodyBytes, &meResp); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		if meResp.Wallet.BalancePaisa != expectedStr {
			t.Errorf("expected balancePaisa %s, got %s", expectedStr, meResp.Wallet.BalancePaisa)
		}
		if len(meResp.Activity) != 1 || meResp.Activity[0].AmountPaisa != expectedStr {
			t.Errorf("expected activity amount %s, got %+v", expectedStr, meResp.Activity)
		}
	})

	// -------------------------------------------------------------------------
	// 7. Verify GET creates zero database records
	// -------------------------------------------------------------------------
	t.Run("Verify GET creates zero database records", func(t *testing.T) {
		wBefore, tBefore, eBefore := countWalletRows()

		// Execute GET for existing user
		req1 := httptest.NewRequest("GET", "/api/wallet/me", nil)
		req1.AddCookie(&http.Cookie{Name: "giki_session", Value: userACookie})
		resp1, err := app.Test(req1, 5000)
		if err != nil {
			t.Fatalf("req1 error: %v", err)
		}
		resp1.Body.Close()

		// Execute GET for user without wallet
		req2 := httptest.NewRequest("GET", "/api/wallet/me", nil)
		req2.AddCookie(&http.Cookie{Name: "giki_session", Value: userNoWalletCookie})
		resp2, err := app.Test(req2, 5000)
		if err != nil {
			t.Fatalf("req2 error: %v", err)
		}
		resp2.Body.Close()

		wAfter, tAfter, eAfter := countWalletRows()

		if wBefore != wAfter {
			t.Errorf("wallets row count changed: before %d, after %d", wBefore, wAfter)
		}
		if tBefore != tAfter {
			t.Errorf("transactions row count changed: before %d, after %d", tBefore, tAfter)
		}
		if eBefore != eAfter {
			t.Errorf("ledger entries row count changed: before %d, after %d", eBefore, eAfter)
		}
	})

	// -------------------------------------------------------------------------
	// 8. Privacy: Counterparty, system accounts, and internal refs not exposed
	// -------------------------------------------------------------------------
	t.Run("Privacy check: no counterparty, internal ref, or system balances exposed", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/wallet/me", nil)
		req.AddCookie(&http.Cookie{Name: "giki_session", Value: userBCookie})

		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("unexpected request error: %v", err)
		}
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}

		rawBody := string(bodyBytes)

		// System account names must NOT appear
		if strings.Contains(rawBody, "SYS_REVENUE") || strings.Contains(rawBody, "SYS_LIABILITY") {
			t.Errorf("privacy breach: system account names leaked in JSON: %s", rawBody)
		}

		// Counterparty wallet IDs must NOT appear
		if strings.Contains(rawBody, sysLiability.ID.String()) || strings.Contains(rawBody, sysRevenue.ID.String()) {
			t.Errorf("privacy breach: counterparty wallet IDs leaked in JSON: %s", rawBody)
		}

		// Internal transaction reference strings must NOT appear
		if strings.Contains(rawBody, "booking-user-b-isolated-001") || strings.Contains(rawBody, "topup-user-b-isolated-001") {
			t.Errorf("privacy breach: internal transaction reference leaked in JSON: %s", rawBody)
		}
	})

	// -------------------------------------------------------------------------
	// 9. Activity order newest first and capped at 50 items
	// -------------------------------------------------------------------------
	userManyID, userManyCookie := seedAndLoginUser("manytx@giki.test", "Many Tx User", user.RoleStudent)
	userManyWallet, err := svc.CreateWallet(ctx, &userManyID, "Many Tx Wallet", WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet for many tx user: %v", err)
	}

	// Post 55 transfers to this user
	for i := 1; i <= 55; i++ {
		_, err = svc.PostTransaction(ctx, TransferParams{
			SenderWalletID:   sysLiability.ID,
			ReceiverWalletID: userManyWallet.ID,
			Amount:           int64(i * 100),
			Type:             "TOPUP",
			ReferenceID:      fmt.Sprintf("many-tx-%03d", i),
		})
		if err != nil {
			t.Fatalf("failed to post transfer %d: %v", i, err)
		}
	}

	t.Run("Activity capped at 50 items newest first", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/wallet/me", nil)
		req.AddCookie(&http.Cookie{Name: "giki_session", Value: userManyCookie})

		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("unexpected request error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var meResp WalletMeResponse
		if err := json.NewDecoder(resp.Body).Decode(&meResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(meResp.Activity) != 50 {
			t.Fatalf("expected exactly 50 activity items, got %d", len(meResp.Activity))
		}

		// Newest entry should be transfer #55 (amount: 5500)
		if meResp.Activity[0].AmountPaisa != "5500" {
			t.Errorf("expected newest entry amount to be '5500', got %s", meResp.Activity[0].AmountPaisa)
		}

		// Assert chronological descending order (or equal for same microsecond, with stable ID tiebreaker)
		for i := 0; i < len(meResp.Activity)-1; i++ {
			t1, err1 := time.Parse(time.RFC3339, meResp.Activity[i].CreatedAt)
			t2, err2 := time.Parse(time.RFC3339, meResp.Activity[i+1].CreatedAt)
			if err1 == nil && err2 == nil {
				if t1.Before(t2) {
					t.Errorf("activity out of order: entry %d (%s) before entry %d (%s)", i, t1, i+1, t2)
				}
			}
		}
	})
}
