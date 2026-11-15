//go:build integration

package padel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"giki-sports-backend/internal/authentication"
	"giki-sports-backend/internal/user"
	"giki-sports-backend/internal/wallet"
)

func TestVerifyTicket_HappyPath_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)
	uow := NewUnitOfWork(pool, walletSvc)
	svc := NewService(repo, walletSvc, uow)

	// 1. Seed facility
	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000801")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-arena-ticket', 'Ticket Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	// 2. Seed student Alice & attendant Bob
	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000801")
	bobID := uuid.MustParse("a0000000-0000-0000-0000-000000000802")
	for _, u := range []struct {
		id    uuid.UUID
		email string
		name  string
		role  string
	}{
		{aliceID, "alice.ticket@example.test", "Alice Ticket", "student"},
		{bobID, "bob.attendant@example.test", "Bob Attendant", "sports_director"},
	} {
		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
			VALUES ($1, $2, $3, 'hash', 'active');
		`, u.id, u.email, u.name)
		if err != nil {
			t.Fatalf("failed to seed user %s: %v", u.name, err)
		}
		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.user_roles (user_id, role)
			VALUES ($1, $2);
		`, u.id, u.role)
		if err != nil {
			t.Fatalf("failed to seed role for %s: %v", u.name, err)
		}
	}

	// 3. Fund Alice's wallet
	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create Alice wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing Ticket", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           300000,
		Type:             "TOPUP",
		ReferenceID:      "TOPUP-TICKET-ALICE-1",
		Description:      "Ticket test topup",
	})
	if err != nil {
		t.Fatalf("failed to fund Alice wallet: %v", err)
	}

	// 4. Create slot & confirm booking via Checkout
	startTime := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Hour)
	endTime := startTime.Add(60 * time.Minute)
	createdSlot, err := repo.CreateSlot(ctx, &Slot{
		FacilityID: facID,
		StartTime:  startTime,
		EndTime:    endTime,
		Status:     SlotStatusAvailable,
		PricePaisa: 100000,
	})
	if err != nil {
		t.Fatalf("failed to create slot: %v", err)
	}

	_, err = svc.HoldSlot(ctx, createdSlot.ID, aliceID)
	if err != nil {
		t.Fatalf("failed to hold slot: %v", err)
	}

	bookingConfirm, err := svc.Checkout(ctx, CheckoutRequest{
		SlotID:         createdSlot.ID.String(),
		IdempotencyKey: "idemp-key-ticket-1",
	}, aliceID)
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	checkInCode := bookingConfirm.CheckInCode
	bookingID := uuid.MustParse(bookingConfirm.BookingID)

	// 5. Attendant Bob verifies check-in code (TC-FR-03-03)
	verifyResp, err := svc.VerifyTicket(ctx, VerifyTicketRequest{
		CheckInCode: checkInCode,
		BookingID:   bookingID.String(),
	}, bobID)
	if err != nil {
		t.Fatalf("ticket verification failed: %v", err)
	}

	// 6. Verify returned DTO
	if verifyResp.BookingID != bookingID.String() {
		t.Errorf("expected booking ID %s, got %s", bookingID, verifyResp.BookingID)
	}
	if verifyResp.Status != "CHECKED_IN" {
		t.Errorf("expected status CHECKED_IN, got %s", verifyResp.Status)
	}
	if verifyResp.CheckInCode != checkInCode {
		t.Errorf("expected code %s, got %s", checkInCode, verifyResp.CheckInCode)
	}
	if verifyResp.CheckedInBy != bobID.String() {
		t.Errorf("expected checkedInBy %s, got %s", bobID, verifyResp.CheckedInBy)
	}
	if verifyResp.UserEmail != "alice.ticket@example.test" {
		t.Errorf("expected user email alice.ticket@example.test, got %s", verifyResp.UserEmail)
	}
	if verifyResp.UserName != "Alice Ticket" {
		t.Errorf("expected user name 'Alice Ticket', got %s", verifyResp.UserName)
	}
	if verifyResp.FacilityName != "Ticket Arena" {
		t.Errorf("expected facility name 'Ticket Arena', got %s", verifyResp.FacilityName)
	}

	// 7. Verify Database State
	var dbStatus string
	var dbCheckedInBy uuid.UUID
	var dbCheckedInAt time.Time
	err = pool.QueryRow(ctx, `
		SELECT status, checked_in_by, checked_in_at
		FROM giki_sports.padel_bookings
		WHERE id = $1;
	`, bookingID).Scan(&dbStatus, &dbCheckedInBy, &dbCheckedInAt)
	if err != nil {
		t.Fatalf("failed to query database booking row: %v", err)
	}
	if dbStatus != "CHECKED_IN" {
		t.Errorf("expected db status CHECKED_IN, got %s", dbStatus)
	}
	if dbCheckedInBy != bobID {
		t.Errorf("expected db checked_in_by %s, got %s", bobID, dbCheckedInBy)
	}
	if dbCheckedInAt.IsZero() {
		t.Errorf("expected non-zero checked_in_at timestamp")
	}

	// 8. Verify Audit Trail (FR-09)
	var auditCount int
	err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM giki_sports.audit_logs
		WHERE actor_id = $1 AND action = 'booking.verify_ticket' AND target_id = $2;
	`, bobID, bookingID).Scan(&auditCount)
	if err != nil {
		t.Fatalf("failed to query audit log: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("expected 1 audit log entry for ticket check-in, got %d", auditCount)
	}

	t.Log("Verified (TC-FR-03-03): Attendant Bob successfully verified ticket code; booking status transitioned to CHECKED_IN and recorded in audit log.")
}

func TestVerifyTicket_RecheckAndRejections_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)
	uow := NewUnitOfWork(pool, walletSvc)
	svc := NewService(repo, walletSvc, uow)

	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000901")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-arena-recheck', 'Recheck Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000901")
	bobID := uuid.MustParse("a0000000-0000-0000-0000-000000000902")
	for _, u := range []struct {
		id    uuid.UUID
		email string
		name  string
		role  string
	}{
		{aliceID, "alice.recheck@example.test", "Alice Recheck", "student"},
		{bobID, "bob.recheck@example.test", "Bob Attendant", "sports_director"},
	} {
		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
			VALUES ($1, $2, $3, 'hash', 'active');
		`, u.id, u.email, u.name)
		if err != nil {
			t.Fatalf("failed to seed user %s: %v", u.name, err)
		}
		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.user_roles (user_id, role)
			VALUES ($1, $2);
		`, u.id, u.role)
		if err != nil {
			t.Fatalf("failed to seed role for %s: %v", u.name, err)
		}
	}

	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create Alice wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing Recheck", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           300000,
		Type:             "TOPUP",
		ReferenceID:      "TOPUP-TICKET-RECHECK-1",
		Description:      "Recheck test topup",
	})
	if err != nil {
		t.Fatalf("failed to fund Alice wallet: %v", err)
	}

	startTime := time.Now().Add(4 * time.Hour).UTC().Truncate(time.Hour)
	endTime := startTime.Add(60 * time.Minute)
	createdSlot, err := repo.CreateSlot(ctx, &Slot{
		FacilityID: facID,
		StartTime:  startTime,
		EndTime:    endTime,
		Status:     SlotStatusAvailable,
		PricePaisa: 100000,
	})
	if err != nil {
		t.Fatalf("failed to create slot: %v", err)
	}

	_, err = svc.HoldSlot(ctx, createdSlot.ID, aliceID)
	if err != nil {
		t.Fatalf("failed to hold slot: %v", err)
	}

	bookingConfirm, err := svc.Checkout(ctx, CheckoutRequest{
		SlotID:         createdSlot.ID.String(),
		IdempotencyKey: "idemp-key-ticket-recheck",
	}, aliceID)
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	// 1. Initial verification succeeds
	_, err = svc.VerifyTicket(ctx, VerifyTicketRequest{
		CheckInCode: bookingConfirm.CheckInCode,
	}, bobID)
	if err != nil {
		t.Fatalf("first verification failed: %v", err)
	}

	// 2. Second verification with same code is rejected with ErrTicketAlreadyCheckedIn
	_, err = svc.VerifyTicket(ctx, VerifyTicketRequest{
		CheckInCode: bookingConfirm.CheckInCode,
	}, bobID)
	if !errors.Is(err, ErrTicketAlreadyCheckedIn) {
		t.Fatalf("expected ErrTicketAlreadyCheckedIn on duplicate scan, got %v", err)
	}

	// 3. Verification with non-existent code returns ErrBookingNotFound
	_, err = svc.VerifyTicket(ctx, VerifyTicketRequest{
		CheckInCode: "0000",
	}, bobID)
	if !errors.Is(err, ErrBookingNotFound) {
		t.Fatalf("expected ErrBookingNotFound, got %v", err)
	}

	// 4. Verification on a CANCELLED booking returns ErrBookingCancelled
	cancelledBookingID := uuid.New()
	cancelledCode := "8888"
	cancelledTime := time.Now().UTC()
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.padel_bookings (
			id, slot_id, user_id, status, fee_paisa, idempotency_key, check_in_code, cancelled_at, cancelled_by, cancellation_reason
		) VALUES (
			$1, $2, $3, 'CANCELLED', 100000, 'cancelled-key', $4, $5, $6, 'Rain cancellation'
		);
	`, cancelledBookingID, createdSlot.ID, aliceID, cancelledCode, cancelledTime, bobID)
	if err != nil {
		t.Fatalf("failed to seed cancelled booking: %v", err)
	}

	_, err = svc.VerifyTicket(ctx, VerifyTicketRequest{
		CheckInCode: cancelledCode,
	}, bobID)
	if !errors.Is(err, ErrBookingCancelled) {
		t.Fatalf("expected ErrBookingCancelled, got %v", err)
	}

	t.Log("Verified: Re-check rejected with ErrTicketAlreadyCheckedIn, unknown code with ErrBookingNotFound, cancelled with ErrBookingCancelled.")
}

func TestVerifyTicket_HTTP_RoleAuthorization_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)
	uow := NewUnitOfWork(pool, walletSvc)
	svc := NewService(repo, walletSvc, uow)

	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000a01")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-arena-auth', 'Auth Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000a01")
	bobID := uuid.MustParse("a0000000-0000-0000-0000-000000000a02")
	for _, u := range []struct {
		id    uuid.UUID
		email string
		name  string
		role  user.Role
	}{
		{aliceID, "alice.auth@example.test", "Alice Student", user.RoleStudent},
		{bobID, "bob.director@example.test", "Bob Director", user.RoleSportsDirector},
	} {
		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
			VALUES ($1, $2, $3, 'hash', 'active');
		`, u.id, u.email, u.name)
		if err != nil {
			t.Fatalf("failed to seed user %s: %v", u.name, err)
		}
		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.user_roles (user_id, role)
			VALUES ($1, $2);
		`, u.id, string(u.role))
		if err != nil {
			t.Fatalf("failed to seed role for %s: %v", u.name, err)
		}
	}

	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create Alice wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing Auth", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           300000,
		Type:             "TOPUP",
		ReferenceID:      "TOPUP-AUTH-ALICE-1",
		Description:      "Auth test topup",
	})
	if err != nil {
		t.Fatalf("failed to fund Alice wallet: %v", err)
	}

	startTime := time.Now().Add(5 * time.Hour).UTC().Truncate(time.Hour)
	endTime := startTime.Add(60 * time.Minute)
	createdSlot, err := repo.CreateSlot(ctx, &Slot{
		FacilityID: facID,
		StartTime:  startTime,
		EndTime:    endTime,
		Status:     SlotStatusAvailable,
		PricePaisa: 100000,
	})
	if err != nil {
		t.Fatalf("failed to create slot: %v", err)
	}

	_, err = svc.HoldSlot(ctx, createdSlot.ID, aliceID)
	if err != nil {
		t.Fatalf("failed to hold slot: %v", err)
	}

	bookingConfirm, err := svc.Checkout(ctx, CheckoutRequest{
		SlotID:         createdSlot.ID.String(),
		IdempotencyKey: "idemp-key-auth-check",
	}, aliceID)
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	// Setup full Fiber app with real authentication and role middleware
	handler := NewHandler(svc, 5*time.Second)

	allowOrigin := func(c *fiber.Ctx) error {
		origin := c.Get("Origin")
		if origin != "http://127.0.0.1:5173" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden origin"})
		}
		return c.Next()
	}

	mockAuthUser := func(u *user.User) fiber.Handler {
		return func(c *fiber.Ctx) error {
			if u != nil {
				authentication.SetPrincipalForTest(c, &authentication.AuthenticatedPrincipal{
					User: u,
				})
				return c.Next()
			}
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}
	}

	aliceUser := &user.User{ID: aliceID, Email: "alice.auth@example.test", Roles: []user.Role{user.RoleStudent}}
	bobDirectorUser := &user.User{ID: bobID, Email: "bob.director@example.test", Roles: []user.Role{user.RoleSportsDirector}}

	// App for Alice (student)
	appAlice := fiber.New()
	apiAlice := appAlice.Group("/api")
	handler.RegisterAdminRoutes(
		apiAlice,
		allowOrigin,
		mockAuthUser(aliceUser),
		authentication.RequireAnyRole(user.RoleSportsDirector, user.RoleSuperAdmin),
	)

	// App for Bob (sports_director)
	appBob := fiber.New()
	apiBob := appBob.Group("/api")
	handler.RegisterAdminRoutes(
		apiBob,
		allowOrigin,
		mockAuthUser(bobDirectorUser),
		authentication.RequireAnyRole(user.RoleSportsDirector, user.RoleSuperAdmin),
	)

	// 1. Student Alice calling verify-ticket receives 403 Forbidden
	verifyBody, _ := json.Marshal(VerifyTicketRequest{
		CheckInCode: bookingConfirm.CheckInCode,
	})
	reqAlice := httptest.NewRequest("POST", "/api/padel/bookings/verify-ticket", bytes.NewReader(verifyBody))
	reqAlice.Header.Set("Origin", "http://127.0.0.1:5173")
	reqAlice.Header.Set("Content-Type", "application/json")
	respAlice, err := appAlice.Test(reqAlice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if respAlice.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Student Alice calling verify-ticket, got %d", respAlice.StatusCode)
	}

	// 2. Director Bob calling verify-ticket receives 200 OK with CHECKED_IN status
	reqBob := httptest.NewRequest("POST", "/api/padel/bookings/verify-ticket", bytes.NewReader(verifyBody))
	reqBob.Header.Set("Origin", "http://127.0.0.1:5173")
	reqBob.Header.Set("Content-Type", "application/json")
	respBob, err := appBob.Test(reqBob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if respBob.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(respBob.Body)
		t.Fatalf("expected 200 OK for Director Bob, got %d: %s", respBob.StatusCode, string(b))
	}

	var verifiedResp VerifyTicketResponse
	_ = json.NewDecoder(respBob.Body).Decode(&verifiedResp)
	if verifiedResp.Status != "CHECKED_IN" {
		t.Errorf("expected status CHECKED_IN, got %s", verifiedResp.Status)
	}
	if verifiedResp.CheckedInBy != bobID.String() {
		t.Errorf("expected checkedInBy %s, got %s", bobID, verifiedResp.CheckedInBy)
	}

	t.Log("Verified: Student calling verify-ticket is rejected with 403 Forbidden; Sports Director receives 200 OK with confirmed CHECKED_IN status.")
}
