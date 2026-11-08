package facility_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"giki-sports-backend/internal/authentication"
	"giki-sports-backend/internal/facility"
	"giki-sports-backend/internal/session"
	"giki-sports-backend/internal/user"
)

type fakeReader struct {
	facilities          []facility.Facility
	listErr             error
	getBySlugFunc       func(ctx context.Context, slug string) (*facility.Facility, error)
	updateWithAuditFunc func(ctx context.Context, facilitySlug string, scheduleID uuid.UUID, input facility.UpdateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error)
	deleteWithAuditFunc func(ctx context.Context, facilitySlug string, scheduleID uuid.UUID, audit facility.AuditParams) error
	createWithAuditFunc func(ctx context.Context, facilitySlug string, input facility.CreateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error)
	hasDeadline         bool
	observedDeadline    time.Time
}

func (f *fakeReader) List(ctx context.Context) ([]facility.Facility, error) {
	dl, ok := ctx.Deadline()
	f.hasDeadline = ok
	f.observedDeadline = dl
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.facilities, nil
}

func (f *fakeReader) GetBySlug(ctx context.Context, slug string) (*facility.Facility, error) {
	dl, ok := ctx.Deadline()
	f.hasDeadline = ok
	f.observedDeadline = dl
	if f.getBySlugFunc != nil {
		return f.getBySlugFunc(ctx, slug)
	}
	for _, fac := range f.facilities {
		if fac.Slug == slug {
			return &fac, nil
		}
	}
	return nil, facility.ErrNotFound
}

func (f *fakeReader) UpdateSchedule(ctx context.Context, facilitySlug string, scheduleID uuid.UUID, input facility.UpdateScheduleInput) (*facility.Schedule, error) {
	return nil, errors.New("not implemented in fakeReader")
}

func (f *fakeReader) UpdateScheduleWithAudit(ctx context.Context, facilitySlug string, scheduleID uuid.UUID, input facility.UpdateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
	dl, ok := ctx.Deadline()
	f.hasDeadline = ok
	f.observedDeadline = dl
	if f.updateWithAuditFunc != nil {
		return f.updateWithAuditFunc(ctx, facilitySlug, scheduleID, input, audit)
	}
	return nil, errors.New("not implemented in fakeReader")
}

func (f *fakeReader) DeleteScheduleWithAudit(ctx context.Context, facilitySlug string, scheduleID uuid.UUID, audit facility.AuditParams) error {
	dl, ok := ctx.Deadline()
	f.hasDeadline = ok
	f.observedDeadline = dl
	if f.deleteWithAuditFunc != nil {
		return f.deleteWithAuditFunc(ctx, facilitySlug, scheduleID, audit)
	}
	return errors.New("not implemented in fakeReader")
}

func (f *fakeReader) CreateScheduleWithAudit(ctx context.Context, facilitySlug string, input facility.CreateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
	dl, ok := ctx.Deadline()
	f.hasDeadline = ok
	f.observedDeadline = dl
	if f.createWithAuditFunc != nil {
		return f.createWithAuditFunc(ctx, facilitySlug, input, audit)
	}
	return nil, errors.New("not implemented in fakeReader")
}

func strPtr(s string) *string {
	return &s
}

func setupTestApp(repo facility.Repository, timeout time.Duration) *fiber.App {
	app := fiber.New()
	api := app.Group("/api")
	h := facility.NewHandler(repo, timeout)
	h.RegisterRoutes(api)
	return app
}

func setupUpdateTestApp(fake *fakeReader, timeout time.Duration, principal *authentication.AuthenticatedPrincipal) *fiber.App {
	app := fiber.New()
	api := app.Group("/api")

	if principal != nil {
		api.Use(func(c *fiber.Ctx) error {
			authentication.SetPrincipalForTest(c, principal)
			return c.Next()
		})
	}

	h := facility.NewHandler(fake, timeout)
	h.RegisterRoutes(api)
	h.RegisterScheduleUpdateRoute(api)
	h.RegisterScheduleDeleteRoute(api)
	h.RegisterScheduleCreateRoute(api)
	return app
}

func samplePrincipal(role user.Role) *authentication.AuthenticatedPrincipal {
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	return &authentication.AuthenticatedPrincipal{
		User: &user.User{
			ID:          userID,
			Email:       "director@example.test",
			DisplayName: "Director User",
			Roles:       []user.Role{role},
		},
		Session: &session.Session{
			ID:     uuid.New(),
			UserID: userID,
		},
	}
}

func TestHandler_List_Success(t *testing.T) {
	fake := &fakeReader{
		facilities: []facility.Facility{
			{
				ID:          uuid.MustParse("d0000001-0000-4000-8000-000000000001"),
				Slug:        "demo-badminton-court",
				Name:        "[Demo] Campus Badminton Court",
				Description: strPtr("Synthetic court"),
				Location:    strPtr("Wing B"),
				Status:      facility.StatusActive,
				Schedules: []facility.Schedule{
					{
						ID:         uuid.MustParse("d0000001-0000-4000-8000-000000000011"),
						FacilityID: uuid.MustParse("d0000001-0000-4000-8000-000000000001"),
						DayOfWeek:  1,
						StartTime:  "09:00:00",
						EndTime:    "12:00:00",
						Audience:   "student",
					},
				},
			},
		},
	}

	app := setupTestApp(fake, 2*time.Second)
	req := httptest.NewRequest("GET", "/api/facilities", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var list []map[string]any
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 facility, got %d", len(list))
	}

	f := list[0]
	// Verify exact camelCase field names
	expectedKeys := []string{"id", "slug", "name", "description", "location", "status", "schedules"}
	for _, k := range expectedKeys {
		if _, ok := f[k]; !ok {
			t.Errorf("missing expected JSON property %q in facility response: %s", k, string(body))
		}
	}

	scheds := f["schedules"].([]any)
	if len(scheds) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(scheds))
	}
	s := scheds[0].(map[string]any)
	expectedSchedKeys := []string{"id", "dayOfWeek", "startTime", "endTime", "audience"}
	for _, k := range expectedSchedKeys {
		if _, ok := s[k]; !ok {
			t.Errorf("missing expected JSON property %q in schedule response: %s", k, string(body))
		}
	}

	if s["startTime"] != "09:00:00" || s["endTime"] != "12:00:00" {
		t.Errorf("unexpected schedule time strings: %+v", s)
	}

	// Verify incoming context was bounded
	if !fake.hasDeadline {
		t.Errorf("expected repository call to receive a bounded context with deadline")
	}
}

func TestHandler_List_EmptyArray(t *testing.T) {
	fake := &fakeReader{
		facilities: []facility.Facility{},
	}

	app := setupTestApp(fake, 2*time.Second)
	req := httptest.NewRequest("GET", "/api/facilities", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("expected empty JSON array '[]', got: %q", string(body))
	}
}

func TestHandler_GetBySlug_Success(t *testing.T) {
	fake := &fakeReader{
		facilities: []facility.Facility{
			{
				ID:          uuid.MustParse("d0000001-0000-4000-8000-000000000001"),
				Slug:        "demo-badminton-court",
				Name:        "[Demo] Campus Badminton Court",
				Description: strPtr("Synthetic court"),
				Location:    strPtr("Wing B"),
				Status:      facility.StatusActive,
				Schedules: []facility.Schedule{
					{
						ID:        uuid.MustParse("d0000001-0000-4000-8000-000000000011"),
						DayOfWeek: 6,
						StartTime: "22:00:00",
						EndTime:   "24:00:00",
						Audience:  "general",
					},
				},
			},
		},
	}

	app := setupTestApp(fake, 2*time.Second)
	req := httptest.NewRequest("GET", "/api/facilities/demo-badminton-court", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var f facility.FacilityResponse
	if err := json.Unmarshal(body, &f); err != nil {
		t.Fatalf("failed to decode facility JSON: %v", err)
	}

	if f.Slug != "demo-badminton-court" || len(f.Schedules) != 1 {
		t.Errorf("unexpected facility response: %+v", f)
	}
	if f.Schedules[0].EndTime != "24:00:00" {
		t.Errorf("expected EndTime '24:00:00' preserved, got %q", f.Schedules[0].EndTime)
	}
}

func TestHandler_GetBySlug_FacilityWithoutSchedules(t *testing.T) {
	fake := &fakeReader{
		facilities: []facility.Facility{
			{
				ID:        uuid.MustParse("d0000002-0000-4000-8000-000000000002"),
				Slug:      "empty-schedules-fac",
				Name:      "Facility No Schedules",
				Status:    facility.StatusActive,
				Schedules: nil, // Domain model has nil or empty slice
			},
		},
	}

	app := setupTestApp(fake, 2*time.Second)
	req := httptest.NewRequest("GET", "/api/facilities/empty-schedules-fac", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	scheds, ok := raw["schedules"]
	if !ok || scheds == nil {
		t.Fatalf("expected non-nil 'schedules' array in JSON, got nil or missing: %s", string(body))
	}
	if arr, ok := scheds.([]any); !ok || len(arr) != 0 {
		t.Errorf("expected empty array [], got %v", scheds)
	}
}

func TestHandler_GetBySlug_NullableFields(t *testing.T) {
	fake := &fakeReader{
		facilities: []facility.Facility{
			{
				ID:          uuid.MustParse("d0000003-0000-4000-8000-000000000003"),
				Slug:        "null-fields-fac",
				Name:        "Null Fields Facility",
				Description: nil,
				Location:    nil,
				Status:      facility.StatusClosed,
			},
		},
	}

	app := setupTestApp(fake, 2*time.Second)
	req := httptest.NewRequest("GET", "/api/facilities/null-fields-fac", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if raw["description"] != nil {
		t.Errorf("expected description to be JSON null, got %v", raw["description"])
	}
	if raw["location"] != nil {
		t.Errorf("expected location to be JSON null, got %v", raw["location"])
	}
}

func TestHandler_GetBySlug_UnknownSlug(t *testing.T) {
	fake := &fakeReader{
		facilities: []facility.Facility{},
	}

	app := setupTestApp(fake, 2*time.Second)
	req := httptest.NewRequest("GET", "/api/facilities/does-not-exist", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var errResp facility.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to decode error JSON: %v", err)
	}

	if errResp.Error != "facility not found" {
		t.Errorf("expected error 'facility not found', got %q", errResp.Error)
	}
}

func TestHandler_InternalError_Sanitized(t *testing.T) {
	fake := &fakeReader{
		listErr: errors.New("FATAL: SELECT password_hash FROM auth_users WHERE secret='leak'"),
	}

	app := setupTestApp(fake, 2*time.Second)
	req := httptest.NewRequest("GET", "/api/facilities", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var errResp facility.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to decode error JSON: %v", err)
	}

	// Must be stable generic error
	if errResp.Error != "internal server error" {
		t.Errorf("expected generic error 'internal server error', got %q", errResp.Error)
	}

	// Must NOT leak SQL or sensitive words
	for _, leaked := range []string{"SELECT", "password_hash", "auth_users", "leak"} {
		if strings.Contains(string(body), leaked) {
			t.Errorf("SECURITY LEAK: response body leaked internal text %q: %s", leaked, string(body))
		}
	}
}

func TestHandler_DeadlineExceeded_MapsTo503(t *testing.T) {
	fake := &fakeReader{
		listErr: context.DeadlineExceeded,
	}

	app := setupTestApp(fake, 2*time.Second)
	req := httptest.NewRequest("GET", "/api/facilities", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var errResp facility.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to decode error JSON: %v", err)
	}

	if errResp.Error != "service unavailable: request timed out" {
		t.Errorf("expected error 'service unavailable: request timed out', got %q", errResp.Error)
	}
}

func TestHandler_ContextIsBounded(t *testing.T) {
	fake := &fakeReader{
		facilities: []facility.Facility{},
	}

	timeout := 1500 * time.Millisecond
	app := setupTestApp(fake, timeout)
	req := httptest.NewRequest("GET", "/api/facilities", nil)
	_, _ = app.Test(req)

	if !fake.hasDeadline {
		t.Fatalf("expected context to have a deadline set by handler")
	}

	remaining := time.Until(fake.observedDeadline)
	if remaining <= 0 || remaining > timeout {
		t.Errorf("expected deadline remaining to be between 0 and %s, got %s", timeout, remaining)
	}
}

func TestHandler_UpdateSchedule_Success(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	targetID := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	var observedActor uuid.UUID
	var observedInput facility.UpdateScheduleInput
	var observedSlug string
	var observedID uuid.UUID

	fake := &fakeReader{
		updateWithAuditFunc: func(ctx context.Context, slug string, schedID uuid.UUID, in facility.UpdateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
			observedSlug = slug
			observedID = schedID
			observedInput = in
			observedActor = audit.ActorID
			return &facility.Schedule{
				ID:         schedID,
				FacilityID: uuid.New(),
				DayOfWeek:  in.DayOfWeek,
				StartTime:  in.StartTime,
				EndTime:    in.EndTime,
				Audience:   in.Audience,
				CreatedAt:  time.Now().Add(-time.Hour),
				UpdatedAt:  time.Now(),
			}, nil
		},
	}

	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	payload := `{"dayOfWeek":3,"startTime":"14:00:00","endTime":"16:30:00","audience":"women_only"}`
	req := httptest.NewRequest("PUT", "/api/facilities/badminton-court/schedules/"+targetID.String(), strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var res facility.ScheduleResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.ID != targetID.String() {
		t.Errorf("expected schedule ID %s, got %s", targetID, res.ID)
	}
	if res.DayOfWeek != 3 || res.StartTime != "14:00:00" || res.EndTime != "16:30:00" || res.Audience != "women_only" {
		t.Errorf("unexpected response fields: %+v", res)
	}

	// Verify actor passed to repository was extracted from principal
	if observedActor != principal.User.ID {
		t.Errorf("expected actor ID %s, got %s", principal.User.ID, observedActor)
	}
	if observedSlug != "badminton-court" || observedID != targetID {
		t.Errorf("expected slug 'badminton-court' and id %s, got slug %q and id %s", targetID, observedSlug, observedID)
	}
	if observedInput.DayOfWeek != 3 || observedInput.StartTime != "14:00:00" || observedInput.EndTime != "16:30:00" || observedInput.Audience != "women_only" {
		t.Errorf("unexpected input passed to repo: %+v", observedInput)
	}
}

func TestHandler_UpdateSchedule_EndTime24(t *testing.T) {
	principal := samplePrincipal(user.RoleSuperAdmin)
	targetID := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	fake := &fakeReader{
		updateWithAuditFunc: func(ctx context.Context, slug string, schedID uuid.UUID, in facility.UpdateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
			return &facility.Schedule{
				ID:        schedID,
				DayOfWeek: in.DayOfWeek,
				StartTime: in.StartTime,
				EndTime:   in.EndTime,
				Audience:  in.Audience,
			}, nil
		},
	}

	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	payload := `{"dayOfWeek":6,"startTime":"20:00:00","endTime":"24:00:00","audience":"general"}`
	req := httptest.NewRequest("PUT", "/api/facilities/pool/schedules/"+targetID.String(), strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var res facility.ScheduleResponse
	_ = json.NewDecoder(resp.Body).Decode(&res)
	if res.EndTime != "24:00:00" {
		t.Errorf("expected endTime '24:00:00', got %q", res.EndTime)
	}
}

func TestHandler_UpdateSchedule_UnauthorizedWhenNoPrincipal(t *testing.T) {
	fake := &fakeReader{}
	// Nil principal -> unauthenticated
	app := setupUpdateTestApp(fake, 5*time.Second, nil)

	payload := `{"dayOfWeek":1,"startTime":"09:00:00","endTime":"11:00:00","audience":"student"}`
	req := httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+uuid.New().String(), strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

func TestHandler_UpdateSchedule_InvalidScheduleUUID(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	payload := `{"dayOfWeek":1,"startTime":"09:00:00","endTime":"11:00:00","audience":"student"}`

	for _, invalidID := range []string{"not-a-uuid", "123", "00000000-0000-0000-0000-000000000000"} {
		req := httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+invalidID, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("expected 400 Bad Request for id %q, got %d", invalidID, resp.StatusCode)
		}
	}
}

func TestHandler_UpdateSchedule_UnsupportedMediaType(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	schedID := uuid.New().String()
	payload := `{"dayOfWeek":1,"startTime":"09:00:00","endTime":"11:00:00","audience":"student"}`

	// 1. Missing Content-Type
	req := httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+schedID, strings.NewReader(payload))
	resp, _ := app.Test(req)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnsupportedMediaType {
		t.Errorf("expected 415 for missing Content-Type, got %d", resp.StatusCode)
	}

	// 2. text/plain Content-Type
	req = httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+schedID, strings.NewReader(payload))
	req.Header.Set("Content-Type", "text/plain")
	resp, _ = app.Test(req)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnsupportedMediaType {
		t.Errorf("expected 415 for text/plain, got %d", resp.StatusCode)
	}
}

func TestHandler_UpdateSchedule_EmptyAndOversizedBody(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	schedID := uuid.New().String()

	// 1. Empty body -> 400 Bad Request
	req := httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+schedID, strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for empty body, got %d", resp.StatusCode)
	}

	// 2. Oversized body (> 4096 bytes) -> 413 Request Entity Too Large
	bigBody := fmt.Sprintf(`{"dayOfWeek":1,"startTime":"09:00:00","endTime":"11:00:00","audience":"student","padding":"%s"}`, strings.Repeat("A", 5000))
	req = httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+schedID, strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 for oversized body, got %d", resp.StatusCode)
	}
}

func TestHandler_UpdateSchedule_MalformedAndUnknownFields(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	schedID := uuid.New().String()

	// 1. Malformed JSON syntax
	req := httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+schedID, strings.NewReader(`{invalid-json}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for malformed JSON, got %d", resp.StatusCode)
	}

	// 2. Unknown field rejected by dec.DisallowUnknownFields()
	req = httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+schedID, strings.NewReader(`{"dayOfWeek":1,"startTime":"09:00:00","endTime":"11:00:00","audience":"student","unknownField":"hack"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for unknown field, got %d", resp.StatusCode)
	}
}

func TestHandler_UpdateSchedule_DomainValidationFailure(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	schedID := uuid.New().String()

	// Invalid day (8) -> 400
	req := httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+schedID, strings.NewReader(`{"dayOfWeek":8,"startTime":"09:00:00","endTime":"11:00:00","audience":"student"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for invalid dayOfWeek, got %d", resp.StatusCode)
	}

	// Inverted times (start > end) -> 400
	req = httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+schedID, strings.NewReader(`{"dayOfWeek":1,"startTime":"15:00:00","endTime":"11:00:00","audience":"student"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for inverted times, got %d", resp.StatusCode)
	}
}

func TestHandler_UpdateSchedule_NotFound(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	schedID := uuid.New()
	fake := &fakeReader{
		updateWithAuditFunc: func(ctx context.Context, slug string, sID uuid.UUID, in facility.UpdateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
			return nil, &facility.ScheduleNotFoundError{FacilitySlug: slug, ScheduleID: sID}
		},
	}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	payload := `{"dayOfWeek":1,"startTime":"09:00:00","endTime":"11:00:00","audience":"student"}`
	req := httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+schedID.String(), strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", resp.StatusCode)
	}
}

func TestHandler_UpdateSchedule_Duplicate(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	schedID := uuid.New()
	fake := &fakeReader{
		updateWithAuditFunc: func(ctx context.Context, slug string, sID uuid.UUID, in facility.UpdateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
			return nil, &facility.DuplicateScheduleError{FacilitySlug: slug, ScheduleID: sID}
		},
	}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	payload := `{"dayOfWeek":1,"startTime":"09:00:00","endTime":"11:00:00","audience":"student"}`
	req := httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+schedID.String(), strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusConflict {
		t.Errorf("expected 409 Conflict, got %d", resp.StatusCode)
	}
}

func TestHandler_UpdateSchedule_DeadlineExceeded(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	schedID := uuid.New()
	fake := &fakeReader{
		updateWithAuditFunc: func(ctx context.Context, slug string, sID uuid.UUID, in facility.UpdateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
			return nil, context.DeadlineExceeded
		},
	}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	payload := `{"dayOfWeek":1,"startTime":"09:00:00","endTime":"11:00:00","audience":"student"}`
	req := httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+schedID.String(), strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("expected 503 Service Unavailable, got %d", resp.StatusCode)
	}
}

func TestHandler_UpdateSchedule_InternalError(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	schedID := uuid.New()
	fake := &fakeReader{
		updateWithAuditFunc: func(ctx context.Context, slug string, sID uuid.UUID, in facility.UpdateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
			return nil, errors.New("database connection reset by peer")
		},
	}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	payload := `{"dayOfWeek":1,"startTime":"09:00:00","endTime":"11:00:00","audience":"student"}`
	req := httptest.NewRequest("PUT", "/api/facilities/gym/schedules/"+schedID.String(), strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "database connection reset") {
		t.Errorf("leak detected: internal error exposed to client: %s", string(body))
	}
}

func TestHandler_DeleteSchedule_Success(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	schedID := uuid.New()
	var capturedSlug string
	var capturedSchedID uuid.UUID
	var capturedAudit facility.AuditParams

	fake := &fakeReader{
		deleteWithAuditFunc: func(ctx context.Context, slug string, sID uuid.UUID, audit facility.AuditParams) error {
			capturedSlug = slug
			capturedSchedID = sID
			capturedAudit = audit
			return nil
		},
	}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	req := httptest.NewRequest("DELETE", "/api/facilities/demo-court/schedules/"+schedID.String(), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("expected empty body for 204 No Content, got: %s", string(body))
	}

	if cacheCtrl := resp.Header.Get("Cache-Control"); !strings.Contains(cacheCtrl, "no-store") {
		t.Errorf("expected Cache-Control: no-store, got: %q", cacheCtrl)
	}

	if capturedSlug != "demo-court" {
		t.Errorf("expected slug 'demo-court', got %q", capturedSlug)
	}
	if capturedSchedID != schedID {
		t.Errorf("expected schedule ID %s, got %s", schedID, capturedSchedID)
	}
	if capturedAudit.ActorID != principal.User.ID {
		t.Errorf("expected audit ActorID %s, got %s", principal.User.ID, capturedAudit.ActorID)
	}
}

func TestHandler_DeleteSchedule_InvalidScheduleUUID(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	req := httptest.NewRequest("DELETE", "/api/facilities/demo-court/schedules/not-a-uuid", nil)
	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for invalid UUID, got %d", resp.StatusCode)
	}
}

func TestHandler_DeleteSchedule_UnauthorizedWhenNoPrincipal(t *testing.T) {
	fake := &fakeReader{}
	app := setupUpdateTestApp(fake, 5*time.Second, nil) // no principal

	req := httptest.NewRequest("DELETE", "/api/facilities/demo-court/schedules/"+uuid.New().String(), nil)
	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized when no principal, got %d", resp.StatusCode)
	}
}

func TestHandler_DeleteSchedule_NotFound(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	schedID := uuid.New()
	fake := &fakeReader{
		deleteWithAuditFunc: func(ctx context.Context, slug string, sID uuid.UUID, audit facility.AuditParams) error {
			return &facility.ScheduleNotFoundError{FacilitySlug: slug, ScheduleID: sID}
		},
	}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	req := httptest.NewRequest("DELETE", "/api/facilities/demo-court/schedules/"+schedID.String(), nil)
	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", resp.StatusCode)
	}
}

func TestHandler_DeleteSchedule_DeadlineExceeded(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	schedID := uuid.New()
	fake := &fakeReader{
		deleteWithAuditFunc: func(ctx context.Context, slug string, sID uuid.UUID, audit facility.AuditParams) error {
			return context.DeadlineExceeded
		},
	}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	req := httptest.NewRequest("DELETE", "/api/facilities/demo-court/schedules/"+schedID.String(), nil)
	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("expected 503 Service Unavailable, got %d", resp.StatusCode)
	}
}

func TestHandler_DeleteSchedule_InternalError(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	schedID := uuid.New()
	fake := &fakeReader{
		deleteWithAuditFunc: func(ctx context.Context, slug string, sID uuid.UUID, audit facility.AuditParams) error {
			return errors.New("database connection reset by peer")
		},
	}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	req := httptest.NewRequest("DELETE", "/api/facilities/demo-court/schedules/"+schedID.String(), nil)
	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "database connection reset") {
		t.Errorf("leak detected: internal error exposed to client: %s", string(body))
	}
}

func TestHandler_DeleteSchedule_ContextIsBounded(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	schedID := uuid.New()
	fake := &fakeReader{
		deleteWithAuditFunc: func(ctx context.Context, slug string, sID uuid.UUID, audit facility.AuditParams) error {
			return nil
		},
	}
	app := setupUpdateTestApp(fake, 3*time.Second, principal)

	req := httptest.NewRequest("DELETE", "/api/facilities/demo-court/schedules/"+schedID.String(), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if !fake.hasDeadline {
		t.Errorf("expected repository call to receive a bounded context with deadline")
	}
	if time.Until(fake.observedDeadline) > 3*time.Second {
		t.Errorf("observed deadline exceeds configured timeout: %v", fake.observedDeadline)
	}
}

func TestHandler_CreateSchedule_Success(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	generatedID := uuid.New()
	facID := uuid.New()

	var observedSlug string
	var observedInput facility.CreateScheduleInput
	var observedAudit facility.AuditParams

	fake := &fakeReader{
		createWithAuditFunc: func(ctx context.Context, slug string, input facility.CreateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
			observedSlug = slug
			observedInput = input
			observedAudit = audit
			return &facility.Schedule{
				ID:         generatedID,
				FacilityID: facID,
				DayOfWeek:  input.DayOfWeek,
				StartTime:  input.StartTime,
				EndTime:    input.EndTime,
				Audience:   input.Audience,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}, nil
		},
	}

	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	bodyJSON := `{"dayOfWeek": 1, "startTime": "11:00:00", "endTime": "13:00:00", "audience": "student"}`
	req := httptest.NewRequest("POST", "/api/facilities/badminton-court/schedules", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TestClient/1.0")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("expected HTTP 201 Created, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("expected Cache-Control: no-store, got %q", resp.Header.Get("Cache-Control"))
	}

	var res facility.ScheduleResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if res.ID != generatedID.String() {
		t.Errorf("expected generated schedule ID %s, got %s", generatedID, res.ID)
	}
	if res.DayOfWeek != 1 || res.StartTime != "11:00:00" || res.EndTime != "13:00:00" || res.Audience != "student" {
		t.Errorf("unexpected schedule response: %+v", res)
	}

	if observedSlug != "badminton-court" {
		t.Errorf("expected slug 'badminton-court', got %q", observedSlug)
	}
	if observedInput.DayOfWeek != 1 || observedInput.StartTime != "11:00:00" || observedInput.EndTime != "13:00:00" || observedInput.Audience != "student" {
		t.Errorf("unexpected input passed to repo: %+v", observedInput)
	}
	if observedAudit.ActorID != principal.User.ID {
		t.Errorf("expected audit actor ID %s, got %s", principal.User.ID, observedAudit.ActorID)
	}
}

func TestHandler_CreateSchedule_UnauthorizedWhenNoPrincipal(t *testing.T) {
	fake := &fakeReader{}
	app := setupUpdateTestApp(fake, 5*time.Second, nil) // no principal

	bodyJSON := `{"dayOfWeek": 1, "startTime": "11:00:00", "endTime": "13:00:00", "audience": "student"}`
	req := httptest.NewRequest("POST", "/api/facilities/badminton-court/schedules", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
	}
}

func TestHandler_CreateSchedule_BlankSlug(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	bodyJSON := `{"dayOfWeek": 1, "startTime": "11:00:00", "endTime": "13:00:00", "audience": "student"}`
	req := httptest.NewRequest("POST", "/api/facilities/%20%20/schedules", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("expected 404 Not Found for blank slug, got %d", resp.StatusCode)
	}
}

func TestHandler_CreateSchedule_UnsupportedMediaType(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	req := httptest.NewRequest("POST", "/api/facilities/badminton-court/schedules", strings.NewReader("plain text"))
	req.Header.Set("Content-Type", "text/plain")

	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusUnsupportedMediaType {
		t.Errorf("expected 415 Unsupported Media Type, got %d", resp.StatusCode)
	}
}

func TestHandler_CreateSchedule_EmptyAndOversizedBody(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	// Empty body -> 400
	reqEmpty := httptest.NewRequest("POST", "/api/facilities/badminton-court/schedules", bytes.NewReader([]byte{}))
	reqEmpty.Header.Set("Content-Type", "application/json")
	respEmpty, _ := app.Test(reqEmpty)
	respEmpty.Body.Close()
	if respEmpty.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for empty body, got %d", respEmpty.StatusCode)
	}

	// Oversized body > 4KB -> 413
	oversized := bytes.Repeat([]byte("a"), 5000)
	reqLarge := httptest.NewRequest("POST", "/api/facilities/badminton-court/schedules", bytes.NewReader(oversized))
	reqLarge.Header.Set("Content-Type", "application/json")
	respLarge, _ := app.Test(reqLarge)
	respLarge.Body.Close()
	if respLarge.StatusCode != fiber.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 Payload Too Large, got %d", respLarge.StatusCode)
	}
}

func TestHandler_CreateSchedule_MalformedAndUnknownFields(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	// Malformed JSON syntax -> 400
	reqBad := httptest.NewRequest("POST", "/api/facilities/badminton-court/schedules", strings.NewReader(`{"dayOfWeek": 1,`))
	reqBad.Header.Set("Content-Type", "application/json")
	respBad, _ := app.Test(reqBad)
	respBad.Body.Close()
	if respBad.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for malformed JSON, got %d", respBad.StatusCode)
	}

	// Unknown field (client attempts to inject actorId or id) -> 400
	reqUnknown := httptest.NewRequest("POST", "/api/facilities/badminton-court/schedules", strings.NewReader(`{
		"dayOfWeek": 1,
		"startTime": "09:00:00",
		"endTime": "12:00:00",
		"audience": "student",
		"actorId": "00000000-0000-0000-0000-000000000001"
	}`))
	reqUnknown.Header.Set("Content-Type", "application/json")
	respUnknown, _ := app.Test(reqUnknown)
	respUnknown.Body.Close()
	if respUnknown.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for unknown fields, got %d", respUnknown.StatusCode)
	}

	// Extra tokens after JSON object -> 400
	reqExtra := httptest.NewRequest("POST", "/api/facilities/badminton-court/schedules", strings.NewReader(`{"dayOfWeek": 1, "startTime": "09:00:00", "endTime": "12:00:00", "audience": "student"} extra`))
	reqExtra.Header.Set("Content-Type", "application/json")
	respExtra, _ := app.Test(reqExtra)
	respExtra.Body.Close()
	if respExtra.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for extra tokens, got %d", respExtra.StatusCode)
	}
}

func TestHandler_CreateSchedule_DomainValidationFailure(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	// Start time inverted after end time
	req := httptest.NewRequest("POST", "/api/facilities/badminton-court/schedules", strings.NewReader(`{
		"dayOfWeek": 1,
		"startTime": "15:00:00",
		"endTime": "10:00:00",
		"audience": "student"
	}`))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for domain validation error, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "must be strictly before") {
		t.Errorf("expected error message to explain time order, got: %s", string(body))
	}
}

func TestHandler_CreateSchedule_NotFound(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{
		createWithAuditFunc: func(ctx context.Context, slug string, input facility.CreateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
			return nil, facility.ErrNotFound
		},
	}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	bodyJSON := `{"dayOfWeek": 1, "startTime": "11:00:00", "endTime": "13:00:00", "audience": "student"}`
	req := httptest.NewRequest("POST", "/api/facilities/unknown-court/schedules", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", resp.StatusCode)
	}
}

func TestHandler_CreateSchedule_Duplicate(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{
		createWithAuditFunc: func(ctx context.Context, slug string, input facility.CreateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
			return nil, facility.ErrDuplicateSchedule
		},
	}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	bodyJSON := `{"dayOfWeek": 1, "startTime": "11:00:00", "endTime": "13:00:00", "audience": "student"}`
	req := httptest.NewRequest("POST", "/api/facilities/demo-court/schedules", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusConflict {
		t.Errorf("expected 409 Conflict for duplicate window, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "duplicates an existing window") {
		t.Errorf("expected conflict message, got: %s", string(body))
	}
}

func TestHandler_CreateSchedule_DeadlineExceeded(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{
		createWithAuditFunc: func(ctx context.Context, slug string, input facility.CreateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
			return nil, context.DeadlineExceeded
		},
	}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	bodyJSON := `{"dayOfWeek": 1, "startTime": "11:00:00", "endTime": "13:00:00", "audience": "student"}`
	req := httptest.NewRequest("POST", "/api/facilities/demo-court/schedules", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("expected 503 Service Unavailable, got %d", resp.StatusCode)
	}
}

func TestHandler_CreateSchedule_InternalError(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{
		createWithAuditFunc: func(ctx context.Context, slug string, input facility.CreateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
			return nil, errors.New("database connection reset by peer")
		},
	}
	app := setupUpdateTestApp(fake, 5*time.Second, principal)

	bodyJSON := `{"dayOfWeek": 1, "startTime": "11:00:00", "endTime": "13:00:00", "audience": "student"}`
	req := httptest.NewRequest("POST", "/api/facilities/demo-court/schedules", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "database connection reset") {
		t.Errorf("leak detected: internal error exposed to client: %s", string(body))
	}
}

func TestHandler_CreateSchedule_ContextIsBounded(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeReader{
		createWithAuditFunc: func(ctx context.Context, slug string, input facility.CreateScheduleInput, audit facility.AuditParams) (*facility.Schedule, error) {
			return &facility.Schedule{ID: uuid.New()}, nil
		},
	}
	app := setupUpdateTestApp(fake, 3*time.Second, principal)

	bodyJSON := `{"dayOfWeek": 1, "startTime": "11:00:00", "endTime": "13:00:00", "audience": "student"}`
	req := httptest.NewRequest("POST", "/api/facilities/demo-court/schedules", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if !fake.hasDeadline {
		t.Errorf("expected repository call to receive a bounded context with deadline")
	}
	if time.Until(fake.observedDeadline) > 3*time.Second {
		t.Errorf("observed deadline exceeds configured timeout: %v", fake.observedDeadline)
	}
}
