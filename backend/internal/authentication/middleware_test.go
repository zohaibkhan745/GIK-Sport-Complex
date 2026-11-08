package authentication

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"giki-sports-backend/internal/session"
	"giki-sports-backend/internal/user"
)

func TestMiddleware_RequireAuth(t *testing.T) {
	testUserID := uuid.New()
	testUser := &user.User{
		ID:          testUserID,
		Email:       "member@giki.edu.pk",
		DisplayName: "Active Member",
		Status:      user.StatusActive,
		Roles:       []user.Role{user.RoleStudent},
	}

	cookieCfg := defaultTestAuthConfig()

	t.Run("missing cookie returns 401 and blocks handler", func(t *testing.T) {
		handlerCalled := false
		fake := &fakeAuthenticator{}

		app := fiber.New()
		app.Get("/test/protected", RequireAuth(fake, NewCookieManager(cookieCfg), 2*time.Second), func(c *fiber.Ctx) error {
			handlerCalled = true
			return c.SendString("ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test/protected", nil)
		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
		if handlerCalled {
			t.Errorf("protected handler was executed despite missing cookie!")
		}
	})

	t.Run("invalid or expired session returns 401 and blocks handler", func(t *testing.T) {
		handlerCalled := false
		fake := &fakeAuthenticator{
			validateSessionFn: func(ctx context.Context, rawToken string) (*AuthenticatedPrincipal, error) {
				return nil, ErrInvalidSession
			},
		}

		app := fiber.New()
		app.Get("/test/protected", RequireAuth(fake, NewCookieManager(cookieCfg), 2*time.Second), func(c *fiber.Ctx) error {
			handlerCalled = true
			return c.SendString("ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test/protected", nil)
		req.Header.Set("Cookie", "giki_session=invalid-or-expired")
		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
		if handlerCalled {
			t.Errorf("protected handler was executed despite invalid session!")
		}
	})

	t.Run("deadline exceeded returns 503 and blocks handler", func(t *testing.T) {
		handlerCalled := false
		fake := &fakeAuthenticator{
			validateSessionFn: func(ctx context.Context, rawToken string) (*AuthenticatedPrincipal, error) {
				return nil, context.DeadlineExceeded
			},
		}

		app := fiber.New()
		app.Get("/test/protected", RequireAuth(fake, NewCookieManager(cookieCfg), 2*time.Second), func(c *fiber.Ctx) error {
			handlerCalled = true
			return c.SendString("ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test/protected", nil)
		req.Header.Set("Cookie", "giki_session=valid-looking-token")
		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("expected 503, got %d", resp.StatusCode)
		}
		if handlerCalled {
			t.Errorf("protected handler was executed despite timeout!")
		}
	})

	t.Run("unexpected infrastructure error returns 500 and redacts details", func(t *testing.T) {
		handlerCalled := false
		fake := &fakeAuthenticator{
			validateSessionFn: func(ctx context.Context, rawToken string) (*AuthenticatedPrincipal, error) {
				return nil, errors.New("sensitive postgresql deadlock or disk corruption")
			},
		}

		app := fiber.New()
		app.Get("/test/protected", RequireAuth(fake, NewCookieManager(cookieCfg), 2*time.Second), func(c *fiber.Ctx) error {
			handlerCalled = true
			return c.SendString("ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test/protected", nil)
		req.Header.Set("Cookie", "giki_session=valid-looking-token")
		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(body), "postgresql") || strings.Contains(string(body), "deadlock") {
			t.Errorf("sensitive error details leaked in response body: %s", string(body))
		}
		if handlerCalled {
			t.Errorf("protected handler was executed despite internal failure!")
		}
	})

	t.Run("valid session populates locals and executes handler with no-store", func(t *testing.T) {
		handlerCalled := false
		fake := &fakeAuthenticator{
			validateSessionFn: func(ctx context.Context, rawToken string) (*AuthenticatedPrincipal, error) {
				return &AuthenticatedPrincipal{
					User: testUser,
					Session: &session.Session{
						ID:                uuid.New(),
						UserID:            testUserID,
						AbsoluteExpiresAt: time.Now().Add(8 * time.Hour),
					},
				}, nil
			},
		}

		app := fiber.New()
		app.Get("/test/protected", RequireAuth(fake, NewCookieManager(cookieCfg), 2*time.Second), func(c *fiber.Ctx) error {
			handlerCalled = true
			principal, ok := GetPrincipal(c)
			if !ok || principal == nil {
				return c.Status(500).SendString("principal not found")
			}
			return c.JSON(ToUserResponse(principal.User))
		})

		req := httptest.NewRequest(http.MethodGet, "/test/protected", nil)
		req.Header.Set("Cookie", "giki_session=valid-token")
		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if !handlerCalled {
			t.Fatalf("protected handler was not executed!")
		}

		if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Errorf("expected Cache-Control: no-store, got %q", cc)
		}

		var u UserResponse
		body, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(body, &u)
		if u.Email != "member@giki.edu.pk" || len(u.Roles) != 1 || u.Roles[0] != "student" {
			t.Errorf("unexpected user DTO: %+v", u)
		}
	})

	t.Run("client-supplied spoofed headers cannot bypass authentication", func(t *testing.T) {
		handlerCalled := false
		fake := &fakeAuthenticator{} // will return error if called

		app := fiber.New()
		app.Get("/test/protected", RequireAuth(fake, NewCookieManager(cookieCfg), 2*time.Second), func(c *fiber.Ctx) error {
			handlerCalled = true
			return c.SendString("should not run")
		})

		req := httptest.NewRequest(http.MethodGet, "/test/protected", nil)
		req.Header.Set("X-User-ID", uuid.New().String())
		req.Header.Set("X-User-Role", "sports_director")
		req.Header.Set("X-Director-Key", "bypass-key")
		// No session cookie provided!

		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 despite spoofed headers, got %d", resp.StatusCode)
		}
		if handlerCalled {
			t.Errorf("protected handler ran because of client spoofed headers!")
		}
	})
}

func TestMiddleware_RequireAnyRole(t *testing.T) {
	studentUser := &user.User{
		ID:    uuid.New(),
		Email: "student@giki.edu.pk",
		Roles: []user.Role{user.RoleStudent},
	}
	directorUser := &user.User{
		ID:    uuid.New(),
		Email: "director@giki.edu.pk",
		Roles: []user.Role{user.RoleSportsDirector, user.RoleFaculty},
	}
	superAdminUser := &user.User{
		ID:    uuid.New(),
		Email: "admin@giki.edu.pk",
		Roles: []user.Role{user.RoleSuperAdmin},
	}
	noRoleUser := &user.User{
		ID:    uuid.New(),
		Email: "norole@giki.edu.pk",
		Roles: []user.Role{},
	}

	setupRoleApp := func(requiredRoles ...user.Role) (*fiber.App, *bool) {
		handlerCalled := false
		app := fiber.New()
		app.Get("/test/role-protected", RequireAnyRole(requiredRoles...), func(c *fiber.Ctx) error {
			handlerCalled = true
			return c.SendString("authorized")
		})
		return app, &handlerCalled
	}

	injectPrincipal := func(app *fiber.App, u *user.User) {
		app.Use(func(c *fiber.Ctx) error {
			if u != nil {
				c.Locals(principalLocalKey, &AuthenticatedPrincipal{
					User:    u,
					Session: &session.Session{ID: uuid.New(), UserID: u.ID},
				})
			}
			return c.Next()
		})
	}

	t.Run("missing principal returns 401", func(t *testing.T) {
		app, called := setupRoleApp(user.RoleStudent)
		req := httptest.NewRequest(http.MethodGet, "/test/role-protected", nil)
		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 when no principal, got %d", resp.StatusCode)
		}
		if *called {
			t.Errorf("protected handler was executed!")
		}
	})

	t.Run("single matching role succeeds with 200", func(t *testing.T) {
		app := fiber.New()
		injectPrincipal(app, studentUser)
		called := false
		app.Get("/test/role-protected", RequireAnyRole(user.RoleStudent), func(c *fiber.Ctx) error {
			called = true
			return c.SendString("authorized")
		})

		req := httptest.NewRequest(http.MethodGet, "/test/role-protected", nil)
		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		if !called {
			t.Errorf("handler was not called!")
		}
	})

	t.Run("one of multiple permitted roles succeeds with 200", func(t *testing.T) {
		app := fiber.New()
		injectPrincipal(app, directorUser)
		called := false
		app.Get("/test/role-protected", RequireAnyRole(user.RoleStudent, user.RoleSportsDirector), func(c *fiber.Ctx) error {
			called = true
			return c.SendString("authorized")
		})

		req := httptest.NewRequest(http.MethodGet, "/test/role-protected", nil)
		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		if !called {
			t.Errorf("handler was not called!")
		}
	})

	t.Run("wrong role returns 403 Forbidden", func(t *testing.T) {
		app := fiber.New()
		injectPrincipal(app, studentUser) // has student, route requires sports_director
		called := false
		app.Get("/test/role-protected", RequireAnyRole(user.RoleSportsDirector), func(c *fiber.Ctx) error {
			called = true
			return c.SendString("authorized")
		})

		req := httptest.NewRequest(http.MethodGet, "/test/role-protected", nil)
		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
		if called {
			t.Errorf("handler was called despite role mismatch!")
		}
	})

	t.Run("user with no roles returns 403 Forbidden", func(t *testing.T) {
		app := fiber.New()
		injectPrincipal(app, noRoleUser)
		called := false
		app.Get("/test/role-protected", RequireAnyRole(user.RoleStudent), func(c *fiber.Ctx) error {
			called = true
			return c.SendString("authorized")
		})

		req := httptest.NewRequest(http.MethodGet, "/test/role-protected", nil)
		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
		if called {
			t.Errorf("handler was called!")
		}
	})

	t.Run("empty role configuration fails closed with 403", func(t *testing.T) {
		app := fiber.New()
		injectPrincipal(app, directorUser)
		called := false
		app.Get("/test/role-protected", RequireAnyRole(), func(c *fiber.Ctx) error {
			called = true
			return c.SendString("authorized")
		})

		req := httptest.NewRequest(http.MethodGet, "/test/role-protected", nil)
		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 for empty role config, got %d", resp.StatusCode)
		}
		if called {
			t.Errorf("handler was called!")
		}
	})

	t.Run("unknown role configuration fails closed with 403", func(t *testing.T) {
		app := fiber.New()
		injectPrincipal(app, directorUser)
		called := false
		app.Get("/test/role-protected", RequireAnyRole(user.Role("unknown_invalid_role")), func(c *fiber.Ctx) error {
			called = true
			return c.SendString("authorized")
		})

		req := httptest.NewRequest(http.MethodGet, "/test/role-protected", nil)
		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 for unknown role config, got %d", resp.StatusCode)
		}
		if called {
			t.Errorf("handler was called!")
		}
	})

	t.Run("super_admin does not automatically bypass role checks", func(t *testing.T) {
		app := fiber.New()
		injectPrincipal(app, superAdminUser) // only has super_admin
		called := false
		// Route requires sports_director specifically, does not list super_admin
		app.Get("/test/role-protected", RequireAnyRole(user.RoleSportsDirector), func(c *fiber.Ctx) error {
			called = true
			return c.SendString("authorized")
		})

		req := httptest.NewRequest(http.MethodGet, "/test/role-protected", nil)
		resp, _ := app.Test(req, 2000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("CRITICAL SECURITY FLAW: super_admin bypassed role guard without being listed! got status %d", resp.StatusCode)
		}
		if called {
			t.Errorf("handler was called!")
		}
	})
}

func TestMiddleware_MeValidatesSessionOnlyOnce(t *testing.T) {
	testUser := &user.User{
		ID:          uuid.New(),
		Email:       "single.validate@giki.edu.pk",
		DisplayName: "Single Validate User",
		Status:      user.StatusActive,
		Roles:       []user.Role{user.RoleStudent},
	}

	fake := &fakeAuthenticator{
		validateSessionFn: func(ctx context.Context, rawToken string) (*AuthenticatedPrincipal, error) {
			return &AuthenticatedPrincipal{
				User:    testUser,
				Session: &session.Session{ID: uuid.New(), UserID: testUser.ID, AbsoluteExpiresAt: time.Now().Add(8 * time.Hour)},
			}, nil
		},
	}

	app, _ := setupTestApp(t, fake, defaultTestAuthConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.Header.Set("Cookie", "giki_session=sample-raw-token")

	resp, err := app.Test(req, 2000)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	if fake.validateCalls != 1 {
		t.Errorf("expected ValidateSession to be called exactly 1 time, got %d times", fake.validateCalls)
	}
}

func TestMiddleware_LogoutWorksWithoutRequireAuth(t *testing.T) {
	fake := &fakeAuthenticator{
		logoutFn: func(ctx context.Context, rawToken string) error {
			// Simulates logout revoking an expired/revoked session without requiring validation
			return nil
		},
	}

	app, _ := setupTestApp(t, fake, defaultTestAuthConfig())

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Cookie", "giki_session=already-expired-token")

	resp, err := app.Test(req, 2000)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for logout of expired session, got %d", resp.StatusCode)
	}

	// Verify cookie was cleared
	setCookie := strings.ToLower(resp.Header.Get("Set-Cookie"))
	if !strings.Contains(setCookie, "giki_session=") || (!strings.Contains(setCookie, "max-age=-1") && !strings.Contains(setCookie, "1970")) {
		t.Errorf("expected cookie deletion header, got: %s", setCookie)
	}

	// Ensure ValidateSession was NEVER called during logout
	if fake.validateCalls != 0 {
		t.Errorf("ValidateSession was called during logout: logout must remain independent of validation!")
	}
}

func TestMiddleware_CookieExpiry_And_SecurityModes(t *testing.T) {
	t.Run("BuildCookie sets MaxAge and Expires strictly bounded by AbsoluteExpiresAt", func(t *testing.T) {
		cfg := defaultTestAuthConfig()
		cm := NewCookieManager(cfg)

		now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
		absoluteExpiresAt := now.Add(4 * time.Hour) // 4 hours = 14400 seconds

		cookie, err := cm.BuildCookie("sample-raw-token", absoluteExpiresAt, now)
		if err != nil {
			t.Fatalf("unexpected error building cookie: %v", err)
		}

		if cookie.MaxAge != 14400 {
			t.Errorf("expected MaxAge 14400, got %d", cookie.MaxAge)
		}
		if !cookie.Expires.Equal(absoluteExpiresAt) {
			t.Errorf("expected Expires %v, got %v", absoluteExpiresAt, cookie.Expires)
		}
		if cookie.HTTPOnly != true {
			t.Errorf("expected HTTPOnly true, got false")
		}
		if cookie.SameSite != "Lax" {
			t.Errorf("expected SameSite Lax, got %s", cookie.SameSite)
		}
		if cookie.Path != "/api" {
			t.Errorf("expected Path /api, got %s", cookie.Path)
		}
		if cookie.Domain != "" {
			t.Errorf("expected Domain to be empty (host-only), got %q", cookie.Domain)
		}
	})

	t.Run("BuildCookie rejects already-expired session", func(t *testing.T) {
		cfg := defaultTestAuthConfig()
		cm := NewCookieManager(cfg)

		now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
		alreadyExpired := now.Add(-10 * time.Minute)

		cookie, err := cm.BuildCookie("sample-raw-token", alreadyExpired, now)
		if err == nil {
			t.Fatalf("expected error building cookie for already-expired session, got cookie: %+v", cookie)
		}
		if !strings.Contains(err.Error(), "already expired") {
			t.Errorf("expected 'already expired' error message, got: %v", err)
		}
	})

	t.Run("BuildCookie rejects session with zero remaining seconds", func(t *testing.T) {
		cfg := defaultTestAuthConfig()
		cm := NewCookieManager(cfg)

		now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
		subSecondRemaining := now.Add(500 * time.Millisecond) // rounds to 0 integer seconds

		_, err := cm.BuildCookie("sample-raw-token", subSecondRemaining, now)
		if err == nil {
			t.Fatalf("expected error for sub-second remaining session, got nil")
		}
	})

	t.Run("local HTTP configuration sets Secure=false", func(t *testing.T) {
		cfg := defaultTestAuthConfig()
		cfg.CookieSecure = false
		cm := NewCookieManager(cfg)

		now := time.Now()
		cookie, _ := cm.BuildCookie("token", now.Add(1*time.Hour), now)
		if cookie.Secure != false {
			t.Errorf("expected Secure=false in local HTTP dev mode, got true")
		}
	})

	t.Run("production HTTPS configuration sets Secure=true", func(t *testing.T) {
		cfg := defaultTestAuthConfig()
		cfg.CookieSecure = true
		cm := NewCookieManager(cfg)

		now := time.Now()
		cookie, _ := cm.BuildCookie("token", now.Add(1*time.Hour), now)
		if cookie.Secure != true {
			t.Errorf("expected Secure=true in production HTTPS mode, got false")
		}
	})
}
