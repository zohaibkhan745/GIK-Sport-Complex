//go:build integration

package authentication_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"giki-sports-backend/internal/session"
	"giki-sports-backend/internal/user"
)

func assertDevDBUntouched(t *testing.T, baseUsers, baseSessions int) {
	t.Helper()
	uCount, sCount := countDevRecords(t)
	if uCount != baseUsers {
		t.Errorf("safety check violated: development users count changed: before=%d, after=%d", baseUsers, uCount)
	}
	if sCount != baseSessions {
		t.Errorf("safety check violated: development sessions count changed: before=%d, after=%d", baseSessions, sCount)
	}
}

// TestAuthenticationHTTP_Integration exercises the end-to-end authentication HTTP endpoints
// backed by real PostgreSQL repositories and Argon2id hashing within an isolated disposable database.
func TestAuthenticationHTTP_Integration(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	ctx := context.Background()

	// 1. Safety Check: Record read-only baseline of development database
	initialUsers, initialSessions := countDevRecords(t)

	// 2. Insert synthetic user fixture with real Argon2id password hash and roles
	userPassword := "IntegrationTestSecret123!"
	passwordHash, err := auth.Hash(userPassword)
	if err != nil {
		t.Fatalf("failed to compute Argon2id password hash for test user: %v", err)
	}

	testUserID := uuid.New()
	testEmail := "athlete.http@giki.edu.pk"
	testDisplayName := "HTTP Integration Athlete"

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, password_hash, display_name, status)
		VALUES ($1, $2, $3, $4, 'active');
	`, testUserID, testEmail, passwordHash, testDisplayName)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.user_roles (user_id, role)
		VALUES ($1, 'student'), ($1, 'faculty');
	`, testUserID)
	if err != nil {
		t.Fatalf("failed to insert test user roles: %v", err)
	}

	// 3. Construct real domain repositories and authentication service
	userRepo := user.NewRepository(pool)
	sessionRepo := session.NewRepository(pool)

	authService, err := authentication.NewService(userRepo, sessionRepo)
	if err != nil {
		t.Fatalf("failed to initialize authentication service: %v", err)
	}

	// 4. Construct HTTP Handler and Fiber Application
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

	// =========================================================================
	// STEP A: Failed Login with Wrong Password -> 401, No Cookie Issued
	// =========================================================================
	wrongLoginBody, _ := json.Marshal(authentication.LoginRequest{
		Email:    testEmail,
		Password: "IncorrectPassword!",
	})
	reqWrong := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(wrongLoginBody))
	reqWrong.Header.Set("Content-Type", "application/json")
	reqWrong.Header.Set("Origin", "http://localhost:5173")

	respWrong, err := app.Test(reqWrong, 10000)
	if err != nil {
		t.Fatalf("login attempt with wrong password failed: %v", err)
	}
	defer respWrong.Body.Close()

	if respWrong.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for wrong password, got %d", respWrong.StatusCode)
	}
	if cookie := respWrong.Header.Get("Set-Cookie"); cookie != "" {
		t.Fatalf("CRITICAL SECURITY FLAW: Set-Cookie was issued on failed login: %s", cookie)
	}

	// =========================================================================
	// STEP B: Successful Login -> 200, Set-Cookie Issued, Safe User JSON
	// =========================================================================
	loginBody, _ := json.Marshal(authentication.LoginRequest{
		Email:    testEmail,
		Password: userPassword,
	})
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	reqLogin.Header.Set("Origin", "http://localhost:5173")

	respLogin, err := app.Test(reqLogin, 10000)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer respLogin.Body.Close()

	if respLogin.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for valid credentials, got %d", respLogin.StatusCode)
	}

	// Capture and parse session cookie
	setCookieHeader := respLogin.Header.Get("Set-Cookie")
	if setCookieHeader == "" {
		t.Fatalf("expected Set-Cookie header on successful login")
	}

	var rawSessionToken string
	for _, part := range strings.Split(setCookieHeader, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "giki_session=") {
			rawSessionToken = strings.TrimPrefix(part, "giki_session=")
			break
		}
	}
	if rawSessionToken == "" {
		t.Fatalf("failed to extract raw session token from Set-Cookie header: %s", setCookieHeader)
	}

	// Verify user profile DTO in JSON response
	respLoginBytes, _ := io.ReadAll(respLogin.Body)
	var loginUserDTO authentication.UserResponse
	if err := json.Unmarshal(respLoginBytes, &loginUserDTO); err != nil {
		t.Fatalf("failed to decode login user DTO: %v", err)
	}
	if loginUserDTO.ID != testUserID.String() || loginUserDTO.Email != testEmail {
		t.Errorf("unexpected user DTO: %+v", loginUserDTO)
	}
	if len(loginUserDTO.Roles) != 2 {
		t.Errorf("expected 2 roles (student, faculty), got %v", loginUserDTO.Roles)
	}

	// CRITICAL: Verify DB stores SHA-256 hash of the token, and NEVER stores raw token
	expectedTokenHash, err := auth.HashSessionToken(rawSessionToken)
	if err != nil {
		t.Fatalf("failed to compute token hash: %v", err)
	}

	var storedHash string
	err = pool.QueryRow(ctx, `
		SELECT token_hash FROM giki_sports.sessions WHERE user_id = $1 AND revoked_at IS NULL;
	`, testUserID).Scan(&storedHash)
	if err != nil {
		t.Fatalf("failed to query session in database: %v", err)
	}
	if storedHash != expectedTokenHash {
		t.Errorf("expected database to store token hash %q, got %q", expectedTokenHash, storedHash)
	}
	if storedHash == rawSessionToken {
		t.Fatalf("CRITICAL SECURITY FLAW: database stored plaintext raw session token!")
	}

	// =========================================================================
	// STEP C: GET /api/auth/me with Valid Session Cookie -> 200, Dynamic Roles
	// =========================================================================
	reqMe := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	reqMe.Header.Set("Cookie", "giki_session="+rawSessionToken)

	respMe, err := app.Test(reqMe, 10000)
	if err != nil {
		t.Fatalf("GET /api/auth/me failed: %v", err)
	}
	defer respMe.Body.Close()

	if respMe.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/auth/me, got %d", respMe.StatusCode)
	}

	var meUserDTO authentication.UserResponse
	respMeBytes, _ := io.ReadAll(respMe.Body)
	if err := json.Unmarshal(respMeBytes, &meUserDTO); err != nil {
		t.Fatalf("failed to decode /me user DTO: %v", err)
	}
	if meUserDTO.ID != testUserID.String() || meUserDTO.Email != testEmail {
		t.Errorf("unexpected /me user DTO: %+v", meUserDTO)
	}

	// =========================================================================
	// STEP D: POST /api/auth/logout -> 204, Clears Cookie, Revokes Session in DB
	// =========================================================================
	reqLogout := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	reqLogout.Header.Set("Origin", "http://localhost:5173")
	reqLogout.Header.Set("Cookie", "giki_session="+rawSessionToken)

	respLogout, err := app.Test(reqLogout, 10000)
	if err != nil {
		t.Fatalf("POST /api/auth/logout failed: %v", err)
	}
	defer respLogout.Body.Close()

	if respLogout.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for logout, got %d", respLogout.StatusCode)
	}

	// Verify cookie clearing header
	logoutSetCookie := strings.ToLower(respLogout.Header.Get("Set-Cookie"))
	if !strings.Contains(logoutSetCookie, "giki_session=") {
		t.Errorf("expected giki_session in cleared cookie header: %s", logoutSetCookie)
	}
	if !strings.Contains(logoutSetCookie, "max-age=-1") && !strings.Contains(logoutSetCookie, "1970") {
		t.Errorf("expected cookie deletion with max-age=-1 or 1970 timestamp: %s", logoutSetCookie)
	}

	// Verify database record has revoked_at populated
	var revokedAt *time.Time
	err = pool.QueryRow(ctx, `
		SELECT revoked_at FROM giki_sports.sessions WHERE token_hash = $1;
	`, expectedTokenHash).Scan(&revokedAt)
	if err != nil {
		t.Fatalf("failed to query revoked session row: %v", err)
	}
	if revokedAt == nil {
		t.Fatalf("expected revoked_at to be non-nil in database after logout")
	}

	// =========================================================================
	// STEP E: Subsequent GET /api/auth/me with Revoked Cookie -> 401 Unauthorized
	// =========================================================================
	reqMeAfterLogout := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	reqMeAfterLogout.Header.Set("Cookie", "giki_session="+rawSessionToken)

	respMeAfterLogout, err := app.Test(reqMeAfterLogout, 10000)
	if err != nil {
		t.Fatalf("GET /api/auth/me after logout failed: %v", err)
	}
	defer respMeAfterLogout.Body.Close()

	if respMeAfterLogout.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for revoked cookie, got %d", respMeAfterLogout.StatusCode)
	}

	// 5. Verify development database remained completely unmutated
	assertDevDBUntouched(t, initialUsers, initialSessions)
}
