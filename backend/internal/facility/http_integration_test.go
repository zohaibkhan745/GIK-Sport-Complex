//go:build integration

package facility_test

import (
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
	"giki-sports-backend/internal/facility"
	"giki-sports-backend/internal/session"
	"giki-sports-backend/internal/user"
)

// TestFacilityHTTP_Integration exercises the Fiber HTTP handlers with the real
// PostgreSQL repository against an isolated disposable database.
func TestFacilityHTTP_Integration(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	ctx := context.Background()

	// Insert synthetic fixtures into disposable database
	var facID string
	err := pool.QueryRow(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, description, location, status)
		VALUES ('http-test-facility', 'HTTP Test Arena', 'Synthetic description', 'North Hall', 'active')
		RETURNING id;
	`).Scan(&facID)
	if err != nil {
		t.Fatalf("failed to insert test facility: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES 
			($1, 1, '09:00:00', '11:00:00', 'student'),
			($1, 6, '22:00:00', '24:00:00', 'general');
	`, facID)
	if err != nil {
		t.Fatalf("failed to insert test schedules: %v", err)
	}

	// Construct repository and HTTP handler
	repo := facility.NewRepository(pool)
	handler := facility.NewHandler(repo, 5*time.Second)

	app := fiber.New()
	api := app.Group("/api")
	handler.RegisterRoutes(api)

	// 1. Test GET /api/facilities
	reqList := httptest.NewRequest("GET", "/api/facilities", nil)
	respList, err := app.Test(reqList)
	if err != nil {
		t.Fatalf("GET /api/facilities failed: %v", err)
	}
	defer respList.Body.Close()

	if respList.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", respList.StatusCode)
	}

	bodyList, _ := io.ReadAll(respList.Body)
	var facilities []facility.FacilityResponse
	if err := json.Unmarshal(bodyList, &facilities); err != nil {
		t.Fatalf("failed to unmarshal facilities JSON: %v", err)
	}

	if len(facilities) != 1 {
		t.Fatalf("expected 1 facility, got %d", len(facilities))
	}
	if facilities[0].Slug != "http-test-facility" {
		t.Errorf("expected slug 'http-test-facility', got %q", facilities[0].Slug)
	}
	if len(facilities[0].Schedules) != 2 {
		t.Fatalf("expected 2 schedules, got %d", len(facilities[0].Schedules))
	}

	// 2. Test GET /api/facilities/http-test-facility (Detail)
	reqDetail := httptest.NewRequest("GET", "/api/facilities/http-test-facility", nil)
	respDetail, err := app.Test(reqDetail)
	if err != nil {
		t.Fatalf("GET /api/facilities/:slug failed: %v", err)
	}
	defer respDetail.Body.Close()

	if respDetail.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", respDetail.StatusCode)
	}

	bodyDetail, _ := io.ReadAll(respDetail.Body)
	var detail facility.FacilityResponse
	if err := json.Unmarshal(bodyDetail, &detail); err != nil {
		t.Fatalf("failed to unmarshal detail JSON: %v", err)
	}

	if detail.Name != "HTTP Test Arena" {
		t.Errorf("expected name 'HTTP Test Arena', got %q", detail.Name)
	}
	if detail.Schedules[1].EndTime != "24:00:00" {
		t.Errorf("expected '24:00:00' preserved in HTTP response, got %q", detail.Schedules[1].EndTime)
	}

	// 3. Test GET /api/facilities/does-not-exist (Unknown slug -> 404)
	reqMissing := httptest.NewRequest("GET", "/api/facilities/does-not-exist", nil)
	respMissing, err := app.Test(reqMissing)
	if err != nil {
		t.Fatalf("GET missing slug failed: %v", err)
	}
	defer respMissing.Body.Close()

	if respMissing.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected status 404 for missing slug, got %d", respMissing.StatusCode)
	}

	bodyMissing, _ := io.ReadAll(respMissing.Body)
	var errResp facility.ErrorResponse
	if err := json.Unmarshal(bodyMissing, &errResp); err != nil {
		t.Fatalf("failed to unmarshal error JSON: %v", err)
	}
	if errResp.Error != "facility not found" {
		t.Errorf("expected 'facility not found', got %q", errResp.Error)
	}
}

// TestFacilityScheduleUpdateHTTP_Integration exercises the complete Step 7D protected PUT endpoint
// against a real PostgreSQL disposable database with the real authentication service, sessions,
// and audit trail.
func TestFacilityScheduleUpdateHTTP_Integration(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	ctx := context.Background()

	// 1. Initialize repositories and authentication service
	facilityRepo := facility.NewRepository(pool)
	userRepo := user.NewRepository(pool)
	sessionRepo := session.NewRepository(pool)

	authService, err := authentication.NewService(userRepo, sessionRepo)
	if err != nil {
		t.Fatalf("failed to initialize auth service: %v", err)
	}

	authCfg := config.AuthConfig{
		CookieName:          "giki_session",
		CookieSecure:        false,
		CookiePath:          "/api",
		CookieSameSite:      "Lax",
		TrustedOrigins:      []string{"http://127.0.0.1:5173", "http://localhost:5173"},
		LoginMaxConcurrent:  4,
		RequestTimeout:      5 * time.Second,
		LoginRateLimit:      50,
		LoginRateWindow:     time.Minute,
	}

	authHandler := authentication.NewHandler(authService, authCfg)
	facilityHandler := facility.NewHandler(facilityRepo, 5*time.Second)

	app := fiber.New()
	api := app.Group("/api")

	authHandler.RegisterRoutes(api)
	facilityHandler.RegisterRoutes(api)
	facilityHandler.RegisterScheduleUpdateRoute(api,
		authHandler.RequireTrustedOrigin(),
		authHandler.RequireAuth(),
		authentication.RequireAnyRole(user.RoleSportsDirector, user.RoleSuperAdmin),
	)

	// 2. Hash shared password once for synthetic test users
	passHash, err := auth.Hash("SecurePass123!")
	if err != nil {
		t.Fatalf("failed to hash test password: %v", err)
	}

	// 3. Seed test users with distinct roles
	type seedUser struct {
		id    uuid.UUID
		email string
		name  string
		role  user.Role
	}
	directorID := uuid.New()
	superAdminID := uuid.New()
	studentID := uuid.New()
	facultyID := uuid.New()

	users := []seedUser{
		{id: directorID, email: "director@giki.test", name: "Sports Director", role: user.RoleSportsDirector},
		{id: superAdminID, email: "superadmin@giki.test", name: "Super Admin", role: user.RoleSuperAdmin},
		{id: studentID, email: "student@giki.test", name: "Student User", role: user.RoleStudent},
		{id: facultyID, email: "faculty@giki.test", name: "Faculty User", role: user.RoleFaculty},
	}

	for _, u := range users {
		_, err := pool.Exec(ctx, `
			INSERT INTO giki_sports.users (id, email, display_name, password_hash)
			VALUES ($1, $2, $3, $4);
		`, u.id, u.email, u.name, passHash)
		if err != nil {
			t.Fatalf("failed to seed user %s: %v", u.email, err)
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.user_roles (user_id, role)
			VALUES ($1, $2);
		`, u.id, u.role)
		if err != nil {
			t.Fatalf("failed to seed role for %s: %v", u.email, err)
		}
	}

	// Helper to login and extract session cookie
	loginUser := func(email string) string {
		payload := fmt.Sprintf(`{"email":%q,"password":"SecurePass123!"}`, email)
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(payload))
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
				return c.Value
			}
		}
		t.Fatalf("session cookie not set for %s", email)
		return ""
	}

	directorCookie := loginUser("director@giki.test")
	superAdminCookie := loginUser("superadmin@giki.test")
	studentCookie := loginUser("student@giki.test")
	facultyCookie := loginUser("faculty@giki.test")

	// 4. Seed test facility and two initial schedules
	var facID, sched1ID, sched2ID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, description, location, status)
		VALUES ('badminton-hall', 'Badminton Hall', 'Primary badminton hall', 'Sports Complex Level 1', 'active')
		RETURNING id;
	`).Scan(&facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 1, '09:00:00', '12:00:00', 'student')
		RETURNING id;
	`, facID).Scan(&sched1ID)
	if err != nil {
		t.Fatalf("failed to seed schedule 1: %v", err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 2, '14:00:00', '17:00:00', 'general')
		RETURNING id;
	`, facID).Scan(&sched2ID)
	if err != nil {
		t.Fatalf("failed to seed schedule 2: %v", err)
	}

	putURL := fmt.Sprintf("/api/facilities/badminton-hall/schedules/%s", sched1ID)
	validEditBody := `{"dayOfWeek":3,"startTime":"10:00:00","endTime":"13:00:00","audience":"women_only"}`

	// -------------------------------------------------------------
	// Verification 1: Anonymous request (missing cookie) -> 401
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("PUT", putURL, strings.NewReader(validEditBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		resp, _ := app.Test(req)
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("expected 401 for anonymous request, got %d", resp.StatusCode)
		}
	}

	// -------------------------------------------------------------
	// Verification 2: Expired / revoked session cookie -> 401
	// -------------------------------------------------------------
	{
		tempCookie := loginUser("director@giki.test")
		// Revoke the session directly in DB (using GREATEST to guarantee revoked_at >= created_at even if host/docker clocks skew)
		_, err := pool.Exec(ctx, "UPDATE giki_sports.sessions SET revoked_at = GREATEST(now(), created_at) WHERE user_id = $1;", directorID)
		if err != nil {
			t.Fatalf("failed to revoke session in DB: %v", err)
		}

		req := httptest.NewRequest("PUT", putURL, strings.NewReader(validEditBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", tempCookie))
		resp, _ := app.Test(req)
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("expected 401 for revoked session, got %d", resp.StatusCode)
		}

		// Re-login director to obtain fresh active session
		directorCookie = loginUser("director@giki.test")
	}

	// -------------------------------------------------------------
	// Verification 3: Student role -> 403 Forbidden
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("PUT", putURL, strings.NewReader(validEditBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", studentCookie))
		resp, _ := app.Test(req)
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("expected 403 for student role, got %d", resp.StatusCode)
		}
	}

	// -------------------------------------------------------------
	// Verification 4: Faculty role -> 403 Forbidden
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("PUT", putURL, strings.NewReader(validEditBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", facultyCookie))
		resp, _ := app.Test(req)
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("expected 403 for faculty role, got %d", resp.StatusCode)
		}
	}

	// -------------------------------------------------------------
	// Verification 5: Missing Origin -> 403 Forbidden
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("PUT", putURL, strings.NewReader(validEditBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))
		// Origin header omitted!
		resp, _ := app.Test(req)
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("expected 403 for missing Origin, got %d", resp.StatusCode)
		}
	}

	// -------------------------------------------------------------
	// Verification 6: Untrusted Origin -> 403 Forbidden
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("PUT", putURL, strings.NewReader(validEditBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://malicious-origin.com")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))
		resp, _ := app.Test(req)
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("expected 403 for untrusted Origin, got %d", resp.StatusCode)
		}
	}

	// -------------------------------------------------------------
	// Verification 7: Director saves valid edit -> 200 OK + audit row + anonymous GET
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("PUT", putURL, strings.NewReader(validEditBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))

		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("director PUT request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200 OK for director edit, got %d: %s", resp.StatusCode, string(body))
		}

		var schedResp facility.ScheduleResponse
		if err := json.NewDecoder(resp.Body).Decode(&schedResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if schedResp.DayOfWeek != 3 || schedResp.StartTime != "10:00:00" || schedResp.EndTime != "13:00:00" || schedResp.Audience != "women_only" {
			t.Errorf("unexpected response content: %+v", schedResp)
		}

		// Verify exactly 1 audit row written with director's User ID
		var auditCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
		if auditCount != 1 {
			t.Fatalf("expected exactly 1 audit log row, got %d", auditCount)
		}

		var auditActorID, auditAction, auditTargetType string
		var auditTargetID uuid.UUID
		err = pool.QueryRow(ctx, `
			SELECT actor_id, action, target_type, target_id
			FROM giki_sports.audit_logs
			LIMIT 1;
		`).Scan(&auditActorID, &auditAction, &auditTargetType, &auditTargetID)
		if err != nil {
			t.Fatalf("failed to query audit log: %v", err)
		}

		if auditActorID != directorID.String() {
			t.Errorf("expected audit actor_id %s, got %s", directorID, auditActorID)
		}
		if auditAction != "schedule.update" || auditTargetType != "facility_schedule" || auditTargetID != sched1ID {
			t.Errorf("unexpected audit entry: action=%s, target_type=%s, target_id=%s", auditAction, auditTargetType, auditTargetID)
		}

		// Anonymous GET returns the updated timetable
		getReq := httptest.NewRequest("GET", "/api/facilities/badminton-hall", nil)
		getResp, err := app.Test(getReq)
		if err != nil {
			t.Fatalf("anonymous GET failed: %v", err)
		}
		defer getResp.Body.Close()

		var facDetail facility.FacilityResponse
		if err := json.NewDecoder(getResp.Body).Decode(&facDetail); err != nil {
			t.Fatalf("failed to decode GET facility: %v", err)
		}

		foundUpdated := false
		for _, s := range facDetail.Schedules {
			if s.ID == sched1ID.String() {
				foundUpdated = true
				if s.DayOfWeek != 3 || s.StartTime != "10:00:00" || s.EndTime != "13:00:00" || s.Audience != "women_only" {
					t.Errorf("anonymous GET did not reflect updated timetable: %+v", s)
				}
			}
		}
		if !foundUpdated {
			t.Errorf("updated schedule not found in anonymous GET response")
		}
	}

	// -------------------------------------------------------------
	// Verification 8: Super admin saves valid edit -> 200 OK (preserves 24:00:00)
	// -------------------------------------------------------------
	{
		superAdminBody := `{"dayOfWeek":5,"startTime":"18:00:00","endTime":"24:00:00","audience":"general"}`
		req := httptest.NewRequest("PUT", putURL, strings.NewReader(superAdminBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", superAdminCookie))

		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("superadmin PUT request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200 OK for superadmin edit, got %d: %s", resp.StatusCode, string(body))
		}

		var schedResp facility.ScheduleResponse
		_ = json.NewDecoder(resp.Body).Decode(&schedResp)
		if schedResp.EndTime != "24:00:00" {
			t.Errorf("expected endTime '24:00:00', got %q", schedResp.EndTime)
		}

		// Verify audit log now has 2 entries, latest is super admin
		var auditCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
		if auditCount != 2 {
			t.Fatalf("expected 2 audit logs, got %d", auditCount)
		}

		var latestActorID string
		_ = pool.QueryRow(ctx, "SELECT actor_id FROM giki_sports.audit_logs ORDER BY created_at DESC LIMIT 1;").Scan(&latestActorID)
		if latestActorID != superAdminID.String() {
			t.Errorf("expected latest audit actor_id %s, got %s", superAdminID, latestActorID)
		}
	}

	// -------------------------------------------------------------
	// Verification 9: Denied, invalid, mismatched, and duplicate requests leave schedule and audit data unchanged
	// -------------------------------------------------------------
	{
		auditCountBefore := 2

		// A. Invalid window: start after end
		invalidBody := `{"dayOfWeek":1,"startTime":"16:00:00","endTime":"10:00:00","audience":"student"}`
		req := httptest.NewRequest("PUT", putURL, strings.NewReader(invalidBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))
		resp, _ := app.Test(req)
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("expected 400 for invalid window, got %d", resp.StatusCode)
		}

		// B. Mismatched slug with existing schedule ID
		mismatchedURL := fmt.Sprintf("/api/facilities/wrong-facility-slug/schedules/%s", sched1ID)
		req = httptest.NewRequest("PUT", mismatchedURL, strings.NewReader(validEditBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))
		resp, _ = app.Test(req)
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("expected 404 for mismatched slug, got %d", resp.StatusCode)
		}

		// C. Duplicate collision with schedule 2 (day 2, 14:00-17:00, general)
		dupBody := `{"dayOfWeek":2,"startTime":"14:00:00","endTime":"17:00:00","audience":"general"}`
		req = httptest.NewRequest("PUT", putURL, strings.NewReader(dupBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))
		resp, _ = app.Test(req)
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusConflict {
			t.Errorf("expected 409 Conflict for duplicate window, got %d", resp.StatusCode)
		}

		// Assert audit log count is completely unchanged
		var auditCountAfter int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCountAfter)
		if auditCountAfter != auditCountBefore {
			t.Errorf("audit logs changed after rejected requests: before=%d, after=%d", auditCountBefore, auditCountAfter)
		}

		// Assert schedule 1 row remains unchanged (day 5, 18:00:00-24:00:00, general)
		var currDay int
		var currEnd, currAudience string
		_ = pool.QueryRow(ctx, "SELECT day_of_week, to_char(end_time, 'HH24:MI:SS'), audience FROM giki_sports.schedules WHERE id = $1;", sched1ID).Scan(&currDay, &currEnd, &currAudience)
		if currDay != 5 || currEnd != "24:00:00" || currAudience != "general" {
			t.Errorf("schedule was mutated by rejected requests: day=%d, end=%s, aud=%s", currDay, currEnd, currAudience)
		}
	}

	// -------------------------------------------------------------
	// Verification 10: Forced audit failure returns generic error and rolls back schedule update
	// -------------------------------------------------------------
	{
		// Install trigger on audit_logs that raises an exception on insert
		_, err := pool.Exec(ctx, `
			CREATE OR REPLACE FUNCTION fail_audit() RETURNS trigger AS $$
			BEGIN
				RAISE EXCEPTION 'forced audit table failure for atomicity test';
			END;
			$$ LANGUAGE plpgsql;
		`)
		if err != nil {
			t.Fatalf("failed to create fail_audit function: %v", err)
		}

		_, err = pool.Exec(ctx, `
			CREATE TRIGGER trg_fail_audit BEFORE INSERT ON giki_sports.audit_logs
				FOR EACH ROW EXECUTE FUNCTION fail_audit();
		`)
		if err != nil {
			t.Fatalf("failed to install test trigger: %v", err)
		}

		// Attempt valid update while audit insert is guaranteed to fail
		attemptBody := `{"dayOfWeek":1,"startTime":"08:00:00","endTime":"10:00:00","audience":"student"}`
		req := httptest.NewRequest("PUT", putURL, strings.NewReader(attemptBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))

		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusInternalServerError {
			t.Errorf("expected 500 Internal Server Error on forced audit failure, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		var errResp facility.ErrorResponse
		_ = json.Unmarshal(body, &errResp)
		if errResp.Error != "internal server error" {
			t.Errorf("expected generic 'internal server error', got %q", errResp.Error)
		}

		// Verify schedule 1 row was NOT modified (atomicity rollback!)
		var currDay int
		var currEnd string
		_ = pool.QueryRow(ctx, "SELECT day_of_week, to_char(end_time, 'HH24:MI:SS') FROM giki_sports.schedules WHERE id = $1;", sched1ID).Scan(&currDay, &currEnd)
		if currDay != 5 || currEnd != "24:00:00" {
			t.Errorf("CRITICAL: schedule row was modified despite audit rollback! day=%d, end=%s", currDay, currEnd)
		}

		// Remove test trigger
		_, _ = pool.Exec(ctx, "DROP TRIGGER IF EXISTS trg_fail_audit ON giki_sports.audit_logs;")
		_, _ = pool.Exec(ctx, "DROP FUNCTION IF EXISTS fail_audit();")
	}
}

// TestFacilityScheduleDeleteHTTP_Integration exercises the complete protected DELETE endpoint
// DELETE /api/facilities/:slug/schedules/:id against an isolated disposable database:
// - Anonymous requests rejected (401).
// - Student role rejected (403).
// - Untrusted / missing Origin rejected (403).
// - Wrong facility slug returns 404 and changes nothing.
// - Forced audit failure returns generic 500, rolls back deletion, and window remains visible in public GET.
// - Sports director deletion succeeds: returns empty HTTP 204 No Content, window disappears from public GET,
//   and exactly 1 audit row is written with director user ID and deleted window values.
// - Repeat DELETE returns 404 and does not add another audit row.
// - Super Admin deletion succeeds: returns empty HTTP 204 No Content, window disappears, second audit row written.
func TestFacilityScheduleDeleteHTTP_Integration(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	ctx := context.Background()

	// 1. Construct real repositories, services, and handlers
	facilityRepo := facility.NewRepository(pool)
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
	facilityHandler := facility.NewHandler(facilityRepo, 5*time.Second)

	app := fiber.New()
	api := app.Group("/api")

	authHandler.RegisterRoutes(api)
	facilityHandler.RegisterRoutes(api)
	facilityHandler.RegisterScheduleUpdateRoute(api,
		authHandler.RequireTrustedOrigin(),
		authHandler.RequireAuth(),
		authentication.RequireAnyRole(user.RoleSportsDirector, user.RoleSuperAdmin),
	)
	facilityHandler.RegisterScheduleDeleteRoute(api,
		authHandler.RequireTrustedOrigin(),
		authHandler.RequireAuth(),
		authentication.RequireAnyRole(user.RoleSportsDirector, user.RoleSuperAdmin),
	)

	// 2. Hash shared password once for synthetic test users
	passHash, err := auth.Hash("SecurePass123!")
	if err != nil {
		t.Fatalf("failed to hash test password: %v", err)
	}

	// 3. Seed test users
	directorID := uuid.New()
	superAdminID := uuid.New()
	studentID := uuid.New()

	users := []struct {
		id    uuid.UUID
		email string
		name  string
		role  user.Role
	}{
		{id: directorID, email: "director@giki.test", name: "Sports Director", role: user.RoleSportsDirector},
		{id: superAdminID, email: "superadmin@giki.test", name: "Super Admin", role: user.RoleSuperAdmin},
		{id: studentID, email: "student@giki.test", name: "Student User", role: user.RoleStudent},
	}

	for _, u := range users {
		_, err := pool.Exec(ctx, `
			INSERT INTO giki_sports.users (id, email, display_name, password_hash)
			VALUES ($1, $2, $3, $4);
		`, u.id, u.email, u.name, passHash)
		if err != nil {
			t.Fatalf("failed to seed user %s: %v", u.email, err)
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.user_roles (user_id, role)
			VALUES ($1, $2);
		`, u.id, u.role)
		if err != nil {
			t.Fatalf("failed to seed role for %s: %v", u.email, err)
		}
	}

	// Login helper
	loginUser := func(email string) string {
		payload := fmt.Sprintf(`{"email":%q,"password":"SecurePass123!"}`, email)
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")

		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("login request failed for %s: %v", email, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("login failed for %s (status %d): %s", email, resp.StatusCode, string(b))
		}

		for _, c := range resp.Cookies() {
			if c.Name == "giki_session" {
				return c.Value
			}
		}
		t.Fatalf("session cookie not found for %s", email)
		return ""
	}

	directorCookie := loginUser("director@giki.test")
	superAdminCookie := loginUser("superadmin@giki.test")
	studentCookie := loginUser("student@giki.test")

	// 4. Seed test facility and 3 schedule windows
	var facID, sched1ID, sched2ID, sched3ID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, description, location, status)
		VALUES ('del-arena', 'Delete Arena Hall', 'Facility for delete verification', 'East Wing', 'active')
		RETURNING id;
	`).Scan(&facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 1, '09:00:00', '12:00:00', 'student')
		RETURNING id;
	`, facID).Scan(&sched1ID)
	if err != nil {
		t.Fatalf("failed to seed schedule 1: %v", err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 3, '14:00:00', '16:00:00', 'faculty')
		RETURNING id;
	`, facID).Scan(&sched2ID)
	if err != nil {
		t.Fatalf("failed to seed schedule 2: %v", err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 5, '18:00:00', '20:00:00', 'general')
		RETURNING id;
	`, facID).Scan(&sched3ID)
	if err != nil {
		t.Fatalf("failed to seed schedule 3: %v", err)
	}

	delSched1URL := fmt.Sprintf("/api/facilities/del-arena/schedules/%s", sched1ID)
	delSched2URL := fmt.Sprintf("/api/facilities/del-arena/schedules/%s", sched2ID)

	// -------------------------------------------------------------
	// Verification 1: Anonymous request (missing cookie) rejected (401)
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("DELETE", delSched1URL, nil)
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("expected 401 for anonymous DELETE, got %d", resp.StatusCode)
		}
	}

	// -------------------------------------------------------------
	// Verification 2: Student role rejected (403)
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("DELETE", delSched1URL, nil)
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", studentCookie))
		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for student DELETE, got %d", resp.StatusCode)
		}
	}

	// -------------------------------------------------------------
	// Verification 3: Untrusted / missing Origin rejected (403)
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("DELETE", delSched1URL, nil)
		req.Header.Set("Origin", "http://evil-origin.test")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))
		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			t.Fatalf("expected 403 for untrusted Origin, got %d", resp.StatusCode)
		}
	}

	// -------------------------------------------------------------
	// Verification 4: Wrong facility slug returns 404 and changes nothing
	// -------------------------------------------------------------
	{
		wrongSlugURL := fmt.Sprintf("/api/facilities/wrong-facility-slug/schedules/%s", sched1ID)
		req := httptest.NewRequest("DELETE", wrongSlugURL, nil)
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))
		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Fatalf("expected 404 for wrong facility slug, got %d", resp.StatusCode)
		}

		// Verify schedule 1 still exists
		var s1Count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.schedules WHERE id = $1;", sched1ID).Scan(&s1Count)
		if s1Count != 1 {
			t.Errorf("schedule was deleted despite mismatched slug!")
		}

		// Verify zero audit logs
		var auditCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
		if auditCount != 0 {
			t.Errorf("audit log created on 404: %d", auditCount)
		}
	}

	// -------------------------------------------------------------
	// Verification 5: Forced audit failure returns generic 500, rolls back, and leaves window visible
	// -------------------------------------------------------------
	{
		// Install trigger on audit_logs that raises an exception on insert
		_, err := pool.Exec(ctx, `
			CREATE OR REPLACE FUNCTION fail_delete_audit() RETURNS trigger AS $$
			BEGIN
				RAISE EXCEPTION 'forced audit table failure for delete atomicity test';
			END;
			$$ LANGUAGE plpgsql;
		`)
		if err != nil {
			t.Fatalf("failed to create fail_delete_audit function: %v", err)
		}

		_, err = pool.Exec(ctx, `
			CREATE TRIGGER trg_fail_delete_audit BEFORE INSERT ON giki_sports.audit_logs
				FOR EACH ROW EXECUTE FUNCTION fail_delete_audit();
		`)
		if err != nil {
			t.Fatalf("failed to install test trigger: %v", err)
		}

		req := httptest.NewRequest("DELETE", delSched1URL, nil)
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))

		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusInternalServerError {
			t.Fatalf("expected 500 for forced audit failure, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		var errResp facility.ErrorResponse
		_ = json.Unmarshal(body, &errResp)
		if errResp.Error != "internal server error" {
			t.Errorf("expected generic 'internal server error', got %q", errResp.Error)
		}

		// Remove trigger
		_, _ = pool.Exec(ctx, "DROP TRIGGER IF EXISTS trg_fail_delete_audit ON giki_sports.audit_logs;")
		_, _ = pool.Exec(ctx, "DROP FUNCTION IF EXISTS fail_delete_audit();")

		// Verify schedule 1 row still exists in database (rollback!)
		var s1Count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.schedules WHERE id = $1;", sched1ID).Scan(&s1Count)
		if s1Count != 1 {
			t.Errorf("CRITICAL: schedule row was deleted despite audit failure rollback!")
		}

		// Verify schedule 1 is STILL VISIBLE in public GET /api/facilities/del-arena
		getReq := httptest.NewRequest("GET", "/api/facilities/del-arena", nil)
		getResp, _ := app.Test(getReq)
		defer getResp.Body.Close()
		var getFacility facility.FacilityResponse
		getBody, _ := io.ReadAll(getResp.Body)
		_ = json.Unmarshal(getBody, &getFacility)

		foundSched1 := false
		for _, s := range getFacility.Schedules {
			if s.ID == sched1ID.String() {
				foundSched1 = true
				break
			}
		}
		if !foundSched1 {
			t.Errorf("schedule 1 missing from public GET after rollback!")
		}
	}

	// -------------------------------------------------------------
	// Verification 6: Sports Director deletion succeeds (204, empty body, window gone in public GET, audit logged)
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("DELETE", delSched1URL, nil)
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))

		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("delete request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusNoContent {
			t.Fatalf("expected 204 No Content for director DELETE, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}
		if len(body) != 0 {
			t.Errorf("expected empty body for 204 No Content, got: %s", string(body))
		}

		if cacheCtrl := resp.Header.Get("Cache-Control"); !strings.Contains(cacheCtrl, "no-store") {
			t.Errorf("expected Cache-Control: no-store, got %q", cacheCtrl)
		}

		// Verify schedule 1 is GONE from public GET /api/facilities/del-arena
		getReq := httptest.NewRequest("GET", "/api/facilities/del-arena", nil)
		getResp, _ := app.Test(getReq)
		defer getResp.Body.Close()
		var getFacility facility.FacilityResponse
		getBody, _ := io.ReadAll(getResp.Body)
		_ = json.Unmarshal(getBody, &getFacility)

		if len(getFacility.Schedules) != 2 {
			t.Fatalf("expected 2 remaining schedules in public GET, found %d", len(getFacility.Schedules))
		}
		for _, s := range getFacility.Schedules {
			if s.ID == sched1ID.String() {
				t.Fatalf("schedule 1 is still present in public GET after successful deletion!")
			}
		}

		// Verify exactly 1 audit row in giki_sports.audit_logs
		var auditCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
		if auditCount != 1 {
			t.Fatalf("expected exactly 1 audit log, got %d", auditCount)
		}

		var auditActorID, auditTargetID uuid.UUID
		var auditAction, auditTargetType, auditStatus string
		var auditDetailsBytes []byte
		err = pool.QueryRow(ctx, `
			SELECT actor_id, action, target_type, target_id, status, details
			FROM giki_sports.audit_logs
			WHERE target_id = $1;
		`, sched1ID).Scan(
			&auditActorID,
			&auditAction,
			&auditTargetType,
			&auditTargetID,
			&auditStatus,
			&auditDetailsBytes,
		)
		if err != nil {
			t.Fatalf("failed to query audit log for deleted schedule 1: %v", err)
		}

		if auditActorID != directorID {
			t.Errorf("expected audit actor_id %s (director), got %s", directorID, auditActorID)
		}
		if auditAction != "schedule.delete" {
			t.Errorf("expected action 'schedule.delete', got %q", auditAction)
		}
		if auditTargetType != "facility_schedule" {
			t.Errorf("expected target_type 'facility_schedule', got %q", auditTargetType)
		}
		if auditTargetID != sched1ID {
			t.Errorf("expected target_id %s, got %s", sched1ID, auditTargetID)
		}
		if auditStatus != "SUCCESS" {
			t.Errorf("expected status 'SUCCESS', got %q", auditStatus)
		}

		var auditDetails map[string]interface{}
		_ = json.Unmarshal(auditDetailsBytes, &auditDetails)
		if auditDetails["facility_slug"] != "del-arena" {
			t.Errorf("expected facility_slug 'del-arena', got %v", auditDetails["facility_slug"])
		}

		oldWin, ok := auditDetails["old_window"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected old_window in audit details, got %T", auditDetails["old_window"])
		}
		if int(oldWin["day_of_week"].(float64)) != 1 ||
			oldWin["start_time"] != "09:00:00" ||
			oldWin["end_time"] != "12:00:00" ||
			oldWin["audience"] != "student" {
			t.Errorf("audit old_window parameters incorrect: %+v", oldWin)
		}
	}

	// -------------------------------------------------------------
	// Verification 7: Repeat DELETE returns 404 and does not add duplicate audit row
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("DELETE", delSched1URL, nil)
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))

		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("repeat delete request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusNotFound {
			t.Fatalf("expected 404 Not Found on repeat DELETE, got %d", resp.StatusCode)
		}

		// Verify audit log count remains 1
		var auditCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
		if auditCount != 1 {
			t.Fatalf("expected audit log count to remain 1 after repeated delete, got %d", auditCount)
		}
	}

	// -------------------------------------------------------------
	// Verification 8: Super Admin deletion succeeds (204, empty body, window disappears)
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("DELETE", delSched2URL, nil)
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", superAdminCookie))

		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("super admin delete request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusNoContent {
			t.Fatalf("expected 204 No Content for super admin DELETE, got %d", resp.StatusCode)
		}

		// Verify public GET now only has 1 schedule (sched3ID)
		getReq := httptest.NewRequest("GET", "/api/facilities/del-arena", nil)
		getResp, _ := app.Test(getReq)
		defer getResp.Body.Close()
		var getFacility facility.FacilityResponse
		getBody, _ := io.ReadAll(getResp.Body)
		_ = json.Unmarshal(getBody, &getFacility)

		if len(getFacility.Schedules) != 1 {
			t.Fatalf("expected exactly 1 remaining schedule in public GET, found %d", len(getFacility.Schedules))
		}
		if getFacility.Schedules[0].ID != sched3ID.String() {
			t.Errorf("expected remaining schedule to be %s, got %s", sched3ID, getFacility.Schedules[0].ID)
		}

		// Verify exactly 2 audit logs now exist
		var auditCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
		if auditCount != 2 {
			t.Fatalf("expected 2 audit logs after super admin delete, got %d", auditCount)
		}

		var latestActorID string
		_ = pool.QueryRow(ctx, "SELECT actor_id FROM giki_sports.audit_logs ORDER BY created_at DESC LIMIT 1;").Scan(&latestActorID)
		if latestActorID != superAdminID.String() {
			t.Errorf("expected latest audit actor_id %s (super admin), got %s", superAdminID, latestActorID)
		}
	}
}

// TestFacilityScheduleCreateHTTP_Integration verifies end-to-end HTTP creation of weekly schedule windows (Step 9B):
// - Anonymous requests rejected with 401 Unauthorized.
// - Untrusted / missing Origin rejected with 403 Forbidden.
// - Student role rejected with 403 Forbidden.
// - Sports director creation succeeds: returns HTTP 201 Created with ScheduleResponse shape,
//   signed-out public GET shows the new window, and 1 matching schedule.create audit row is written.
// - Super admin creation succeeds: returns HTTP 201 Created, second audit row written with super admin user ID.
// - Duplicate window returns 409 Conflict and adds NO audit row.
// - Forced audit failure returns generic 500, rolls back schedule insert, leaving NO new schedule in the database.
func TestFacilityScheduleCreateHTTP_Integration(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	ctx := context.Background()

	// 1. Construct real repositories, services, and handlers
	facilityRepo := facility.NewRepository(pool)
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
	facilityHandler := facility.NewHandler(facilityRepo, 5*time.Second)

	app := fiber.New()
	api := app.Group("/api")

	authHandler.RegisterRoutes(api)
	facilityHandler.RegisterRoutes(api)
	facilityHandler.RegisterScheduleUpdateRoute(api,
		authHandler.RequireTrustedOrigin(),
		authHandler.RequireAuth(),
		authentication.RequireAnyRole(user.RoleSportsDirector, user.RoleSuperAdmin),
	)
	facilityHandler.RegisterScheduleDeleteRoute(api,
		authHandler.RequireTrustedOrigin(),
		authHandler.RequireAuth(),
		authentication.RequireAnyRole(user.RoleSportsDirector, user.RoleSuperAdmin),
	)
	facilityHandler.RegisterScheduleCreateRoute(api,
		authHandler.RequireTrustedOrigin(),
		authHandler.RequireAuth(),
		authentication.RequireAnyRole(user.RoleSportsDirector, user.RoleSuperAdmin),
	)

	// 2. Hash shared password once for test users
	passHash, err := auth.Hash("SecurePass123!")
	if err != nil {
		t.Fatalf("failed to hash test password: %v", err)
	}

	// 3. Seed test users
	directorID := uuid.New()
	superAdminID := uuid.New()
	studentID := uuid.New()

	users := []struct {
		id    uuid.UUID
		email string
		name  string
		role  user.Role
	}{
		{id: directorID, email: "director@giki.test", name: "Sports Director", role: user.RoleSportsDirector},
		{id: superAdminID, email: "superadmin@giki.test", name: "Super Admin", role: user.RoleSuperAdmin},
		{id: studentID, email: "student@giki.test", name: "Student User", role: user.RoleStudent},
	}

	for _, u := range users {
		_, err := pool.Exec(ctx, `
			INSERT INTO giki_sports.users (id, email, display_name, password_hash)
			VALUES ($1, $2, $3, $4);
		`, u.id, u.email, u.name, passHash)
		if err != nil {
			t.Fatalf("failed to seed user %s: %v", u.email, err)
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.user_roles (user_id, role)
			VALUES ($1, $2);
		`, u.id, u.role)
		if err != nil {
			t.Fatalf("failed to seed role for %s: %v", u.email, err)
		}
	}

	// Login helper
	loginUser := func(email string) string {
		loginBody := fmt.Sprintf(`{"email":"%s","password":"SecurePass123!"}`, email)
		loginReq := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(loginBody))
		loginReq.Header.Set("Content-Type", "application/json")
		loginReq.Header.Set("Origin", "http://localhost:5173")

		resp, err := app.Test(loginReq, 10000)
		if err != nil {
			t.Fatalf("login failed for %s: %v", email, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("expected 200 for login %s, got %d", email, resp.StatusCode)
		}

		cookieHeader := resp.Header.Get("Set-Cookie")
		if cookieHeader == "" {
			t.Fatalf("expected Set-Cookie header for %s", email)
		}
		parts := strings.Split(cookieHeader, ";")
		return parts[0]
	}

	directorCookie := loginUser("director@giki.test")
	superAdminCookie := loginUser("superadmin@giki.test")
	studentCookie := loginUser("student@giki.test")

	// 4. Seed test facility with an empty timetable
	var facID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, description, location, status)
		VALUES ('create-arena', 'Create Arena', 'Arena for schedule creation testing', 'West Wing', 'active')
		RETURNING id;
	`).Scan(&facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	validWindowBody := `{"dayOfWeek": 1, "startTime": "09:00:00", "endTime": "11:00:00", "audience": "student"}`

	// -------------------------------------------------------------
	// Verification 1: Anonymous request rejected (401 Unauthorized)
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("POST", "/api/facilities/create-arena/schedules", strings.NewReader(validWindowBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:5173")

		resp, _ := app.Test(req, 10000)
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("expected 401 for anonymous POST, got %d", resp.StatusCode)
		}
	}

	// -------------------------------------------------------------
	// Verification 2: Untrusted Origin rejected (403 Forbidden)
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("POST", "/api/facilities/create-arena/schedules", strings.NewReader(validWindowBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://untrusted-site.evil")
		req.Header.Set("Cookie", directorCookie)

		resp, _ := app.Test(req, 10000)
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusForbidden {
			t.Fatalf("expected 403 for untrusted Origin, got %d", resp.StatusCode)
		}
	}

	// -------------------------------------------------------------
	// Verification 3: Student role rejected (403 Forbidden)
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("POST", "/api/facilities/create-arena/schedules", strings.NewReader(validWindowBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:5173")
		req.Header.Set("Cookie", studentCookie)

		resp, _ := app.Test(req, 10000)
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusForbidden {
			t.Fatalf("expected 403 for student role, got %d", resp.StatusCode)
		}
	}

	var firstCreatedID string

	// -------------------------------------------------------------
	// Verification 4: Sports Director creates first window (201 Created)
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("POST", "/api/facilities/create-arena/schedules", strings.NewReader(validWindowBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:5173")
		req.Header.Set("Cookie", directorCookie)

		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("director create request failed: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("expected 201 Created, got %d, body: %s", resp.StatusCode, string(body))
		}

		var createdResp facility.ScheduleResponse
		if err := json.Unmarshal(body, &createdResp); err != nil {
			t.Fatalf("failed to parse created schedule response: %v", err)
		}

		if createdResp.ID == "" {
			t.Fatal("expected non-empty PostgreSQL-generated schedule ID")
		}
		firstCreatedID = createdResp.ID

		if createdResp.DayOfWeek != 1 || createdResp.StartTime != "09:00:00" || createdResp.EndTime != "11:00:00" || createdResp.Audience != "student" {
			t.Errorf("unexpected schedule response attributes: %+v", createdResp)
		}

		// Verify signed-out public GET shows the new window
		getReq := httptest.NewRequest("GET", "/api/facilities/create-arena", nil)
		getResp, _ := app.Test(getReq, 10000)
		defer getResp.Body.Close()

		var getFacility facility.FacilityResponse
		getBody, _ := io.ReadAll(getResp.Body)
		_ = json.Unmarshal(getBody, &getFacility)

		if len(getFacility.Schedules) != 1 {
			t.Fatalf("expected exactly 1 schedule in public GET, found %d", len(getFacility.Schedules))
		}
		if getFacility.Schedules[0].ID != firstCreatedID {
			t.Errorf("expected schedule ID %s, got %s", firstCreatedID, getFacility.Schedules[0].ID)
		}

		// Verify database audit log
		var auditCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
		if auditCount != 1 {
			t.Fatalf("expected exactly 1 audit log, found %d", auditCount)
		}

		var auditActorID, auditAction, auditTargetType, auditTargetID string
		var auditDetailsBytes []byte
		err = pool.QueryRow(ctx, `
			SELECT actor_id, action, target_type, target_id, details
			FROM giki_sports.audit_logs
			WHERE target_id = $1;
		`, firstCreatedID).Scan(&auditActorID, &auditAction, &auditTargetType, &auditTargetID, &auditDetailsBytes)
		if err != nil {
			t.Fatalf("failed to query audit log for schedule: %v", err)
		}

		if auditActorID != directorID.String() {
			t.Errorf("expected audit actor_id %s (director), got %s", directorID, auditActorID)
		}
		if auditAction != "schedule.create" {
			t.Errorf("expected action 'schedule.create', got %q", auditAction)
		}
		if auditTargetType != "facility_schedule" {
			t.Errorf("expected target_type 'facility_schedule', got %q", auditTargetType)
		}
		if auditTargetID != firstCreatedID {
			t.Errorf("expected target_id %s, got %s", firstCreatedID, auditTargetID)
		}

		var details map[string]interface{}
		_ = json.Unmarshal(auditDetailsBytes, &details)
		savedWindow := details["saved_window"].(map[string]interface{})
		if int(savedWindow["day_of_week"].(float64)) != 1 || savedWindow["start_time"] != "09:00:00" || savedWindow["end_time"] != "11:00:00" || savedWindow["audience"] != "student" {
			t.Errorf("unexpected audit saved_window details: %+v", savedWindow)
		}
	}

	// -------------------------------------------------------------
	// Verification 5: Super Admin creates second window (201 Created)
	// -------------------------------------------------------------
	{
		secondBody := `{"dayOfWeek": 3, "startTime": "14:00:00", "endTime": "16:00:00", "audience": "faculty"}`
		req := httptest.NewRequest("POST", "/api/facilities/create-arena/schedules", strings.NewReader(secondBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:5173")
		req.Header.Set("Cookie", superAdminCookie)

		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("super admin create request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("expected 201 Created for super admin, got %d", resp.StatusCode)
		}

		// Verify public GET now has 2 schedules
		getReq := httptest.NewRequest("GET", "/api/facilities/create-arena", nil)
		getResp, _ := app.Test(getReq, 10000)
		defer getResp.Body.Close()

		var getFacility facility.FacilityResponse
		getBody, _ := io.ReadAll(getResp.Body)
		_ = json.Unmarshal(getBody, &getFacility)

		if len(getFacility.Schedules) != 2 {
			t.Fatalf("expected exactly 2 schedules in public GET, found %d", len(getFacility.Schedules))
		}

		// Verify database has exactly 2 audit logs
		var auditCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
		if auditCount != 2 {
			t.Fatalf("expected 2 audit logs after super admin create, got %d", auditCount)
		}

		var latestActorID string
		_ = pool.QueryRow(ctx, "SELECT actor_id FROM giki_sports.audit_logs ORDER BY created_at DESC LIMIT 1;").Scan(&latestActorID)
		if latestActorID != superAdminID.String() {
			t.Errorf("expected latest audit actor_id %s (super admin), got %s", superAdminID, latestActorID)
		}
	}

	// -------------------------------------------------------------
	// Verification 6: Duplicate window returns 409 Conflict and adds NO audit row
	// -------------------------------------------------------------
	{
		req := httptest.NewRequest("POST", "/api/facilities/create-arena/schedules", strings.NewReader(validWindowBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:5173")
		req.Header.Set("Cookie", directorCookie)

		resp, _ := app.Test(req, 10000)
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusConflict {
			t.Fatalf("expected 409 Conflict for duplicate window, got %d", resp.StatusCode)
		}

		// Verify audit log count remains at 2 (no audit row written for failed duplicate)
		var auditCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
		if auditCount != 2 {
			t.Fatalf("expected audit log count to remain 2, found %d", auditCount)
		}
	}

	// -------------------------------------------------------------
	// Verification 7: Forced audit failure returns generic 500 and leaves NO new schedule
	// -------------------------------------------------------------
	{
		// Install trigger on audit_logs that raises an exception on insert
		_, err := pool.Exec(ctx, `
			CREATE OR REPLACE FUNCTION fail_create_audit() RETURNS trigger AS $$
			BEGIN
				RAISE EXCEPTION 'forced audit table failure for create atomicity test';
			END;
			$$ LANGUAGE plpgsql;
		`)
		if err != nil {
			t.Fatalf("failed to create fail_create_audit function: %v", err)
		}

		_, err = pool.Exec(ctx, `
			CREATE TRIGGER trg_fail_create_audit BEFORE INSERT ON giki_sports.audit_logs
				FOR EACH ROW EXECUTE FUNCTION fail_create_audit();
		`)
		if err != nil {
			t.Fatalf("failed to create fail_create_audit trigger: %v", err)
		}

		thirdBody := `{"dayOfWeek": 5, "startTime": "10:00:00", "endTime": "12:00:00", "audience": "general"}`
		req := httptest.NewRequest("POST", "/api/facilities/create-arena/schedules", strings.NewReader(thirdBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:5173")
		req.Header.Set("Cookie", directorCookie)

		resp, _ := app.Test(req, 10000)
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusInternalServerError {
			t.Fatalf("expected 500 for forced audit failure, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		var errResp facility.ErrorResponse
		_ = json.Unmarshal(body, &errResp)
		if errResp.Error != "internal server error" {
			t.Errorf("expected generic 'internal server error', got %q", errResp.Error)
		}

		// Remove trigger
		_, _ = pool.Exec(ctx, `DROP TRIGGER trg_fail_create_audit ON giki_sports.audit_logs; DROP FUNCTION fail_create_audit;`)

		// Verify schedules count remains exactly 2 (atomicity rollback!)
		var schedCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.schedules WHERE facility_id = $1;", facID).Scan(&schedCount)
		if schedCount != 2 {
			t.Fatalf("CRITICAL ATOMICITY VIOLATION: expected 2 schedules in DB after rollback, found %d", schedCount)
		}

		// Verify public GET still returns exactly 2 schedules
		getReq := httptest.NewRequest("GET", "/api/facilities/create-arena", nil)
		getResp, _ := app.Test(getReq, 10000)
		defer getResp.Body.Close()

		var getFacility facility.FacilityResponse
		getBody, _ := io.ReadAll(getResp.Body)
		_ = json.Unmarshal(getBody, &getFacility)

		if len(getFacility.Schedules) != 2 {
			t.Fatalf("expected exactly 2 schedules after rollback, found %d", len(getFacility.Schedules))
		}
	}
}
