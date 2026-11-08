//go:build integration

package authentication_test

import (
	"bytes"
	"context"
	"encoding/json"
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
)

// TestAuthenticationMiddleware_Integration exercises RequireAuth and RequireAnyRole
// against a real PostgreSQL database with live migrations, sessions, and roles.
func TestAuthenticationMiddleware_Integration(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	ctx := context.Background()

	// 1. Safety Check: Record read-only baseline of development database
	initialUsers, initialSessions := countDevRecords(t)

	// 2. Insert synthetic Student fixture
	studentPassword := "test-student-pass"
	studentHash, err := auth.Hash(studentPassword)
	if err != nil {
		t.Fatalf("failed to compute hash for student: %v", err)
	}

	studentID := uuid.New()
	studentEmail := "student.mw@giki.edu.pk"
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, password_hash, display_name, status)
		VALUES ($1, $2, $3, 'Middleware Student', 'active');
	`, studentID, studentEmail, studentHash)
	if err != nil {
		t.Fatalf("failed to insert student user: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.user_roles (user_id, role) VALUES ($1, 'student');
	`, studentID)
	if err != nil {
		t.Fatalf("failed to insert student role: %v", err)
	}

	// 3. Insert synthetic Director fixture
	directorPassword := "test-director-pass"
	directorHash, err := auth.Hash(directorPassword)
	if err != nil {
		t.Fatalf("failed to compute hash for director: %v", err)
	}

	directorID := uuid.New()
	directorEmail := "director.mw@giki.edu.pk"
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, password_hash, display_name, status)
		VALUES ($1, $2, $3, 'Middleware Director', 'active');
	`, directorID, directorEmail, directorHash)
	if err != nil {
		t.Fatalf("failed to insert director user: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.user_roles (user_id, role) VALUES ($1, 'sports_director');
	`, directorID)
	if err != nil {
		t.Fatalf("failed to insert director role: %v", err)
	}

	// 4. Construct repositories, service, and HTTP handler
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
		TrustedOrigins:     []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		LoginRateLimit:     50,
		LoginRateWindow:    1 * time.Minute,
		LoginMaxConcurrent: 4,
		RequestTimeout:     5 * time.Second,
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	api := app.Group("/api")
	authHandler := authentication.NewHandler(authService, authCfg)
	authHandler.RegisterRoutes(api)

	// 5. Register TEST-ONLY protected endpoints (isolated inside integration test application)
	testGroup := app.Group("/test-protected", authHandler.RequireAuth())

	testGroup.Get("/student-only", authentication.RequireAnyRole(user.RoleStudent), func(c *fiber.Ctx) error {
		principal, _ := authentication.GetPrincipal(c)
		return c.JSON(fiber.Map{"status": "ok", "user": principal.User.Email})
	})

	testGroup.Get("/director-only", authentication.RequireAnyRole(user.RoleSportsDirector), func(c *fiber.Ctx) error {
		principal, _ := authentication.GetPrincipal(c)
		return c.JSON(fiber.Map{"status": "ok", "user": principal.User.Email})
	})

	testGroup.Get("/any-member", authentication.RequireAnyRole(user.RoleStudent, user.RoleSportsDirector), func(c *fiber.Ctx) error {
		principal, _ := authentication.GetPrincipal(c)
		return c.JSON(fiber.Map{"status": "ok", "user": principal.User.Email})
	})

	// Helper to perform login and extract cookie
	loginAndGetCookie := func(email, password string) string {
		loginBody, _ := json.Marshal(authentication.LoginRequest{
			Email:    email,
			Password: password,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:5173")

		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("login failed for %s: %v", email, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for login %s, got %d", email, resp.StatusCode)
		}

		setCookieHeader := resp.Header.Get("Set-Cookie")
		for _, part := range strings.Split(setCookieHeader, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "giki_session=") {
				return strings.TrimPrefix(part, "giki_session=")
			}
		}
		t.Fatalf("failed to find giki_session in Set-Cookie: %s", setCookieHeader)
		return ""
	}

	studentCookie := loginAndGetCookie(studentEmail, studentPassword)
	directorCookie := loginAndGetCookie(directorEmail, directorPassword)

	// =========================================================================
	// TEST CASE A: Student accesses student-only and any-member (200 OK)
	// =========================================================================
	reqStudentOK := httptest.NewRequest(http.MethodGet, "/test-protected/student-only", nil)
	reqStudentOK.Header.Set("Cookie", "giki_session="+studentCookie)
	respStudentOK, err := app.Test(reqStudentOK, 5000)
	if err != nil {
		t.Fatalf("req failed: %v", err)
	}
	defer respStudentOK.Body.Close()
	if respStudentOK.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for student accessing student-only, got %d", respStudentOK.StatusCode)
	}

	reqStudentMemberOK := httptest.NewRequest(http.MethodGet, "/test-protected/any-member", nil)
	reqStudentMemberOK.Header.Set("Cookie", "giki_session="+studentCookie)
	respStudentMemberOK, _ := app.Test(reqStudentMemberOK, 5000)
	defer respStudentMemberOK.Body.Close()
	if respStudentMemberOK.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for student accessing any-member, got %d", respStudentMemberOK.StatusCode)
	}

	// =========================================================================
	// TEST CASE B: Student denied director-only route (403 Forbidden)
	// =========================================================================
	reqStudentDenied := httptest.NewRequest(http.MethodGet, "/test-protected/director-only", nil)
	reqStudentDenied.Header.Set("Cookie", "giki_session="+studentCookie)
	respStudentDenied, _ := app.Test(reqStudentDenied, 5000)
	defer respStudentDenied.Body.Close()
	if respStudentDenied.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for student accessing director-only, got %d", respStudentDenied.StatusCode)
	}

	// =========================================================================
	// TEST CASE C: Director allowed on director-only, denied on student-only
	// =========================================================================
	reqDirectorOK := httptest.NewRequest(http.MethodGet, "/test-protected/director-only", nil)
	reqDirectorOK.Header.Set("Cookie", "giki_session="+directorCookie)
	respDirectorOK, _ := app.Test(reqDirectorOK, 5000)
	defer respDirectorOK.Body.Close()
	if respDirectorOK.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for director accessing director-only, got %d", respDirectorOK.StatusCode)
	}

	reqDirectorDenied := httptest.NewRequest(http.MethodGet, "/test-protected/student-only", nil)
	reqDirectorDenied.Header.Set("Cookie", "giki_session="+directorCookie)
	respDirectorDenied, _ := app.Test(reqDirectorDenied, 5000)
	defer respDirectorDenied.Body.Close()
	if respDirectorDenied.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for director accessing student-only, got %d", respDirectorDenied.StatusCode)
	}

	// =========================================================================
	// TEST CASE D: Dynamic role removal is reflected immediately on next request
	// =========================================================================
	// Delete student role from PostgreSQL database
	_, err = pool.Exec(ctx, `DELETE FROM giki_sports.user_roles WHERE user_id = $1;`, studentID)
	if err != nil {
		t.Fatalf("failed to delete user roles: %v", err)
	}

	// Next request with the SAME active session cookie must now be denied
	reqAfterRoleRemoval := httptest.NewRequest(http.MethodGet, "/test-protected/student-only", nil)
	reqAfterRoleRemoval.Header.Set("Cookie", "giki_session="+studentCookie)
	respAfterRoleRemoval, _ := app.Test(reqAfterRoleRemoval, 5000)
	defer respAfterRoleRemoval.Body.Close()

	if respAfterRoleRemoval.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden immediately after DB role removal, got %d", respAfterRoleRemoval.StatusCode)
	}

	// =========================================================================
	// TEST CASE E: Logged-out cookie is rejected with 401 Unauthorized
	// =========================================================================
	reqLogout := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	reqLogout.Header.Set("Origin", "http://localhost:5173")
	reqLogout.Header.Set("Cookie", "giki_session="+directorCookie)
	respLogout, _ := app.Test(reqLogout, 5000)
	defer respLogout.Body.Close()
	if respLogout.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 on logout, got %d", respLogout.StatusCode)
	}

	// Access with logged-out cookie must now fail with 401 Unauthorized
	reqDirectorAfterLogout := httptest.NewRequest(http.MethodGet, "/test-protected/director-only", nil)
	reqDirectorAfterLogout.Header.Set("Cookie", "giki_session="+directorCookie)
	respDirectorAfterLogout, _ := app.Test(reqDirectorAfterLogout, 5000)
	defer respDirectorAfterLogout.Body.Close()

	if respDirectorAfterLogout.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for logged-out cookie, got %d", respDirectorAfterLogout.StatusCode)
	}

	// 6. Safety Check: Verify development database remained completely unmutated
	assertDevDBUntouched(t, initialUsers, initialSessions)
}
