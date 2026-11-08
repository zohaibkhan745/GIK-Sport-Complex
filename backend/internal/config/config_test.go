package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// cleanEnv clears all DB and PORT related environment variables for testing isolation.
func cleanEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"PORT", "DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD",
		"DB_SSL_MODE", "DB_MAX_CONNS", "DB_MIN_CONNS", "DB_MAX_CONN_LIFETIME",
		"DB_MAX_CONN_IDLE_TIME", "DB_CONNECT_TIMEOUT", "ENV_FILE",
		"AUTH_COOKIE_NAME", "AUTH_COOKIE_SECURE", "SESSION_COOKIE_SECURE", "AUTH_COOKIE_PATH",
		"AUTH_COOKIE_SAMESITE", "AUTH_TRUSTED_ORIGINS", "AUTH_LOGIN_RATE_LIMIT",
		"AUTH_LOGIN_RATE_WINDOW", "AUTH_LOGIN_MAX_CONCURRENT", "AUTH_REQUEST_TIMEOUT",
	}
	for _, k := range keys {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}

func TestConfig_ValidationFailures(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		errContains string
	}{
		{
			name: "missing DB_PASSWORD",
			env: map[string]string{
				"DB_NAME": "testdb",
				"DB_USER": "testuser",
			},
			errContains: "DB_PASSWORD is required",
		},
		{
			name: "missing DB_NAME",
			env: map[string]string{
				"DB_USER":     "testuser",
				"DB_PASSWORD": "secretpassword",
			},
			errContains: "DB_NAME is required",
		},
		{
			name: "missing DB_USER",
			env: map[string]string{
				"DB_NAME":     "testdb",
				"DB_PASSWORD": "secretpassword",
			},
			errContains: "DB_USER is required",
		},
		{
			name: "invalid non-integer DB_PORT",
			env: map[string]string{
				"DB_NAME":     "testdb",
				"DB_USER":     "testuser",
				"DB_PASSWORD": "secretpassword",
				"DB_PORT":     "not-a-port",
			},
			errContains: "invalid DB_PORT",
		},
		{
			name: "out of range DB_PORT",
			env: map[string]string{
				"DB_NAME":     "testdb",
				"DB_USER":     "testuser",
				"DB_PASSWORD": "secretpassword",
				"DB_PORT":     "70000",
			},
			errContains: "DB_PORT '70000' is out of range",
		},
		{
			name: "invalid SSLMode",
			env: map[string]string{
				"DB_NAME":     "testdb",
				"DB_USER":     "testuser",
				"DB_PASSWORD": "secretpassword",
				"DB_SSL_MODE": "invalid_mode",
			},
			errContains: "DB_SSL_MODE 'invalid_mode' is invalid",
		},
		{
			name: "invalid pool bounds (MaxConns < MinConns)",
			env: map[string]string{
				"DB_NAME":      "testdb",
				"DB_USER":      "testuser",
				"DB_PASSWORD":  "secretpassword",
				"DB_MAX_CONNS": "2",
				"DB_MIN_CONNS": "5",
			},
			errContains: "DB_MAX_CONNS (2) cannot be less than DB_MIN_CONNS (5)",
		},
		{
			name: "invalid DB_MAX_CONNS non-positive",
			env: map[string]string{
				"DB_NAME":      "testdb",
				"DB_USER":      "testuser",
				"DB_PASSWORD":  "secretpassword",
				"DB_MAX_CONNS": "0",
			},
			errContains: "DB_MAX_CONNS '0' must be greater than 0",
		},
		{
			name: "invalid DB_CONNECT_TIMEOUT duration",
			env: map[string]string{
				"DB_NAME":            "testdb",
				"DB_USER":            "testuser",
				"DB_PASSWORD":        "secretpassword",
				"DB_CONNECT_TIMEOUT": "invalid_duration",
			},
			errContains: "invalid DB_CONNECT_TIMEOUT",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			_, err := LoadFromEnv()
			if err == nil {
				t.Fatalf("expected error containing '%s', got nil", tc.errContains)
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Fatalf("expected error containing '%s', got '%s'", tc.errContains, err.Error())
			}
		})
	}
}

func TestConfig_ValidDefaults(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_NAME", "giki_sports_complex")
	t.Setenv("DB_USER", "giki_sports_app")
	t.Setenv("DB_PASSWORD", "super_secret_dev_pass")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("expected default Port '8080', got '%s'", cfg.Port)
	}
	if cfg.DB.Host != "127.0.0.1" {
		t.Errorf("expected default DB.Host '127.0.0.1', got '%s'", cfg.DB.Host)
	}
	if cfg.DB.Port != 5434 {
		t.Errorf("expected default DB.Port 5434, got %d", cfg.DB.Port)
	}
	if cfg.DB.SSLMode != "disable" {
		t.Errorf("expected default DB.SSLMode 'disable', got '%s'", cfg.DB.SSLMode)
	}
	if cfg.DB.MaxConns != 10 {
		t.Errorf("expected default DB.MaxConns 10, got %d", cfg.DB.MaxConns)
	}
	if cfg.DB.MinConns != 2 {
		t.Errorf("expected default DB.MinConns 2, got %d", cfg.DB.MinConns)
	}
	if cfg.DB.ConnectTimeout != 5*time.Second {
		t.Errorf("expected default DB.ConnectTimeout 5s, got %s", cfg.DB.ConnectTimeout)
	}
}

func TestConfig_PrecedenceAndRedaction(t *testing.T) {
	cleanEnv(t)

	// Create temporary isolated env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.test")
	fileContent := `
PORT=9090
DB_PORT=5439
DB_NAME=file_db
DB_USER=file_user
DB_PASSWORD=file_secret
`
	if err := os.WriteFile(envFile, []byte(fileContent), 0600); err != nil {
		t.Fatalf("failed to write temp env file: %v", err)
	}

	// Real environment variable that MUST take precedence
	t.Setenv("DB_PORT", "5434")
	t.Setenv("DB_USER", "env_override_user")

	cfg, err := LoadFromFile(envFile)
	if err != nil {
		t.Fatalf("failed to load from test file: %v", err)
	}

	// Verify precedence: env var 5434 must override file's 5439
	if cfg.DB.Port != 5434 {
		t.Errorf("expected real env var to take precedence (5434), got %d", cfg.DB.Port)
	}
	// Verify precedence: env var env_override_user must override file's file_user
	if cfg.DB.User != "env_override_user" {
		t.Errorf("expected real env var to take precedence ('env_override_user'), got '%s'", cfg.DB.User)
	}
	// Verify file value populated where env var was unset
	if cfg.DB.Name != "file_db" {
		t.Errorf("expected file value ('file_db'), got '%s'", cfg.DB.Name)
	}
	if cfg.Port != "9090" {
		t.Errorf("expected file value ('9090'), got '%s'", cfg.Port)
	}

	// Verify credentials are redacted in String() representation
	cfgString := cfg.DB.String()
	if strings.Contains(cfgString, "file_secret") {
		t.Fatalf("CRITICAL SECURITY FLAW: DatabaseConfig.String() leaked the raw password!")
	}
	if !strings.Contains(cfgString, "Password=[REDACTED]") {
		t.Errorf("expected DatabaseConfig.String() to contain 'Password=[REDACTED]', got '%s'", cfgString)
	}
}

func TestConfig_AuthSettings(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_NAME", "testdb")
	t.Setenv("DB_USER", "testuser")
	t.Setenv("DB_PASSWORD", "secret")

	// 1. Defaults verification
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected valid default config, got: %v", err)
	}
	if cfg.Auth.CookieName != "giki_session" {
		t.Errorf("expected CookieName 'giki_session', got %q", cfg.Auth.CookieName)
	}
	if cfg.Auth.CookieSecure != false {
		t.Errorf("expected default CookieSecure false for dev, got true")
	}
	if cfg.Auth.CookiePath != "/api" {
		t.Errorf("expected CookiePath '/api', got %q", cfg.Auth.CookiePath)
	}
	if cfg.Auth.CookieSameSite != "Lax" {
		t.Errorf("expected CookieSameSite 'Lax', got %q", cfg.Auth.CookieSameSite)
	}
	if len(cfg.Auth.TrustedOrigins) != 4 {
		t.Errorf("expected 4 default trusted origins, got %d", len(cfg.Auth.TrustedOrigins))
	}
	if cfg.Auth.LoginRateLimit != 10 {
		t.Errorf("expected LoginRateLimit 10, got %d", cfg.Auth.LoginRateLimit)
	}
	if cfg.Auth.LoginMaxConcurrent != 4 {
		t.Errorf("expected LoginMaxConcurrent 4, got %d", cfg.Auth.LoginMaxConcurrent)
	}
	if cfg.Auth.RequestTimeout != 5*time.Second {
		t.Errorf("expected RequestTimeout 5s, got %v", cfg.Auth.RequestTimeout)
	}

	// 2. Custom overrides verification
	t.Setenv("AUTH_COOKIE_NAME", "custom_session")
	t.Setenv("AUTH_COOKIE_SECURE", "true")
	t.Setenv("AUTH_COOKIE_PATH", "/api/custom")
	t.Setenv("AUTH_COOKIE_SAMESITE", "Strict")
	t.Setenv("AUTH_TRUSTED_ORIGINS", "https://app.giki.edu.pk, https://sports.giki.edu.pk")
	t.Setenv("AUTH_LOGIN_RATE_LIMIT", "20")
	t.Setenv("AUTH_LOGIN_RATE_WINDOW", "2m")
	t.Setenv("AUTH_LOGIN_MAX_CONCURRENT", "8")
	t.Setenv("AUTH_REQUEST_TIMEOUT", "10s")

	cfg2, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected valid overridden config, got: %v", err)
	}
	if cfg2.Auth.CookieName != "custom_session" {
		t.Errorf("expected 'custom_session', got %q", cfg2.Auth.CookieName)
	}
	if cfg2.Auth.CookieSecure != true {
		t.Errorf("expected CookieSecure true, got false")
	}
	if cfg2.Auth.CookiePath != "/api/custom" {
		t.Errorf("expected '/api/custom', got %q", cfg2.Auth.CookiePath)
	}
	if cfg2.Auth.CookieSameSite != "Strict" {
		t.Errorf("expected 'Strict', got %q", cfg2.Auth.CookieSameSite)
	}
	if len(cfg2.Auth.TrustedOrigins) != 2 || cfg2.Auth.TrustedOrigins[0] != "https://app.giki.edu.pk" {
		t.Errorf("expected parsed trusted origins, got %v", cfg2.Auth.TrustedOrigins)
	}
	if cfg2.Auth.LoginRateLimit != 20 {
		t.Errorf("expected 20, got %d", cfg2.Auth.LoginRateLimit)
	}
	if cfg2.Auth.LoginRateWindow != 2*time.Minute {
		t.Errorf("expected 2m, got %v", cfg2.Auth.LoginRateWindow)
	}
	if cfg2.Auth.LoginMaxConcurrent != 8 {
		t.Errorf("expected 8, got %d", cfg2.Auth.LoginMaxConcurrent)
	}
	if cfg2.Auth.RequestTimeout != 10*time.Second {
		t.Errorf("expected 10s, got %v", cfg2.Auth.RequestTimeout)
	}
}
