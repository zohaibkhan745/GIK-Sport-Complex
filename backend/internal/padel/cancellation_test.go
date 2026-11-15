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

	"github.com/google/uuid"

	"giki-sports-backend/internal/user"
	"giki-sports-backend/internal/wallet"
)

type mockCancelBookingRepo struct {
	booking     *Booking
	lockErr     error
	releaseErr  error
	cancelErr   error
	released    bool
	cancelled   bool
	auditLogged bool
}

func (m *mockCancelBookingRepo) LockSlotForBooking(ctx context.Context, slotID uuid.UUID) (*Slot, error) {
	return &Slot{
		ID:         slotID,
		FacilityID: uuid.New(),
		Status:     SlotStatusBooked,
		PricePaisa: 100000,
	}, nil
}

func (m *mockCancelBookingRepo) CreateBooking(ctx context.Context, booking *Booking) error {
	return nil
}

func (m *mockCancelBookingRepo) MarkSlotBooked(ctx context.Context, slotID uuid.UUID) error {
	return nil
}

func (m *mockCancelBookingRepo) FindBookingByIdempotency(ctx context.Context, userID uuid.UUID, idempotencyKey string) (*Booking, error) {
	return nil, nil
}

func (m *mockCancelBookingRepo) FindBookingByID(ctx context.Context, bookingID uuid.UUID) (*Booking, error) {
	return m.booking, nil
}

func (m *mockCancelBookingRepo) LockBookingForUpdate(ctx context.Context, bookingID uuid.UUID) (*Booking, error) {
	if m.lockErr != nil {
		return nil, m.lockErr
	}
	if m.booking == nil {
		return nil, ErrBookingNotFound
	}
	return m.booking, nil
}

func (m *mockCancelBookingRepo) ReleaseSlotToAvailable(ctx context.Context, slotID uuid.UUID) error {
	if m.releaseErr != nil {
		return m.releaseErr
	}
	m.released = true
	return nil
}

func (m *mockCancelBookingRepo) CancelBooking(ctx context.Context, bookingID uuid.UUID, adminID uuid.UUID, reason string, refundTxID uuid.UUID, now time.Time) (*Booking, error) {
	if m.cancelErr != nil {
		return nil, m.cancelErr
	}
	m.cancelled = true
	b := *m.booking
	b.Status = BookingStatusCancelled
	b.CancelledAt = &now
	b.CancelledBy = &adminID
	b.CancellationReason = &reason
	b.RefundTransactionID = &refundTxID
	return &b, nil
}

func (m *mockCancelBookingRepo) RecordAuditLog(ctx context.Context, actorID uuid.UUID, action string, targetType string, targetID uuid.UUID, details map[string]interface{}) error {
	m.auditLogged = true
	return nil
}

type mockCancelLedgerRepo struct {
	txID uuid.UUID
}

func (m *mockCancelLedgerRepo) FindPersonalWallet(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (m *mockCancelLedgerRepo) FindOrCreateSystemWallet(ctx context.Context, name string, wType wallet.WalletType, currency string) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (m *mockCancelLedgerRepo) TransferFunds(ctx context.Context, params wallet.TransferParams) (*wallet.PostResult, error) {
	return &wallet.PostResult{TransactionID: m.txID, AlreadyPosted: false}, nil
}

type mockCancelFactory struct {
	bookingRepo *mockCancelBookingRepo
	ledgerRepo  *mockCancelLedgerRepo
}

func (f *mockCancelFactory) Bookings() BookingRepository {
	return f.bookingRepo
}

func (f *mockCancelFactory) Ledger() LedgerRepository {
	return f.ledgerRepo
}

type mockCancelUoW struct {
	factory *mockCancelFactory
}

func (u *mockCancelUoW) Execute(ctx context.Context, fn func(factory TransactionRepositoryFactory) error) error {
	return fn(u.factory)
}

func TestService_CancelBooking(t *testing.T) {
	now := time.Now().UTC()
	adminID := uuid.New()
	userID := uuid.New()
	slotID := uuid.New()
	bookingID := uuid.New()
	refundTxID := uuid.New()

	t.Run("nil unit of work returns error", func(t *testing.T) {
		svc := NewServiceWithClock(newMockRepository(), nil, nil, staticClock{t: now}, DefaultPakistanLocation())
		_, err := svc.CancelBooking(context.Background(), bookingID, adminID, CancelBookingRequest{})
		if err == nil || err.Error() != "unit of work is uninitialized" {
			t.Errorf("expected unit of work uninitialized error, got: %v", err)
		}
	})

	t.Run("nil booking ID returns ErrBookingNotFound", func(t *testing.T) {
		uow := &mockCancelUoW{}
		svc := NewServiceWithClock(newMockRepository(), nil, uow, staticClock{t: now}, DefaultPakistanLocation())
		_, err := svc.CancelBooking(context.Background(), uuid.Nil, adminID, CancelBookingRequest{})
		if !errors.Is(err, ErrBookingNotFound) {
			t.Errorf("expected ErrBookingNotFound, got: %v", err)
		}
	})

	t.Run("nil admin ID returns error", func(t *testing.T) {
		uow := &mockCancelUoW{}
		svc := NewServiceWithClock(newMockRepository(), nil, uow, staticClock{t: now}, DefaultPakistanLocation())
		_, err := svc.CancelBooking(context.Background(), bookingID, uuid.Nil, CancelBookingRequest{})
		if err == nil || err.Error() != "admin ID is required" {
			t.Errorf("expected admin ID is required error, got: %v", err)
		}
	})

	t.Run("booking not found returns ErrBookingNotFound", func(t *testing.T) {
		bRepo := &mockCancelBookingRepo{booking: nil}
		lRepo := &mockCancelLedgerRepo{txID: refundTxID}
		uow := &mockCancelUoW{factory: &mockCancelFactory{bookingRepo: bRepo, ledgerRepo: lRepo}}
		svc := NewServiceWithClock(newMockRepository(), nil, uow, staticClock{t: now}, DefaultPakistanLocation())

		_, err := svc.CancelBooking(context.Background(), bookingID, adminID, CancelBookingRequest{})
		if !errors.Is(err, ErrBookingNotFound) {
			t.Errorf("expected ErrBookingNotFound, got: %v", err)
		}
	})

	t.Run("already cancelled booking returns ErrBookingAlreadyCancelled", func(t *testing.T) {
		bRepo := &mockCancelBookingRepo{
			booking: &Booking{
				ID:       bookingID,
				SlotID:   slotID,
				UserID:   userID,
				Status:   BookingStatusCancelled,
				FeePaisa: 100000,
			},
		}
		lRepo := &mockCancelLedgerRepo{txID: refundTxID}
		uow := &mockCancelUoW{factory: &mockCancelFactory{bookingRepo: bRepo, ledgerRepo: lRepo}}
		svc := NewServiceWithClock(newMockRepository(), nil, uow, staticClock{t: now}, DefaultPakistanLocation())

		_, err := svc.CancelBooking(context.Background(), bookingID, adminID, CancelBookingRequest{})
		if !errors.Is(err, ErrBookingAlreadyCancelled) {
			t.Errorf("expected ErrBookingAlreadyCancelled, got: %v", err)
		}
	})

	t.Run("checked-in booking returns ErrCannotCancelCheckedInBooking", func(t *testing.T) {
		bRepo := &mockCancelBookingRepo{
			booking: &Booking{
				ID:       bookingID,
				SlotID:   slotID,
				UserID:   userID,
				Status:   BookingStatusCheckedIn,
				FeePaisa: 100000,
			},
		}
		lRepo := &mockCancelLedgerRepo{txID: refundTxID}
		uow := &mockCancelUoW{factory: &mockCancelFactory{bookingRepo: bRepo, ledgerRepo: lRepo}}
		svc := NewServiceWithClock(newMockRepository(), nil, uow, staticClock{t: now}, DefaultPakistanLocation())

		_, err := svc.CancelBooking(context.Background(), bookingID, adminID, CancelBookingRequest{})
		if !errors.Is(err, ErrCannotCancelCheckedInBooking) {
			t.Errorf("expected ErrCannotCancelCheckedInBooking, got: %v", err)
		}
	})

	t.Run("happy path cancels booking, refunds ledger, releases slot, and logs audit", func(t *testing.T) {
		bRepo := &mockCancelBookingRepo{
			booking: &Booking{
				ID:       bookingID,
				SlotID:   slotID,
				UserID:   userID,
				Status:   BookingStatusConfirmed,
				FeePaisa: 100000,
			},
		}
		lRepo := &mockCancelLedgerRepo{txID: refundTxID}
		uow := &mockCancelUoW{factory: &mockCancelFactory{bookingRepo: bRepo, ledgerRepo: lRepo}}
		svc := NewServiceWithClock(newMockRepository(), nil, uow, staticClock{t: now}, DefaultPakistanLocation())

		resp, err := svc.CancelBooking(context.Background(), bookingID, adminID, CancelBookingRequest{
			Reason: "Heavy rain floodlight failure",
		})
		if err != nil {
			t.Fatalf("unexpected cancellation error: %v", err)
		}

		if resp.Status != "CANCELLED" {
			t.Errorf("expected status CANCELLED, got %s", resp.Status)
		}
		if resp.BookingID != bookingID.String() {
			t.Errorf("expected bookingID %s, got %s", bookingID, resp.BookingID)
		}
		if resp.RefundTransactionID != refundTxID.String() {
			t.Errorf("expected refundTxID %s, got %s", refundTxID, resp.RefundTransactionID)
		}
		if resp.FeePaisa != "100000" {
			t.Errorf("expected feePaisa 100000, got %s", resp.FeePaisa)
		}
		if resp.Reason != "Heavy rain floodlight failure" {
			t.Errorf("expected reason match, got %s", resp.Reason)
		}
		if !bRepo.released {
			t.Errorf("expected slot to be released back to AVAILABLE")
		}
		if !bRepo.cancelled {
			t.Errorf("expected booking to be updated to CANCELLED")
		}
		if !bRepo.auditLogged {
			t.Errorf("expected audit log to be recorded")
		}
	})
}

func TestHandler_CancelBooking(t *testing.T) {
	now := time.Now().UTC()
	adminID := uuid.New()
	userID := uuid.New()
	slotID := uuid.New()
	bookingID := uuid.New()
	refundTxID := uuid.New()

	directorUser := &user.User{
		ID:          adminID,
		Email:       "director@example.test",
		DisplayName: "Director Sports",
		Roles:       []user.Role{user.RoleSportsDirector},
	}

	t.Run("invalid UUID param returns 400 Bad Request", func(t *testing.T) {
		uow := &mockCancelUoW{}
		svc := NewServiceWithClock(newMockRepository(), nil, uow, staticClock{t: now}, DefaultPakistanLocation())
		app := setupTestApp(svc, directorUser)

		req := httptest.NewRequest("POST", "/api/admin/bookings/not-a-uuid/cancel", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
		}
	})

	t.Run("unauthenticated request returns 401 Unauthorized", func(t *testing.T) {
		uow := &mockCancelUoW{}
		svc := NewServiceWithClock(newMockRepository(), nil, uow, staticClock{t: now}, DefaultPakistanLocation())
		app := setupTestApp(svc, nil) // no principal

		req := httptest.NewRequest("POST", "/api/admin/bookings/"+bookingID.String()+"/cancel", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("booking not found returns 404 Not Found", func(t *testing.T) {
		bRepo := &mockCancelBookingRepo{booking: nil}
		lRepo := &mockCancelLedgerRepo{txID: refundTxID}
		uow := &mockCancelUoW{factory: &mockCancelFactory{bookingRepo: bRepo, ledgerRepo: lRepo}}
		svc := NewServiceWithClock(newMockRepository(), nil, uow, staticClock{t: now}, DefaultPakistanLocation())
		app := setupTestApp(svc, directorUser)

		req := httptest.NewRequest("POST", "/api/admin/bookings/"+bookingID.String()+"/cancel", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 Not Found, got %d", resp.StatusCode)
		}
	})

	t.Run("already cancelled returns 409 Conflict", func(t *testing.T) {
		bRepo := &mockCancelBookingRepo{
			booking: &Booking{
				ID:       bookingID,
				SlotID:   slotID,
				UserID:   userID,
				Status:   BookingStatusCancelled,
				FeePaisa: 100000,
			},
		}
		lRepo := &mockCancelLedgerRepo{txID: refundTxID}
		uow := &mockCancelUoW{factory: &mockCancelFactory{bookingRepo: bRepo, ledgerRepo: lRepo}}
		svc := NewServiceWithClock(newMockRepository(), nil, uow, staticClock{t: now}, DefaultPakistanLocation())
		app := setupTestApp(svc, directorUser)

		req := httptest.NewRequest("POST", "/api/admin/bookings/"+bookingID.String()+"/cancel", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("expected 409 Conflict, got %d", resp.StatusCode)
		}
	})

	t.Run("successful cancellation returns 200 OK with details", func(t *testing.T) {
		bRepo := &mockCancelBookingRepo{
			booking: &Booking{
				ID:       bookingID,
				SlotID:   slotID,
				UserID:   userID,
				Status:   BookingStatusConfirmed,
				FeePaisa: 100000,
			},
		}
		lRepo := &mockCancelLedgerRepo{txID: refundTxID}
		uow := &mockCancelUoW{factory: &mockCancelFactory{bookingRepo: bRepo, ledgerRepo: lRepo}}
		svc := NewServiceWithClock(newMockRepository(), nil, uow, staticClock{t: now}, DefaultPakistanLocation())
		app := setupTestApp(svc, directorUser)

		body, _ := json.Marshal(CancelBookingRequest{Reason: "Maintenance"})
		req := httptest.NewRequest("POST", "/api/admin/bookings/"+bookingID.String()+"/cancel", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}

		var cancelResp CancelBookingResponse
		if err := json.NewDecoder(resp.Body).Decode(&cancelResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if cancelResp.Status != "CANCELLED" {
			t.Errorf("expected status CANCELLED, got %s", cancelResp.Status)
		}
		if cancelResp.RefundTransactionID != refundTxID.String() {
			t.Errorf("expected refund transaction id match, got %s", cancelResp.RefundTransactionID)
		}
	})

	t.Run("student self-cancellation route returns 403 Forbidden (TC-FR-06-01)", func(t *testing.T) {
		studentUser := &user.User{
			ID:          userID,
			Email:       "student@example.test",
			DisplayName: "Student Alice",
			Roles:       []user.Role{user.RoleStudent},
		}

		uow := &mockCancelUoW{}
		svc := NewServiceWithClock(newMockRepository(), nil, uow, staticClock{t: now}, DefaultPakistanLocation())
		app := setupTestApp(svc, studentUser)

		req := httptest.NewRequest("POST", "/api/padel/bookings/"+bookingID.String()+"/cancel", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for student self-cancellation, got %d", resp.StatusCode)
		}
	})
}
