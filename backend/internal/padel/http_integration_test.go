//go:build integration

package padel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	"giki-sports-backend/internal/wallet"
)

func TestPadelHTTP_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

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
	walletSvc := wallet.NewService(pool)
	padelUoW := NewUnitOfWork(pool, walletSvc)
	padelService := NewService(repo, walletSvc, padelUoW)
	padelHandler := NewHandler(padelService, 5*time.Second)

	app := fiber.New()
	api := app.Group("/api")

	authHandler.RegisterRoutes(api)
	padelHandler.RegisterPublicRoutes(api)
	padelHandler.RegisterAuthenticatedRoutes(
		api,
		authHandler.RequireTrustedOrigin(),
		authHandler.RequireAuth(),
	)
	padelHandler.RegisterAdminRoutes(
		api,
		authHandler.RequireTrustedOrigin(),
		authHandler.RequireAuth(),
		authentication.RequireAnyRole(user.RoleSportsDirector, user.RoleSuperAdmin),
	)

	// Seed test facility: Campus Padel Court 1
	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000001")
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'campus-padel-court-1', 'Campus Padel Court 1', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

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

		var cookieStr string
		for _, c := range resp.Cookies() {
			if c.Name == "giki_session" {
				cookieStr = fmt.Sprintf("giki_session=%s", c.Value)
				break
			}
		}
		if cookieStr == "" {
			t.Fatalf("missing giki_session cookie in login response for %s", email)
		}

		return userID, cookieStr
	}

	studentUserID, studentCookie := seedAndLoginUser("student@example.test", "Alice Student", user.RoleStudent)
	_, directorCookie := seedAndLoginUser("director@example.test", "Director Sports", user.RoleSportsDirector)

	t.Run("public can list padel facilities", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/padel/facilities", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var facs []FacilitySummaryDTO
		_ = json.NewDecoder(resp.Body).Decode(&facs)
		if len(facs) == 0 {
			t.Fatalf("expected at least 1 facility, got %d", len(facs))
		}
		if facs[0].ID != facID.String() {
			t.Errorf("expected facility ID %s, got %s", facID, facs[0].ID)
		}
	})

	t.Run("unauthenticated user cannot call generate slots (401 Unauthorized)", func(t *testing.T) {
		body, _ := json.Marshal(GenerateSlotsRequest{
			FacilityID: facID.String(),
			Date:       "2026-11-20",
		})
		req := httptest.NewRequest("POST", "/api/admin/padel/slots/generate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("student cannot call generate slots (403 Forbidden)", func(t *testing.T) {
		body, _ := json.Marshal(GenerateSlotsRequest{
			FacilityID: facID.String(),
			Date:       "2026-11-20",
		})
		req := httptest.NewRequest("POST", "/api/admin/padel/slots/generate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", studentCookie)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", resp.StatusCode)
		}
	})

	t.Run("sports director without trusted origin is rejected (403 Forbidden)", func(t *testing.T) {
		body, _ := json.Marshal(GenerateSlotsRequest{
			FacilityID: facID.String(),
			Date:       "2026-11-20",
		})
		req := httptest.NewRequest("POST", "/api/admin/padel/slots/generate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://evil-attacker.example")
		req.Header.Set("Cookie", directorCookie)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for untrusted origin, got %d", resp.StatusCode)
		}
	})

	var createdSlotID string
	t.Run("sports director with trusted origin generates slots successfully (201 Created)", func(t *testing.T) {
		body, _ := json.Marshal(GenerateSlotsRequest{
			FacilityID:          facID.String(),
			Date:                "2026-11-20",
			StartHour:           8,
			EndHour:             23,
			SlotDurationMinutes: 60,
			PricePaisa:          "150000",
		})
		req := httptest.NewRequest("POST", "/api/admin/padel/slots/generate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", directorCookie)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 201 Created, got %d: %s", resp.StatusCode, string(b))
		}

		var genResp GenerateSlotsResponse
		_ = json.NewDecoder(resp.Body).Decode(&genResp)
		if genResp.SlotsCreated != 15 {
			t.Fatalf("expected 15 slots created, got %d", genResp.SlotsCreated)
		}
		if len(genResp.Slots) != 15 {
			t.Fatalf("expected 15 slots returned, got %d", len(genResp.Slots))
		}
		createdSlotID = genResp.Slots[0].ID
	})

	t.Run("public GET /api/padel/slots?date=2026-11-20 returns generated slots with exact paisa strings", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/padel/slots?date=2026-11-20", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var slotsResp SlotsResponse
		_ = json.NewDecoder(resp.Body).Decode(&slotsResp)
		if len(slotsResp.Slots) != 15 {
			t.Fatalf("expected 15 slots, got %d", len(slotsResp.Slots))
		}
		firstSlot := slotsResp.Slots[0]
		if firstSlot.PricePaisa != "150000" {
			t.Errorf("expected pricePaisa string '150000', got %q", firstSlot.PricePaisa)
		}
		if firstSlot.Status != "AVAILABLE" {
			t.Errorf("expected status 'AVAILABLE', got %q", firstSlot.Status)
		}
	})

	t.Run("sports director can update slot price via PATCH /api/admin/padel/slots/:id", func(t *testing.T) {
		newPrice := "180000"
		body, _ := json.Marshal(UpdateSlotRequest{
			PricePaisa: &newPrice,
		})
		req := httptest.NewRequest("PATCH", "/api/admin/padel/slots/"+createdSlotID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", directorCookie)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(b))
		}

		var updated SlotDTO
		_ = json.NewDecoder(resp.Body).Decode(&updated)
		if updated.PricePaisa != "180000" {
			t.Errorf("expected updated pricePaisa '180000', got %q", updated.PricePaisa)
		}
	})

	// --- Step 12D Hold State Machine Verification ---
	t.Run("unauthenticated POST /api/padel/slots/:id/hold returns 401 Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/padel/slots/"+createdSlotID+"/hold", nil)
		req.Header.Set("Origin", "http://127.0.0.1:5173")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("untrusted origin for hold returns 403 Forbidden", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/padel/slots/"+createdSlotID+"/hold", nil)
		req.Header.Set("Origin", "http://malicious-site.example")
		req.Header.Set("Cookie", studentCookie)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for untrusted origin, got %d", resp.StatusCode)
		}
	})

	t.Run("Student Alice successfully acquires 5-minute hold on available slot (200 OK)", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/padel/slots/"+createdSlotID+"/hold", nil)
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", studentCookie)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(b))
		}

		var holdResp HoldSlotResponse
		_ = json.NewDecoder(resp.Body).Decode(&holdResp)
		if holdResp.Status != "HELD" {
			t.Errorf("expected status HELD, got %s", holdResp.Status)
		}
		if holdResp.RemainingSeconds <= 0 || holdResp.RemainingSeconds > 300 {
			t.Errorf("expected remainingSeconds between 1 and 300, got %d", holdResp.RemainingSeconds)
		}
		if holdResp.PricePaisa != "180000" {
			t.Errorf("expected pricePaisa '180000', got %q", holdResp.PricePaisa)
		}
	})

	t.Run("Another student Bob attempting to hold Alice's active slot receives 409 Conflict", func(t *testing.T) {
		_, bobCookie := seedAndLoginUser("bob@example.test", "Bob Student", user.RoleStudent)

		req := httptest.NewRequest("POST", "/api/padel/slots/"+createdSlotID+"/hold", nil)
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", bobCookie)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("expected 409 Conflict, got %d", resp.StatusCode)
		}

		var errResp ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "slot is currently held by another user" {
			t.Errorf("expected error 'slot is currently held by another user', got %q", errResp.Error)
		}
	})

	t.Run("Bob cannot release Alice's hold (403 Forbidden)", func(t *testing.T) {
		_, bobCookie := seedAndLoginUser("bob-releaser@example.test", "Bob Releaser", user.RoleStudent)

		req := httptest.NewRequest("POST", "/api/padel/slots/"+createdSlotID+"/release-hold", nil)
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", bobCookie)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", resp.StatusCode)
		}
	})

	t.Run("Alice voluntarily releases her hold (200 OK)", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/padel/slots/"+createdSlotID+"/release-hold", nil)
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", studentCookie)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(b))
		}

		var relResp ReleaseHoldResponse
		_ = json.NewDecoder(resp.Body).Decode(&relResp)
		if relResp.Status != "AVAILABLE" {
			t.Errorf("expected status AVAILABLE, got %s", relResp.Status)
		}
	})

	t.Run("Full HTTP Checkout Flow with Wallet Debit and Replay (POST /api/padel/checkout)", func(t *testing.T) {
		// 1. Re-acquire hold for Alice
		reqHold := httptest.NewRequest("POST", "/api/padel/slots/"+createdSlotID+"/hold", nil)
		reqHold.Header.Set("Origin", "http://127.0.0.1:5173")
		reqHold.Header.Set("Cookie", studentCookie)
		respHold, err := app.Test(reqHold)
		if err != nil {
			t.Fatalf("unexpected hold error: %v", err)
		}
		if respHold.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK on hold, got %d", respHold.StatusCode)
		}

		// 2. Checkout without funds returns 400 Bad Request
		checkoutPayload, _ := json.Marshal(CheckoutRequest{
			SlotID:         createdSlotID,
			IdempotencyKey: "http-checkout-alice-key",
		})
		reqCheckoutNoFunds := httptest.NewRequest("POST", "/api/padel/checkout", bytes.NewReader(checkoutPayload))
		reqCheckoutNoFunds.Header.Set("Origin", "http://127.0.0.1:5173")
		reqCheckoutNoFunds.Header.Set("Content-Type", "application/json")
		reqCheckoutNoFunds.Header.Set("Cookie", studentCookie)
		respNoFunds, err := app.Test(reqCheckoutNoFunds)
		if err != nil {
			t.Fatalf("unexpected checkout error: %v", err)
		}
		if respNoFunds.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request due to insufficient funds, got %d", respNoFunds.StatusCode)
		}

		// 3. Fund Alice's wallet
		aliceWallet, err := walletSvc.CreateWallet(ctx, &studentUserID, "Alice Personal", wallet.WalletTypePersonal, "PKR")
		if err != nil {
			t.Fatalf("failed to create wallet for Alice: %v", err)
		}
		clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Clearing HTTP", wallet.WalletTypeLiability, "PKR")
		if err != nil {
			t.Fatalf("failed to create clearing wallet: %v", err)
		}
		_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
			SenderWalletID:   clearingWallet.ID,
			ReceiverWalletID: aliceWallet.ID,
			Amount:           500000,
			Type:             "TOPUP",
			ReferenceID:      "TOPUP-HTTP-ALICE",
			Description:      "HTTP topup",
		})
		if err != nil {
			t.Fatalf("failed to fund Alice: %v", err)
		}

		// 4. Successful Checkout with funds
		reqCheckoutSuccess := httptest.NewRequest("POST", "/api/padel/checkout", bytes.NewReader(checkoutPayload))
		reqCheckoutSuccess.Header.Set("Origin", "http://127.0.0.1:5173")
		reqCheckoutSuccess.Header.Set("Content-Type", "application/json")
		reqCheckoutSuccess.Header.Set("Cookie", studentCookie)
		respSuccess, err := app.Test(reqCheckoutSuccess)
		if err != nil {
			t.Fatalf("unexpected checkout error: %v", err)
		}
		if respSuccess.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(respSuccess.Body)
			t.Fatalf("expected 200 OK on checkout, got %d: %s", respSuccess.StatusCode, string(b))
		}

		var confirm BookingConfirmationDTO
		if err := json.NewDecoder(respSuccess.Body).Decode(&confirm); err != nil {
			t.Fatalf("failed to decode confirmation DTO: %v", err)
		}
		if confirm.AlreadyBooked {
			t.Errorf("expected AlreadyBooked to be false on initial checkout")
		}
		if len(confirm.CheckInCode) != 4 {
			t.Errorf("expected 4-digit check-in code, got %s", confirm.CheckInCode)
		}

		// 5. Replay with identical key returns 200 OK with AlreadyBooked: true
		reqReplay := httptest.NewRequest("POST", "/api/padel/checkout", bytes.NewReader(checkoutPayload))
		reqReplay.Header.Set("Origin", "http://127.0.0.1:5173")
		reqReplay.Header.Set("Content-Type", "application/json")
		reqReplay.Header.Set("Cookie", studentCookie)
		respReplay, err := app.Test(reqReplay)
		if err != nil {
			t.Fatalf("unexpected replay error: %v", err)
		}
		if respReplay.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK on replay, got %d", respReplay.StatusCode)
		}

		var replayConfirm BookingConfirmationDTO
		if err := json.NewDecoder(respReplay.Body).Decode(&replayConfirm); err != nil {
			t.Fatalf("failed to decode replay DTO: %v", err)
		}
		if !replayConfirm.AlreadyBooked {
			t.Errorf("expected AlreadyBooked to be true on replay")
		}
		if replayConfirm.BookingID != confirm.BookingID {
			t.Errorf("expected matching BookingID %s, got %s", confirm.BookingID, replayConfirm.BookingID)
		}
	})
}
