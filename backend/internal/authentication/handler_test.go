package authentication

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/session"
	"giki-sports-backend/internal/user"
)

// fakeAuthenticator provides a controllable mock of Authenticator for fast, deterministic unit tests.
type fakeAuthenticator struct {
	mu                sync.Mutex
	loginFn           func(ctx context.Context, email, password string) (*LoginResult, error)
	validateSessionFn func(ctx context.Context, rawToken string) (*AuthenticatedPrincipal, error)
	logoutFn          func(ctx context.Context, rawToken string) error

	loginCalls        int
	validateCalls     int
	logoutCalls       int
	lastPasswordInput string
}

func (f *fakeAuthenticator) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	f.mu.Lock()
	f.loginCalls++
	f.lastPasswordInput = password
	fn := f.loginFn
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, email, password)
	}
	return nil, errors.New("unexpected Login call")
}

func (f *fakeAuthenticator) ValidateSession(ctx context.Context, rawToken string) (*AuthenticatedPrincipal, error) {
	f.mu.Lock()
	f.validateCalls++
	fn := f.validateSessionFn
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, rawToken)
	}
	return nil, errors.New("unexpected ValidateSession call")
}

func (f *fakeAuthenticator) Logout(ctx context.Context, rawToken string) error {
	f.mu.Lock()
	f.logoutCalls++
	fn := f.logoutFn
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, rawToken)
	}
	return errors.New("unexpected Logout call")
}

func setupTestApp(t *testing.T, authService Authenticator, cfg config.AuthConfig) (*fiber.App, *Handler) {
	t.Helper()
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	handler := NewHandler(authService, cfg)
	api := app.Group("/api")
	handler.RegisterRoutes(api)
	return app, handler
}

func defaultTestAuthConfig() config.AuthConfig {
	return config.AuthConfig{
		CookieName:         "giki_session",
		CookieSecure:       false,
		CookiePath:         "/api",
		CookieSameSite:     "Lax",
		TrustedOrigins:     []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		LoginRateLimit:     100,
		LoginRateWindow:    1 * time.Minute,
		LoginMaxConcurrent: 4,
		RequestTimeout:     2 * time.Second,
	}
}

func TestHandler_Login_Success(t *testing.T) {
	userID := uuid.New()
	testUser := &user.User{
		ID:          userID,
		Email:       "student@giki.edu.pk",
		DisplayName: "Test Student",
		Status:      user.StatusActive,
		Roles:       []user.Role{user.RoleStudent},
	}
	sampleToken := "abcdef0123456789abcdef0123456789"

	fake := &fakeAuthenticator{
		loginFn: func(ctx context.Context, email, password string) (*LoginResult, error) {
			return &LoginResult{
				RawToken: sampleToken,
				User:     testUser,
				Session: &session.Session{
					ID:                uuid.New(),
					UserID:            userID,
					AbsoluteExpiresAt: time.Now().Add(session.AbsoluteLifetime),
				},
			}, nil
		},
	}

	app, _ := setupTestApp(t, fake, defaultTestAuthConfig())

	body := `{"email":"student@giki.edu.pk","password":"SecretPassword123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")

	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	// Verify Cache-Control: no-store
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("expected Cache-Control to contain 'no-store', got %q", cc)
	}

	// Verify Cookie Attributes
	setCookie := resp.Header.Get("Set-Cookie")
	if setCookie == "" {
		t.Fatalf("expected Set-Cookie header to be present")
	}
	cookieLower := strings.ToLower(setCookie)
	if !strings.Contains(setCookie, "giki_session="+sampleToken) {
		t.Errorf("expected cookie to contain raw token value, got: %s", setCookie)
	}
	if !strings.Contains(cookieLower, "path=/api") {
		t.Errorf("expected path=/api in cookie, got: %s", setCookie)
	}
	if !strings.Contains(cookieLower, "httponly") {
		t.Errorf("expected HttpOnly in cookie, got: %s", setCookie)
	}
	if !strings.Contains(cookieLower, "samesite=lax") {
		t.Errorf("expected SameSite=Lax in cookie, got: %s", setCookie)
	}
	if !strings.Contains(cookieLower, "max-age=28800") {
		t.Errorf("expected max-age=28800 (8h) in cookie, got: %s", setCookie)
	}

	// Verify public user DTO and secret isolation
	respBodyBytes, _ := io.ReadAll(resp.Body)
	respStr := string(respBodyBytes)

	if strings.Contains(respStr, sampleToken) {
		t.Fatalf("CRITICAL SECURITY FLAW: Raw session token leaked in JSON response body!")
	}

	var userDTO UserResponse
	if err := json.Unmarshal(respBodyBytes, &userDTO); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if userDTO.ID != userID.String() || userDTO.Email != "student@giki.edu.pk" || userDTO.DisplayName != "Test Student" {
		t.Errorf("unexpected user DTO content: %+v", userDTO)
	}
	if len(userDTO.Roles) != 1 || userDTO.Roles[0] != "student" {
		t.Errorf("expected roles ['student'], got: %v", userDTO.Roles)
	}
}

func TestHandler_Login_PreservesPasswordWhitespace(t *testing.T) {
	fake := &fakeAuthenticator{
		loginFn: func(ctx context.Context, email, password string) (*LoginResult, error) {
			return nil, ErrInvalidCredentials
		},
	}

	app, _ := setupTestApp(t, fake, defaultTestAuthConfig())

	rawPasswordWithSpaces := "  pass with leading and trailing spaces  "
	payload, _ := json.Marshal(LoginRequest{
		Email:    "test@giki.edu.pk",
		Password: rawPasswordWithSpaces,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")

	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if fake.lastPasswordInput != rawPasswordWithSpaces {
		t.Errorf("CRITICAL: Password whitespace was modified! expected %q, got %q",
			rawPasswordWithSpaces, fake.lastPasswordInput)
	}
}

func TestHandler_Login_OriginRejection(t *testing.T) {
	fake := &fakeAuthenticator{}
	app, _ := setupTestApp(t, fake, defaultTestAuthConfig())

	tests := []struct {
		name         string
		originHeader string
	}{
		{"missing origin", ""},
		{"null origin", "null"},
		{"untrusted external origin", "https://malicious-attacker.com"},
		{"untrusted subdomain trick", "http://localhost:5173.attacker.com"},
		{"malformed uri", "://invalid-uri"},
		{"path appended to origin", "http://localhost:5173/extra-path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake.loginCalls = 0
			body := `{"email":"student@giki.edu.pk","password":"SecretPassword123"}`
			req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tt.originHeader != "" {
				req.Header.Set("Origin", tt.originHeader)
			}

			resp, err := app.Test(req, 5000)
			if err != nil {
				t.Fatalf("unexpected app.Test error: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("expected 403 Forbidden for %q, got %d", tt.originHeader, resp.StatusCode)
			}

			// Ensure expensive password work was NOT executed
			if fake.loginCalls != 0 {
				t.Errorf("expensive login service was called despite untrusted origin!")
			}
		})
	}
}

func TestHandler_Login_ContentTypeAndBodyLimits(t *testing.T) {
	fake := &fakeAuthenticator{}
	app, _ := setupTestApp(t, fake, defaultTestAuthConfig())

	t.Run("unsupported media type text/plain", func(t *testing.T) {
		fake.loginCalls = 0
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("Origin", "http://localhost:5173")

		resp, _ := app.Test(req, 5000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnsupportedMediaType {
			t.Errorf("expected 415, got %d", resp.StatusCode)
		}
		if fake.loginCalls != 0 {
			t.Errorf("expensive login service was called!")
		}
	})

	t.Run("body exceeds size limit 4KB", func(t *testing.T) {
		fake.loginCalls = 0
		oversized := strings.Repeat("A", 4097)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(oversized))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:5173")

		resp, _ := app.Test(req, 5000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Errorf("expected 413, got %d", resp.StatusCode)
		}
		if fake.loginCalls != 0 {
			t.Errorf("expensive login service was called!")
		}
	})

	t.Run("malformed json payload", func(t *testing.T) {
		fake.loginCalls = 0
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{invalid-json}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:5173")

		resp, _ := app.Test(req, 5000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		if fake.loginCalls != 0 {
			t.Errorf("expensive login service was called!")
		}
	})
}

func TestHandler_Login_InvalidCredentials_SetsNoCookie(t *testing.T) {
	fake := &fakeAuthenticator{
		loginFn: func(ctx context.Context, email, password string) (*LoginResult, error) {
			return nil, ErrInvalidCredentials
		},
	}
	app, _ := setupTestApp(t, fake, defaultTestAuthConfig())

	body := `{"email":"student@giki.edu.pk","password":"WrongPassword"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")

	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
	}

	if cookie := resp.Header.Get("Set-Cookie"); cookie != "" {
		t.Errorf("cookie was set on failed credentials: %s", cookie)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	_ = json.Unmarshal(respBody, &errResp)
	if errResp.Error != "invalid email or password" {
		t.Errorf("expected generic error message, got %q", errResp.Error)
	}
}

func TestHandler_Login_InternalErrorRedaction(t *testing.T) {
	fake := &fakeAuthenticator{
		loginFn: func(ctx context.Context, email, password string) (*LoginResult, error) {
			return nil, errors.New("sensitive postgres connection pool leak / internal disk failure")
		},
	}
	app, _ := setupTestApp(t, fake, defaultTestAuthConfig())

	body := `{"email":"student@giki.edu.pk","password":"SomePassword"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")

	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(respBody), "postgres") || strings.Contains(string(respBody), "disk") {
		t.Fatalf("CRITICAL: Sensitive internal error was exposed to client: %s", string(respBody))
	}
}

func TestHandler_Login_ConcurrencySaturation(t *testing.T) {
	cfg := defaultTestAuthConfig()
	cfg.LoginMaxConcurrent = 1

	fake := &fakeAuthenticator{}
	app, handler := setupTestApp(t, fake, cfg)

	// Artificially saturate the semaphore
	handler.concurrencySem <- struct{}{}
	defer func() { <-handler.concurrencySem }()

	body := `{"email":"student@giki.edu.pk","password":"SomePassword"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")

	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 Service Unavailable when saturated, got %d", resp.StatusCode)
	}
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "1" {
		t.Errorf("expected Retry-After: 1 header, got %q", retryAfter)
	}
	if fake.loginCalls != 0 {
		t.Errorf("login service was called while concurrency semaphore was full!")
	}
}

func TestHandler_Login_RateLimiter(t *testing.T) {
	cfg := defaultTestAuthConfig()
	cfg.LoginRateLimit = 2 // allow 2 attempts per window

	fake := &fakeAuthenticator{
		loginFn: func(ctx context.Context, email, password string) (*LoginResult, error) {
			return nil, ErrInvalidCredentials
		},
	}
	app, _ := setupTestApp(t, fake, cfg)

	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"t@g.pk","password":"p"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:5173")
		resp, _ := app.Test(req, 5000)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("attempt %d: expected 401, got %d", i, resp.StatusCode)
		}
	}

	// 3rd attempt must trigger rate limiter
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"t@g.pk","password":"p"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")
	resp, _ := app.Test(req, 5000)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected 429 Too Many Requests on attempt 3, got %d", resp.StatusCode)
	}
}

func TestHandler_Me_SuccessAndValidation(t *testing.T) {
	userID := uuid.New()
	testUser := &user.User{
		ID:          userID,
		Email:       "director@giki.edu.pk",
		DisplayName: "Sports Director",
		Status:      user.StatusActive,
		Roles:       []user.Role{user.RoleSportsDirector, user.RoleFaculty},
	}

	fake := &fakeAuthenticator{
		validateSessionFn: func(ctx context.Context, rawToken string) (*AuthenticatedPrincipal, error) {
			if rawToken == "valid-session-token" {
				return &AuthenticatedPrincipal{
					User: testUser,
					Session: &session.Session{
						ID:     uuid.New(),
						UserID: userID,
					},
				}, nil
			}
			return nil, ErrInvalidSession
		},
	}

	app, _ := setupTestApp(t, fake, defaultTestAuthConfig())

	t.Run("valid session cookie returns public user DTO", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req.Header.Set("Cookie", "giki_session=valid-session-token")

		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("unexpected app.Test error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Errorf("expected Cache-Control: no-store, got %q", cc)
		}

		var u UserResponse
		body, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(body, &u)

		if u.Email != "director@giki.edu.pk" || len(u.Roles) != 2 {
			t.Errorf("unexpected user DTO: %+v", u)
		}
	})

	t.Run("missing session cookie returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)

		resp, _ := app.Test(req, 5000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid or expired session token returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req.Header.Set("Cookie", "giki_session=expired-or-revoked-token")

		resp, _ := app.Test(req, 5000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})
}

func TestHandler_Logout_SuccessAndClearsCookie(t *testing.T) {
	fake := &fakeAuthenticator{
		logoutFn: func(ctx context.Context, rawToken string) error {
			if rawToken == "valid-token-to-revoke" {
				return nil
			}
			return errors.New("db error")
		},
	}
	app, _ := setupTestApp(t, fake, defaultTestAuthConfig())

	t.Run("successful logout clears cookie with 204", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		req.Header.Set("Cookie", "giki_session=valid-token-to-revoke")

		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("unexpected app.Test error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected 204 No Content, got %d", resp.StatusCode)
		}

		setCookie := resp.Header.Get("Set-Cookie")
		cookieLower := strings.ToLower(setCookie)
		if !strings.Contains(setCookie, "giki_session=") {
			t.Errorf("expected giki_session in cleared cookie, got: %s", setCookie)
		}
		if !strings.Contains(cookieLower, "max-age=-1") && !strings.Contains(cookieLower, "1970") {
			t.Errorf("expected cookie deletion with max-age=-1 or 1970 expiry, got: %s", setCookie)
		}
		if !strings.Contains(cookieLower, "path=/api") {
			t.Errorf("expected path=/api on cleared cookie, got: %s", setCookie)
		}
	})

	t.Run("logout with untrusted origin rejected with 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
		req.Header.Set("Origin", "http://untrusted-site.com")
		req.Header.Set("Cookie", "giki_session=valid-token-to-revoke")

		resp, _ := app.Test(req, 5000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", resp.StatusCode)
		}
	})

	t.Run("logout with genuine database failure returns 500", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		req.Header.Set("Cookie", "giki_session=failing-token")

		resp, _ := app.Test(req, 5000)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected 500 for genuine database failure, got %d", resp.StatusCode)
		}
	})
}
