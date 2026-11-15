package padel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"giki-sports-backend/internal/authentication"
	"giki-sports-backend/internal/user"
)

func setupTestApp(svc *Service, principalUser *user.User) *fiber.App {
	app := fiber.New()
	handler := NewHandler(svc, 2*time.Second)
	api := app.Group("/api")
	handler.RegisterPublicRoutes(api)

	// Mock middleware injecting principal
	mockAuth := func(c *fiber.Ctx) error {
		if principalUser != nil {
			authentication.SetPrincipalForTest(c, &authentication.AuthenticatedPrincipal{
				User: principalUser,
			})
			return c.Next()
		}
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	allowAll := func(c *fiber.Ctx) error { return c.Next() }

	handler.RegisterAuthenticatedRoutes(api, allowAll, mockAuth)
	handler.RegisterAdminRoutes(api, allowAll, mockAuth, allowAll)
	return app
}

func TestHandler_GetSlotsValidation(t *testing.T) {
	repo := newMockRepository()
	facID := uuid.New()
	repo.facilities = append(repo.facilities, FacilitySummaryDTO{
		ID:     facID.String(),
		Slug:   "padel-court-1",
		Name:   "Padel Court 1",
		Status: "active",
	})
	svc := NewService(repo, nil, nil)
	adminUser := &user.User{
		ID:          uuid.New(),
		Email:       "admin@example.test",
		DisplayName: "Admin Director",
		Roles:       []user.Role{user.RoleSportsDirector},
	}
	app := setupTestApp(svc, adminUser)

	t.Run("missing date returns 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/padel/slots", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid date format returns 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/padel/slots?date=invalid-date", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid facilityId UUID returns 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/padel/slots?date=2026-11-15&facilityId=bad-uuid", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
		}
	})

	t.Run("unknown facilityId returns 404", func(t *testing.T) {
		unknownID := uuid.New()
		req := httptest.NewRequest("GET", "/api/padel/slots?date=2026-11-15&facilityId="+unknownID.String(), nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 Not Found, got %d", resp.StatusCode)
		}
	})

	t.Run("valid request with default facility returns 200", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/padel/slots?date=2026-11-15", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}
	})
}

func TestHandler_AdminSlotOperations(t *testing.T) {
	repo := newMockRepository()
	facID := uuid.New()
	repo.facilities = append(repo.facilities, FacilitySummaryDTO{
		ID:     facID.String(),
		Slug:   "padel-court-1",
		Name:   "Padel Court 1",
		Status: "active",
	})
	svc := NewService(repo, nil, nil)
	adminUser := &user.User{
		ID:          uuid.New(),
		Email:       "admin@example.test",
		DisplayName: "Admin Director",
		Roles:       []user.Role{user.RoleSportsDirector},
	}
	app := setupTestApp(svc, adminUser)

	t.Run("generate slots returns 201 Created", func(t *testing.T) {
		body, _ := json.Marshal(GenerateSlotsRequest{
			FacilityID:          facID.String(),
			Date:                "2026-11-15",
			StartHour:           9,
			EndHour:             12,
			SlotDurationMinutes: 60,
			PricePaisa:          "120000",
		})
		req := httptest.NewRequest("POST", "/api/admin/padel/slots/generate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected 201 Created, got %d", resp.StatusCode)
		}
	})

	t.Run("update slot price returns 200 OK", func(t *testing.T) {
		// Insert slot
		slotID := uuid.New()
		now := time.Now().UTC()
		repo.slots[slotID] = Slot{
			ID:         slotID,
			FacilityID: facID,
			StartTime:  now,
			EndTime:    now.Add(60 * time.Minute),
			Status:     SlotStatusAvailable,
			PricePaisa: 100000,
		}

		newPrice := "135000"
		body, _ := json.Marshal(UpdateSlotRequest{
			PricePaisa: &newPrice,
		})
		req := httptest.NewRequest("PATCH", "/api/admin/padel/slots/"+slotID.String(), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}
	})

	t.Run("update booked slot price returns 409 Conflict", func(t *testing.T) {
		slotID := uuid.New()
		now := time.Now().UTC()
		repo.slots[slotID] = Slot{
			ID:         slotID,
			FacilityID: facID,
			StartTime:  now,
			EndTime:    now.Add(60 * time.Minute),
			Status:     SlotStatusBooked,
			PricePaisa: 100000,
		}

		newPrice := "150000"
		body, _ := json.Marshal(UpdateSlotRequest{
			PricePaisa: &newPrice,
		})
		req := httptest.NewRequest("PATCH", "/api/admin/padel/slots/"+slotID.String(), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("expected 409 Conflict, got %d", resp.StatusCode)
		}
	})
}

func TestHandler_HoldAndReleaseSlot(t *testing.T) {
	repo := newMockRepository()
	facID := uuid.New()
	repo.facilities = append(repo.facilities, FacilitySummaryDTO{
		ID:     facID.String(),
		Slug:   "padel-court-1",
		Name:   "Padel Court 1",
		Status: "active",
	})
	svc := NewService(repo, nil, nil)

	studentUser := &user.User{
		ID:          uuid.New(),
		Email:       "student@example.test",
		DisplayName: "Student Alice",
	}
	app := setupTestApp(svc, studentUser)
	unauthApp := setupTestApp(svc, nil)

	slotID := uuid.New()
	now := time.Now().UTC()
	repo.slots[slotID] = Slot{
		ID:         slotID,
		FacilityID: facID,
		StartTime:  now.Add(2 * time.Hour),
		EndTime:    now.Add(3 * time.Hour),
		Status:     SlotStatusAvailable,
		PricePaisa: 100000,
	}

	t.Run("unauthenticated hold request returns 401 Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/padel/slots/"+slotID.String()+"/hold", nil)
		resp, err := unauthApp.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid slot ID returns 400 Bad Request", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/padel/slots/invalid-uuid/hold", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
		}
	})

	t.Run("authenticated hold request succeeds with 200 OK", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/padel/slots/"+slotID.String()+"/hold", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}
		var holdResp HoldSlotResponse
		_ = json.NewDecoder(resp.Body).Decode(&holdResp)
		if holdResp.Status != "HELD" {
			t.Errorf("expected status HELD, got %s", holdResp.Status)
		}
		if holdResp.RemainingSeconds <= 0 {
			t.Errorf("expected positive remaining seconds, got %d", holdResp.RemainingSeconds)
		}
	})

	t.Run("release hold succeeds with 200 OK", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/padel/slots/"+slotID.String()+"/release-hold", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}
		var relResp ReleaseHoldResponse
		_ = json.NewDecoder(resp.Body).Decode(&relResp)
		if relResp.Status != "AVAILABLE" {
			t.Errorf("expected status AVAILABLE, got %s", relResp.Status)
		}
	})
}
