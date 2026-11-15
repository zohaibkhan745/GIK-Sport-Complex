//go:build integration

package padel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// TestAdminCancelBooking_HappyPath_Integration tests the complete administrative cancellation
// and 100% wallet ledger refund workflow (FR-06, TC-FR-06-02, QR-03).
func TestAdminCancelBooking_HappyPath_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)
	uow := NewUnitOfWork(pool, walletSvc)
	svc := NewService(repo, walletSvc, uow)

	// 1. Seed facility
	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000901")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-cancel-arena', 'Cancellation Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	// 2. Seed student Alice & sports director Bob
	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000901")
	bobID := uuid.MustParse("a0000000-0000-0000-0000-000000000902")
	for _, u := range []struct {
		id    uuid.UUID
		email string
		name  string
		role  string
	}{
		{aliceID, "alice.cancel@example.test", "Alice Student", "student"},
		{bobID, "bob.director.cancel@example.test", "Bob Director", "sports_director"},
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

	// 3. Fund Alice's wallet with 300,000 paisa (PKR 3,000.00)
	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Cancel Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create Alice wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing Cancel", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           300000,
		Type:             "TOPUP",
		ReferenceID:      "TOPUP-CANCEL-ALICE-1",
		Description:      "Cancel test topup",
	})
	if err != nil {
		t.Fatalf("failed to fund Alice wallet: %v", err)
	}

	// 4. Create slot & confirm booking via Checkout
	startTime := time.Now().Add(4 * time.Hour).UTC().Truncate(time.Hour)
	endTime := startTime.Add(60 * time.Minute)
	createdSlot, err := repo.CreateSlot(ctx, &Slot{
		FacilityID: facID,
		StartTime:  startTime,
		EndTime:    endTime,
		Status:     SlotStatusAvailable,
		PricePaisa: 100000, // PKR 1,000.00
	})
	if err != nil {
		t.Fatalf("failed to create slot: %v", err)
	}

	holdRes, err := svc.HoldSlot(ctx, createdSlot.ID, aliceID)
	if err != nil {
		t.Fatalf("failed to hold slot: %v", err)
	}
	if holdRes.Status != "HELD" {
		t.Fatalf("expected slot to be held, got: %s", holdRes.Status)
	}

	bookingConfirm, err := svc.Checkout(ctx, CheckoutRequest{
		SlotID:         createdSlot.ID.String(),
		IdempotencyKey: "idemp-key-cancel-happy-1",
	}, aliceID)
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	bookingUUID := uuid.MustParse(bookingConfirm.BookingID)

	// Check Alice's balance after checkout (should be 300,000 - 100,000 = 200,000 paisa)
	aliceBalPostCheckout, err := walletSvc.GetUserWalletBalance(ctx, aliceID)
	if err != nil {
		t.Fatalf("failed to get Alice balance: %v", err)
	}
	if aliceBalPostCheckout.Balance != 200000 {
		t.Fatalf("expected Alice balance 200,000 paisa after checkout, got: %d", aliceBalPostCheckout.Balance)
	}

	// 5. Sports Director Bob cancels the booking with rationale (TC-FR-06-02)
	cancelResp, err := svc.CancelBooking(ctx, bookingUUID, bobID, CancelBookingRequest{
		Reason: "Court maintenance - glass replacement",
	})
	if err != nil {
		t.Fatalf("admin cancellation failed: %v", err)
	}

	if cancelResp.Status != "CANCELLED" {
		t.Errorf("expected booking status CANCELLED, got: %s", cancelResp.Status)
	}
	if cancelResp.CancelledBy != bobID.String() {
		t.Errorf("expected cancelledBy %s, got: %s", bobID, cancelResp.CancelledBy)
	}
	if cancelResp.Reason != "Court maintenance - glass replacement" {
		t.Errorf("expected reason match, got: %s", cancelResp.Reason)
	}
	if cancelResp.RefundTransactionID == "" {
		t.Errorf("expected refundTransactionId to be set")
	}

	// 6. Verify booking record in PostgreSQL
	var dbStatus string
	var refundTxID *uuid.UUID
	var cancelledBy *uuid.UUID
	var cancelledAt *time.Time
	var reason *string
	err = pool.QueryRow(ctx, `
		SELECT status, refund_transaction_id, cancelled_by, cancelled_at, cancellation_reason
		FROM giki_sports.padel_bookings
		WHERE id = $1;
	`, bookingUUID).Scan(&dbStatus, &refundTxID, &cancelledBy, &cancelledAt, &reason)
	if err != nil {
		t.Fatalf("failed to query cancelled booking: %v", err)
	}
	if dbStatus != "CANCELLED" {
		t.Errorf("expected DB status CANCELLED, got: %s", dbStatus)
	}
	if refundTxID == nil || refundTxID.String() != cancelResp.RefundTransactionID {
		t.Errorf("expected refund transaction ID match in DB")
	}
	if cancelledBy == nil || *cancelledBy != bobID {
		t.Errorf("expected cancelledBy in DB to be %s", bobID)
	}
	if cancelledAt == nil {
		t.Errorf("expected cancelledAt in DB to be populated")
	}
	if reason == nil || *reason != "Court maintenance - glass replacement" {
		t.Errorf("expected reason match in DB")
	}

	// 7. Verify slot status in PostgreSQL: must be transitioned back to AVAILABLE
	var slotStatus string
	var slotHolderID *uuid.UUID
	var slotHoldExpiresAt *time.Time
	err = pool.QueryRow(ctx, `
		SELECT status, holder_id, hold_expires_at
		FROM giki_sports.padel_slots
		WHERE id = $1;
	`, createdSlot.ID).Scan(&slotStatus, &slotHolderID, &slotHoldExpiresAt)
	if err != nil {
		t.Fatalf("failed to query released slot: %v", err)
	}
	if slotStatus != "AVAILABLE" {
		t.Errorf("expected slot status AVAILABLE after cancellation, got: %s", slotStatus)
	}
	if slotHolderID != nil {
		t.Errorf("expected holder_id to be NULL after slot release")
	}
	if slotHoldExpiresAt != nil {
		t.Errorf("expected hold_expires_at to be NULL after slot release")
	}

	// 8. Verify Alice's wallet balance: must have received 100% full refund (200,000 + 100,000 = 300,000 paisa)
	aliceBalPostRefund, err := walletSvc.GetUserWalletBalance(ctx, aliceID)
	if err != nil {
		t.Fatalf("failed to get Alice balance post refund: %v", err)
	}
	if aliceBalPostRefund.Balance != 300000 {
		t.Errorf("expected Alice balance 300,000 paisa after 100%% refund, got: %d", aliceBalPostRefund.Balance)
	}

	// 9. Verify double-entry refund ledger entry in giki_sports.wallet_transactions
	var refundTxType string
	var refundRefID string
	err = pool.QueryRow(ctx, `
		SELECT type, reference_id
		FROM giki_sports.wallet_transactions
		WHERE id = $1;
	`, uuid.MustParse(cancelResp.RefundTransactionID)).Scan(&refundTxType, &refundRefID)
	if err != nil {
		t.Fatalf("failed to query refund transaction: %v", err)
	}
	if refundTxType != "PADEL_REFUND" {
		t.Errorf("expected refund transaction type PADEL_REFUND, got: %s", refundTxType)
	}
	if refundRefID != "padel_refund:"+bookingUUID.String() {
		t.Errorf("expected reference_id padel_refund:%s, got: %s", bookingUUID, refundRefID)
	}

	// 10. Verify audit log entry in giki_sports.audit_logs
	var auditCount int
	err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM giki_sports.audit_logs
		WHERE target_type = 'padel_booking' 
		  AND target_id = $1 
		  AND action = 'booking.admin_cancel'
		  AND actor_id = $2;
	`, bookingUUID, bobID).Scan(&auditCount)
	if err != nil {
		t.Fatalf("failed to query audit log: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("expected exactly 1 admin_cancel audit log entry, got %d", auditCount)
	}

	// 11. Re-booking verification: slot can be immediately held and booked again!
	hold2, err := svc.HoldSlot(ctx, createdSlot.ID, aliceID)
	if err != nil {
		t.Fatalf("failed to re-hold slot: %v", err)
	}
	if hold2.Status != "HELD" {
		t.Fatalf("expected slot to be held again, got: %s", hold2.Status)
	}

	bookingConfirm2, err := svc.Checkout(ctx, CheckoutRequest{
		SlotID:         createdSlot.ID.String(),
		IdempotencyKey: "idemp-key-cancel-rebook-2",
	}, aliceID)
	if err != nil {
		t.Fatalf("failed to re-checkout slot after cancellation: %v", err)
	}
	if bookingConfirm2.Status != "CONFIRMED" {
		t.Errorf("expected re-booked status CONFIRMED, got: %s", bookingConfirm2.Status)
	}
	t.Logf("Verified (TC-FR-06-02): Admin Bob cancelled Alice's booking; Alice received 100%% refund (300,000 paisa), slot was released, and Alice successfully re-booked.")
}

// TestAdminCancelBooking_SingleRefundAndRejections_Integration tests the single-refund invariant
// and rejection of invalid cancellation requests.
func TestAdminCancelBooking_SingleRefundAndRejections_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)
	uow := NewUnitOfWork(pool, walletSvc)
	svc := NewService(repo, walletSvc, uow)

	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000902")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-cancel-rejections', 'Rejection Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000903")
	bobID := uuid.MustParse("a0000000-0000-0000-0000-000000000904")
	for _, u := range []struct {
		id    uuid.UUID
		email string
		name  string
		role  string
	}{
		{aliceID, "alice.single@example.test", "Alice Single", "student"},
		{bobID, "bob.director.single@example.test", "Bob Director", "sports_director"},
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

	// Fund Alice wallet with 200,000 paisa
	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Single Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing Single", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           200000,
		Type:             "TOPUP",
		ReferenceID:      "TOPUP-SINGLE-ALICE-1",
		Description:      "Single refund test topup",
	})
	if err != nil {
		t.Fatalf("failed to fund wallet: %v", err)
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
		IdempotencyKey: "idemp-key-single-refund-1",
	}, aliceID)
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	bookingUUID := uuid.MustParse(bookingConfirm.BookingID)

	// First cancellation succeeds
	_, err = svc.CancelBooking(ctx, bookingUUID, bobID, CancelBookingRequest{Reason: "Test cancellation 1"})
	if err != nil {
		t.Fatalf("first cancellation failed: %v", err)
	}

	// Second cancellation attempt MUST be rejected with ErrBookingAlreadyCancelled (Single-refund invariant)
	_, err = svc.CancelBooking(ctx, bookingUUID, bobID, CancelBookingRequest{Reason: "Test cancellation 2"})
	if !errors.Is(err, ErrBookingAlreadyCancelled) {
		t.Errorf("expected ErrBookingAlreadyCancelled on duplicate cancellation, got: %v", err)
	}

	// Assert Alice balance is exactly 200,000 (refunded once, not twice!)
	bal, err := walletSvc.GetUserWalletBalance(ctx, aliceID)
	if err != nil {
		t.Fatalf("failed to get Alice balance: %v", err)
	}
	if bal.Balance != 200000 {
		t.Errorf("expected Alice balance 200,000 after single refund, got: %d", bal.Balance)
	}

	// Non-existent booking ID returns ErrBookingNotFound
	nonExistentID := uuid.MustParse("00000000-0000-0000-0000-000000000999")
	_, err = svc.CancelBooking(ctx, nonExistentID, bobID, CancelBookingRequest{})
	if !errors.Is(err, ErrBookingNotFound) {
		t.Errorf("expected ErrBookingNotFound for unknown booking ID, got: %v", err)
	}

	t.Logf("Verified: Duplicate cancellation is rejected with ErrBookingAlreadyCancelled, enforcing the single-refund invariant.")
}

// TestAdminCancelBooking_CannotCancelCheckedIn_Integration verifies that checked-in bookings cannot be cancelled.
func TestAdminCancelBooking_CannotCancelCheckedIn_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)
	uow := NewUnitOfWork(pool, walletSvc)
	svc := NewService(repo, walletSvc, uow)

	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000903")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-cancel-checkedin', 'Checked In Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000905")
	bobID := uuid.MustParse("a0000000-0000-0000-0000-000000000906")
	for _, u := range []struct {
		id    uuid.UUID
		email string
		name  string
		role  string
	}{
		{aliceID, "alice.checkin.cancel@example.test", "Alice Checkin", "student"},
		{bobID, "bob.checkin.cancel@example.test", "Bob Checkin", "sports_director"},
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

	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Checkin Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing Checkin", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           200000,
		Type:             "TOPUP",
		ReferenceID:      "TOPUP-CHECKIN-ALICE-1",
		Description:      "Checkin test topup",
	})
	if err != nil {
		t.Fatalf("failed to fund wallet: %v", err)
	}

	startTime := time.Now().Add(6 * time.Hour).UTC().Truncate(time.Hour)
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
		IdempotencyKey: "idemp-key-checkin-cancel-1",
	}, aliceID)
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	bookingUUID := uuid.MustParse(bookingConfirm.BookingID)

	// Attendant verifies ticket at the entrance
	_, err = svc.VerifyTicket(ctx, VerifyTicketRequest{
		CheckInCode: bookingConfirm.CheckInCode,
	}, bobID)
	if err != nil {
		t.Fatalf("ticket verification failed: %v", err)
	}

	// Admin attempts to cancel checked-in booking -> MUST be rejected with ErrCannotCancelCheckedInBooking
	_, err = svc.CancelBooking(ctx, bookingUUID, bobID, CancelBookingRequest{Reason: "Attempt cancel checked in"})
	if !errors.Is(err, ErrCannotCancelCheckedInBooking) {
		t.Errorf("expected ErrCannotCancelCheckedInBooking, got: %v", err)
	}

	t.Logf("Verified: Checked-in bookings cannot be cancelled or refunded.")
}

// TestCancelBooking_HTTP_RoleAuthorization_Integration verifies strict HTTP role enforcement (TC-FR-06-01).
func TestCancelBooking_HTTP_RoleAuthorization_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)
	uow := NewUnitOfWork(pool, walletSvc)
	svc := NewService(repo, walletSvc, uow)

	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000904")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-auth-cancel-arena', 'Auth Cancel Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000907")
	bobID := uuid.MustParse("a0000000-0000-0000-0000-000000000908")
	for _, u := range []struct {
		id    uuid.UUID
		email string
		name  string
		role  string
	}{
		{aliceID, "alice.auth.cancel@example.test", "Alice Auth Student", "student"},
		{bobID, "bob.auth.cancel@example.test", "Bob Auth Director", "sports_director"},
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

	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Auth Cancel Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing Auth Cancel", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           200000,
		Type:             "TOPUP",
		ReferenceID:      "TOPUP-AUTH-CANCEL-ALICE-1",
		Description:      "Auth cancel test topup",
	})
	if err != nil {
		t.Fatalf("failed to fund wallet: %v", err)
	}

	startTime := time.Now().Add(7 * time.Hour).UTC().Truncate(time.Hour)
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
		IdempotencyKey: "idemp-key-auth-cancel-1",
	}, aliceID)
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

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

	aliceUser := &user.User{ID: aliceID, Email: "alice.auth.cancel@example.test", Roles: []user.Role{user.RoleStudent}}
	bobDirectorUser := &user.User{ID: bobID, Email: "bob.auth.cancel@example.test", Roles: []user.Role{user.RoleSportsDirector}}

	// App for Alice (student)
	appAlice := fiber.New()
	apiAlice := appAlice.Group("/api")
	handler.RegisterAuthenticatedRoutes(apiAlice, allowOrigin, mockAuthUser(aliceUser))
	handler.RegisterAdminRoutes(
		apiAlice,
		allowOrigin,
		mockAuthUser(aliceUser),
		authentication.RequireAnyRole(user.RoleSportsDirector, user.RoleSuperAdmin),
	)

	// App for Bob (sports_director)
	appBob := fiber.New()
	apiBob := appBob.Group("/api")
	handler.RegisterAuthenticatedRoutes(apiBob, allowOrigin, mockAuthUser(bobDirectorUser))
	handler.RegisterAdminRoutes(
		apiBob,
		allowOrigin,
		mockAuthUser(bobDirectorUser),
		authentication.RequireAnyRole(user.RoleSportsDirector, user.RoleSuperAdmin),
	)

	// 1. Student Alice calling POST /api/admin/bookings/:id/cancel receives 403 Forbidden (TC-FR-06-01)
	reqAliceAdmin := httptest.NewRequest("POST", "/api/admin/bookings/"+bookingConfirm.BookingID+"/cancel", nil)
	reqAliceAdmin.Header.Set("Origin", "http://127.0.0.1:5173")
	respAliceAdmin, err := appAlice.Test(reqAliceAdmin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if respAliceAdmin.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Student Alice calling admin cancel endpoint, got %d", respAliceAdmin.StatusCode)
	}

	// 2. Student Alice calling user-facing self-service cancel POST /api/padel/bookings/:id/cancel receives 403 Forbidden (TC-FR-06-01)
	reqAliceUser := httptest.NewRequest("POST", "/api/padel/bookings/"+bookingConfirm.BookingID+"/cancel", nil)
	reqAliceUser.Header.Set("Origin", "http://127.0.0.1:5173")
	respAliceUser, err := appAlice.Test(reqAliceUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if respAliceUser.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for Student Alice calling self-service cancel route, got %d", respAliceUser.StatusCode)
	}

	// 3. Director Bob calling POST /api/admin/bookings/:id/cancel receives 200 OK
	cancelBody, _ := json.Marshal(CancelBookingRequest{Reason: "Director authorized cancel"})
	reqBob := httptest.NewRequest("POST", "/api/admin/bookings/"+bookingConfirm.BookingID+"/cancel", bytes.NewReader(cancelBody))
	reqBob.Header.Set("Origin", "http://127.0.0.1:5173")
	reqBob.Header.Set("Content-Type", "application/json")
	respBob, err := appBob.Test(reqBob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if respBob.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for Director Bob calling admin cancel, got %d", respBob.StatusCode)
	}

	t.Logf("Verified (TC-FR-06-01): Student attempts to cancel are rejected with 403 Forbidden; Sports Director receives 200 OK.")
}
