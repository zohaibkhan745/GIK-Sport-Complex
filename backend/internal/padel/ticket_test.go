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
)

func TestService_VerifyTicket(t *testing.T) {
	repo := newMockRepository()
	now := time.Now().UTC()
	svc := NewServiceWithClock(repo, nil, nil, staticClock{t: now}, DefaultPakistanLocation())
	attendantID := uuid.New()

	t.Run("nil attendant ID returns error", func(t *testing.T) {
		_, err := svc.VerifyTicket(context.Background(), VerifyTicketRequest{
			CheckInCode: "1234",
		}, uuid.Nil)
		if err == nil || err.Error() != "attendant ID is required" {
			t.Errorf("expected attendant ID is required error, got: %v", err)
		}
	})

	t.Run("blank check-in code returns ErrInvalidCheckInCode", func(t *testing.T) {
		_, err := svc.VerifyTicket(context.Background(), VerifyTicketRequest{
			CheckInCode: "   ",
		}, attendantID)
		if !errors.Is(err, ErrInvalidCheckInCode) {
			t.Errorf("expected ErrInvalidCheckInCode, got: %v", err)
		}
	})

	t.Run("valid code returns verified checked-in ticket", func(t *testing.T) {
		resp, err := svc.VerifyTicket(context.Background(), VerifyTicketRequest{
			CheckInCode: "VALID1",
		}, attendantID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Status != "CHECKED_IN" {
			t.Errorf("expected status CHECKED_IN, got %s", resp.Status)
		}
		if resp.CheckInCode != "VALID1" {
			t.Errorf("expected code VALID1, got %s", resp.CheckInCode)
		}
		if resp.CheckedInBy != attendantID.String() {
			t.Errorf("expected checkedInBy %s, got %s", attendantID, resp.CheckedInBy)
		}
	})

	t.Run("already checked-in ticket returns ErrTicketAlreadyCheckedIn", func(t *testing.T) {
		_, err := svc.VerifyTicket(context.Background(), VerifyTicketRequest{
			CheckInCode: "ALREADY",
		}, attendantID)
		if !errors.Is(err, ErrTicketAlreadyCheckedIn) {
			t.Errorf("expected ErrTicketAlreadyCheckedIn, got: %v", err)
		}
	})

	t.Run("cancelled booking returns ErrBookingCancelled", func(t *testing.T) {
		_, err := svc.VerifyTicket(context.Background(), VerifyTicketRequest{
			CheckInCode: "CANCELLED",
		}, attendantID)
		if !errors.Is(err, ErrBookingCancelled) {
			t.Errorf("expected ErrBookingCancelled, got: %v", err)
		}
	})

	t.Run("non-existent code returns ErrBookingNotFound", func(t *testing.T) {
		_, err := svc.VerifyTicket(context.Background(), VerifyTicketRequest{
			CheckInCode: "UNKNOWN",
		}, attendantID)
		if !errors.Is(err, ErrBookingNotFound) {
			t.Errorf("expected ErrBookingNotFound, got: %v", err)
		}
	})
}

func TestHandler_VerifyTicket(t *testing.T) {
	repo := newMockRepository()
	now := time.Now().UTC()
	svc := NewServiceWithClock(repo, nil, nil, staticClock{t: now}, DefaultPakistanLocation())

	directorUser := &user.User{
		ID:          uuid.New(),
		Email:       "director@example.test",
		DisplayName: "Director Ali",
		Roles:       []user.Role{user.RoleSportsDirector},
	}

	app := setupTestApp(svc, directorUser)
	unauthApp := setupTestApp(svc, nil)

	t.Run("unauthenticated request returns 401 Unauthorized", func(t *testing.T) {
		body, _ := json.Marshal(VerifyTicketRequest{
			CheckInCode: "VALID1",
		})
		req := httptest.NewRequest("POST", "/api/padel/bookings/verify-ticket", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := unauthApp.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("blank check-in code in body returns 400 Bad Request", func(t *testing.T) {
		body, _ := json.Marshal(VerifyTicketRequest{
			CheckInCode: "   ",
		})
		req := httptest.NewRequest("POST", "/api/padel/bookings/verify-ticket", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
		}
	})

	t.Run("valid code returns 200 OK with CHECKED_IN status", func(t *testing.T) {
		body, _ := json.Marshal(VerifyTicketRequest{
			CheckInCode: "VALID1",
		})
		req := httptest.NewRequest("POST", "/api/padel/bookings/verify-ticket", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}

		var verifyResp VerifyTicketResponse
		_ = json.NewDecoder(resp.Body).Decode(&verifyResp)
		if verifyResp.Status != "CHECKED_IN" {
			t.Errorf("expected status CHECKED_IN, got %s", verifyResp.Status)
		}
	})

	t.Run("already checked-in ticket returns 409 Conflict", func(t *testing.T) {
		body, _ := json.Marshal(VerifyTicketRequest{
			CheckInCode: "ALREADY",
		})
		req := httptest.NewRequest("POST", "/api/padel/bookings/verify-ticket", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("expected 409 Conflict, got %d", resp.StatusCode)
		}
	})

	t.Run("cancelled booking returns 409 Conflict", func(t *testing.T) {
		body, _ := json.Marshal(VerifyTicketRequest{
			CheckInCode: "CANCELLED",
		})
		req := httptest.NewRequest("POST", "/api/padel/bookings/verify-ticket", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("expected 409 Conflict, got %d", resp.StatusCode)
		}
	})

	t.Run("unknown check-in code returns 404 Not Found", func(t *testing.T) {
		body, _ := json.Marshal(VerifyTicketRequest{
			CheckInCode: "UNKNOWN",
		})
		req := httptest.NewRequest("POST", "/api/padel/bookings/verify-ticket", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 Not Found, got %d", resp.StatusCode)
		}
	})
}
