package padel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"giki-sports-backend/internal/user"
	"giki-sports-backend/internal/wallet"
)

func TestCheckInCodeGenerator(t *testing.T) {
	t.Run("generates valid 4-digit code in range 1000-9999", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 100; i++ {
			code, err := GenerateCheckInCode()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(code) != 4 {
				t.Errorf("expected 4 digits, got length %d (%s)", len(code), code)
			}
			num, err := strconv.Atoi(code)
			if err != nil {
				t.Errorf("code %s is not a valid integer", code)
			}
			if num < 1000 || num > 9999 {
				t.Errorf("code %d out of range [1000, 9999]", num)
			}
			seen[code] = true
		}
		if len(seen) < 80 {
			t.Errorf("insufficient randomness: only %d unique codes across 100 runs", len(seen))
		}
	})
}

func TestService_Checkout_Validation(t *testing.T) {
	repo := newMockRepository()
	svc := NewService(repo, nil, nil)
	userID := uuid.New()

	t.Run("uninitialized unit of work returns error", func(t *testing.T) {
		_, err := svc.Checkout(context.Background(), CheckoutRequest{
			SlotID:         uuid.New().String(),
			IdempotencyKey: "test-key",
		}, userID)
		if err == nil || err.Error() != "unit of work is uninitialized" {
			t.Errorf("expected unit of work uninitialized error, got: %v", err)
		}
	})

	t.Run("blank idempotency key returns ErrBlankIdempotencyKey", func(t *testing.T) {
		mockUoW := &mockUnitOfWork{}
		mockWalletSvc := wallet.NewService(nil)
		svcWithUoW := NewService(repo, mockWalletSvc, mockUoW)

		_, err := svcWithUoW.Checkout(context.Background(), CheckoutRequest{
			SlotID:         uuid.New().String(),
			IdempotencyKey: "   ",
		}, userID)
		if !errors.Is(err, ErrBlankIdempotencyKey) {
			t.Errorf("expected ErrBlankIdempotencyKey, got: %v", err)
		}
	})

	t.Run("invalid slot ID returns ErrSlotNotFound", func(t *testing.T) {
		mockUoW := &mockUnitOfWork{}
		mockWalletSvc := wallet.NewService(nil)
		svcWithUoW := NewService(repo, mockWalletSvc, mockUoW)

		_, err := svcWithUoW.Checkout(context.Background(), CheckoutRequest{
			SlotID:         "invalid-uuid",
			IdempotencyKey: "test-key",
		}, userID)
		if !errors.Is(err, ErrSlotNotFound) {
			t.Errorf("expected ErrSlotNotFound, got: %v", err)
		}
	})

	t.Run("nil user ID returns error", func(t *testing.T) {
		mockUoW := &mockUnitOfWork{}
		mockWalletSvc := wallet.NewService(nil)
		svcWithUoW := NewService(repo, mockWalletSvc, mockUoW)

		_, err := svcWithUoW.Checkout(context.Background(), CheckoutRequest{
			SlotID:         uuid.New().String(),
			IdempotencyKey: "test-key",
		}, uuid.Nil)
		if err == nil || err.Error() != "user ID is required" {
			t.Errorf("expected user ID is required error, got: %v", err)
		}
	})
}

func TestHandler_Checkout_HTTP(t *testing.T) {
	repo := newMockRepository()
	mockUoW := &mockUnitOfWork{}
	svc := NewService(repo, wallet.NewService(nil), mockUoW)

	student := &user.User{
		ID:          uuid.New(),
		Email:       "student@example.test",
		DisplayName: "Student Bob",
	}
	app := setupTestApp(svc, student)
	unauthApp := setupTestApp(svc, nil)

	t.Run("unauthenticated request returns 401 Unauthorized", func(t *testing.T) {
		body, _ := json.Marshal(CheckoutRequest{
			SlotID:         uuid.New().String(),
			IdempotencyKey: "key-1",
		})
		req := httptest.NewRequest("POST", "/api/padel/checkout", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := unauthApp.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("missing slot ID in body returns 400 Bad Request", func(t *testing.T) {
		body, _ := json.Marshal(CheckoutRequest{
			SlotID:         "",
			IdempotencyKey: "key-1",
		})
		req := httptest.NewRequest("POST", "/api/padel/checkout", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
		}
	})

	t.Run("blank idempotency key in body returns 400 Bad Request", func(t *testing.T) {
		body, _ := json.Marshal(CheckoutRequest{
			SlotID:         uuid.New().String(),
			IdempotencyKey: "   ",
		})
		req := httptest.NewRequest("POST", "/api/padel/checkout", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
		}
	})
}

// mockUnitOfWork implements UnitOfWork for pure unit testing without a PostgreSQL pool.
type mockUnitOfWork struct {
	executeFunc func(factory TransactionRepositoryFactory) error
}

func (m *mockUnitOfWork) Execute(ctx context.Context, fn func(factory TransactionRepositoryFactory) error) error {
	if m.executeFunc != nil {
		return m.executeFunc(nil)
	}
	return fn(&mockTransactionRepositoryFactory{})
}

type mockTransactionRepositoryFactory struct{}

func (f *mockTransactionRepositoryFactory) Bookings() BookingRepository {
	return &mockBookingRepository{}
}

func (f *mockTransactionRepositoryFactory) Ledger() LedgerRepository {
	return &mockLedgerRepository{}
}

type mockBookingRepository struct{}

func (r *mockBookingRepository) LockSlotForBooking(ctx context.Context, slotID uuid.UUID) (*Slot, error) {
	return nil, ErrSlotNotFound
}

func (r *mockBookingRepository) CreateBooking(ctx context.Context, booking *Booking) error {
	return nil
}

func (r *mockBookingRepository) MarkSlotBooked(ctx context.Context, slotID uuid.UUID) error {
	return nil
}

func (r *mockBookingRepository) FindBookingByIdempotency(ctx context.Context, userID uuid.UUID, idempotencyKey string) (*Booking, error) {
	return nil, nil
}

func (r *mockBookingRepository) FindBookingByID(ctx context.Context, bookingID uuid.UUID) (*Booking, error) {
	return nil, nil
}

func (r *mockBookingRepository) LockBookingForUpdate(ctx context.Context, bookingID uuid.UUID) (*Booking, error) {
	return nil, nil
}

func (r *mockBookingRepository) ReleaseSlotToAvailable(ctx context.Context, slotID uuid.UUID) error {
	return nil
}

func (r *mockBookingRepository) CancelBooking(ctx context.Context, bookingID uuid.UUID, adminID uuid.UUID, reason string, refundTxID uuid.UUID, now time.Time) (*Booking, error) {
	return nil, nil
}

func (r *mockBookingRepository) RecordAuditLog(ctx context.Context, actorID uuid.UUID, action string, targetType string, targetID uuid.UUID, details map[string]interface{}) error {
	return nil
}

type mockLedgerRepository struct{}

func (r *mockLedgerRepository) FindPersonalWallet(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (r *mockLedgerRepository) FindOrCreateSystemWallet(ctx context.Context, name string, wType wallet.WalletType, currency string) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (r *mockLedgerRepository) TransferFunds(ctx context.Context, params wallet.TransferParams) (*wallet.PostResult, error) {
	return &wallet.PostResult{
		TransactionID: uuid.New(),
		AlreadyPosted: false,
	}, nil
}
