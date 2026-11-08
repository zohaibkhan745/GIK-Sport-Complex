package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config encapsulates runtime configuration for the sports complex application.
type Config struct {
	Port string
	DB   DatabaseConfig
	Auth AuthConfig
}

// AuthConfig encapsulates HTTP session cookie, CSRF origin verification, and abuse control parameters.
type AuthConfig struct {
	CookieName         string
	CookieSecure       bool
	CookiePath         string
	CookieSameSite     string
	TrustedOrigins     []string
	LoginRateLimit     int
	LoginRateWindow    time.Duration
	LoginMaxConcurrent int
	RequestTimeout     time.Duration
}

// DatabaseConfig holds connection settings and pool parameters for PostgreSQL.
type DatabaseConfig struct {
	Host            string
	Port            int
	Name            string
	User            string
	Password        string
	SSLMode         string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	ConnectTimeout  time.Duration
}

// String returns a safe representation of the DatabaseConfig with credentials redacted.
func (d DatabaseConfig) String() string {
	return fmt.Sprintf("Host=%s Port=%d Name=%s User=%s SSLMode=%s MaxConns=%d MinConns=%d MaxConnLifetime=%s MaxConnIdleTime=%s ConnectTimeout=%s Password=[REDACTED]",
		d.Host, d.Port, d.Name, d.User, d.SSLMode, d.MaxConns, d.MinConns, d.MaxConnLifetime, d.MaxConnIdleTime, d.ConnectTimeout)
}

// Load reads configuration using predictable local .env lookup, allowing environment variables to take precedence.
// It searches in order:
// 1. Explicit path in ENV_FILE environment variable, if set.
// 2. ".env" in the current working directory.
// 3. "../.env" (project root when running from backend/).
// It does not search arbitrary parent directories.
func Load() (*Config, error) {
	envPath := findPredictableEnvFile()
	if envPath != "" {
		if err := loadEnvFile(envPath); err != nil {
			return nil, fmt.Errorf("failed to load environment file %s: %w", envPath, err)
		}
	}
	return LoadFromEnv()
}

// LoadFromFile explicitly loads the specified file path and parses configuration.
func LoadFromFile(filePath string) (*Config, error) {
	if filePath != "" {
		if err := loadEnvFile(filePath); err != nil {
			return nil, fmt.Errorf("failed to load environment file %s: %w", filePath, err)
		}
	}
	return LoadFromEnv()
}

// LoadFromEnv parses and validates configuration strictly from active environment variables.
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		Port: getEnvOrDefault("PORT", "8080"),
		DB: DatabaseConfig{
			Host:            getEnvOrDefault("DB_HOST", "127.0.0.1"),
			Port:            5434, // Default for GIKI Sports Complex
			Name:            os.Getenv("DB_NAME"),
			User:            os.Getenv("DB_USER"),
			Password:        os.Getenv("DB_PASSWORD"),
			SSLMode:         getEnvOrDefault("DB_SSL_MODE", "disable"),
			MaxConns:        10,
			MinConns:        2,
			MaxConnLifetime: 1 * time.Hour,
			MaxConnIdleTime: 30 * time.Minute,
			ConnectTimeout:  5 * time.Second,
		},
		Auth: AuthConfig{
			CookieName:         getEnvOrDefault("AUTH_COOKIE_NAME", "giki_session"),
			CookieSecure:       parseBoolOrDefault(getEnvOrDefault("AUTH_COOKIE_SECURE", getEnvOrDefault("SESSION_COOKIE_SECURE", "false")), false),
			CookiePath:         getEnvOrDefault("AUTH_COOKIE_PATH", "/api"),
			CookieSameSite:     getEnvOrDefault("AUTH_COOKIE_SAMESITE", "Lax"),
			TrustedOrigins:     parseOriginsOrDefault(os.Getenv("AUTH_TRUSTED_ORIGINS"), []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:8081", "http://127.0.0.1:8081"}),
			LoginRateLimit:     10,
			LoginRateWindow:    1 * time.Minute,
			LoginMaxConcurrent: 4,
			RequestTimeout:     5 * time.Second,
		},
	}

	// Parse DB_PORT
	if portStr := os.Getenv("DB_PORT"); portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid DB_PORT '%s': must be a valid integer: %w", portStr, err)
		}
		cfg.DB.Port = p
	}

	// Parse MaxConns
	if maxConnsStr := os.Getenv("DB_MAX_CONNS"); maxConnsStr != "" {
		mc, err := strconv.ParseInt(maxConnsStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid DB_MAX_CONNS '%s': must be an integer: %w", maxConnsStr, err)
		}
		cfg.DB.MaxConns = int32(mc)
	}

	// Parse MinConns
	if minConnsStr := os.Getenv("DB_MIN_CONNS"); minConnsStr != "" {
		mc, err := strconv.ParseInt(minConnsStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid DB_MIN_CONNS '%s': must be an integer: %w", minConnsStr, err)
		}
		cfg.DB.MinConns = int32(mc)
	}

	// Parse MaxConnLifetime
	if lifetimeStr := os.Getenv("DB_MAX_CONN_LIFETIME"); lifetimeStr != "" {
		d, err := time.ParseDuration(lifetimeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid DB_MAX_CONN_LIFETIME '%s': must be a valid duration (e.g. 1h, 30m): %w", lifetimeStr, err)
		}
		cfg.DB.MaxConnLifetime = d
	}

	// Parse MaxConnIdleTime
	if idleStr := os.Getenv("DB_MAX_CONN_IDLE_TIME"); idleStr != "" {
		d, err := time.ParseDuration(idleStr)
		if err != nil {
			return nil, fmt.Errorf("invalid DB_MAX_CONN_IDLE_TIME '%s': must be a valid duration (e.g. 30m, 10m): %w", idleStr, err)
		}
		cfg.DB.MaxConnIdleTime = d
	}

	// Parse ConnectTimeout
	if timeoutStr := os.Getenv("DB_CONNECT_TIMEOUT"); timeoutStr != "" {
		d, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid DB_CONNECT_TIMEOUT '%s': must be a valid duration (e.g. 5s, 10s): %w", timeoutStr, err)
		}
		cfg.DB.ConnectTimeout = d
	}

	// Parse LoginRateLimit
	if rlStr := os.Getenv("AUTH_LOGIN_RATE_LIMIT"); rlStr != "" {
		rl, err := strconv.Atoi(rlStr)
		if err != nil {
			return nil, fmt.Errorf("invalid AUTH_LOGIN_RATE_LIMIT '%s': must be a valid integer: %w", rlStr, err)
		}
		cfg.Auth.LoginRateLimit = rl
	}

	// Parse LoginRateWindow
	if rwStr := os.Getenv("AUTH_LOGIN_RATE_WINDOW"); rwStr != "" {
		d, err := time.ParseDuration(rwStr)
		if err != nil {
			return nil, fmt.Errorf("invalid AUTH_LOGIN_RATE_WINDOW '%s': must be a valid duration: %w", rwStr, err)
		}
		cfg.Auth.LoginRateWindow = d
	}

	// Parse LoginMaxConcurrent
	if mcStr := os.Getenv("AUTH_LOGIN_MAX_CONCURRENT"); mcStr != "" {
		mc, err := strconv.Atoi(mcStr)
		if err != nil {
			return nil, fmt.Errorf("invalid AUTH_LOGIN_MAX_CONCURRENT '%s': must be a valid integer: %w", mcStr, err)
		}
		cfg.Auth.LoginMaxConcurrent = mc
	}

	// Parse RequestTimeout
	if rtStr := os.Getenv("AUTH_REQUEST_TIMEOUT"); rtStr != "" {
		d, err := time.ParseDuration(rtStr)
		if err != nil {
			return nil, fmt.Errorf("invalid AUTH_REQUEST_TIMEOUT '%s': must be a valid duration: %w", rtStr, err)
		}
		cfg.Auth.RequestTimeout = d
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks all mandatory configuration constraints.
func (c *Config) Validate() error {
	var errs []string

	// Validate HTTP Port
	if c.Port == "" {
		errs = append(errs, "HTTP port cannot be empty")
	} else {
		p, err := strconv.Atoi(c.Port)
		if err != nil || p < 1 || p > 65535 {
			errs = append(errs, fmt.Sprintf("HTTP port '%s' is invalid (must be 1-65535)", c.Port))
		}
	}

	// Validate Database Config
	if strings.TrimSpace(c.DB.Host) == "" {
		errs = append(errs, "DB_HOST is required")
	}
	if c.DB.Port < 1 || c.DB.Port > 65535 {
		errs = append(errs, fmt.Sprintf("DB_PORT '%d' is out of range (must be 1-65535)", c.DB.Port))
	}
	if strings.TrimSpace(c.DB.Name) == "" {
		errs = append(errs, "DB_NAME is required and cannot be empty")
	}
	if strings.TrimSpace(c.DB.User) == "" {
		errs = append(errs, "DB_USER is required and cannot be empty")
	}
	if c.DB.Password == "" {
		errs = append(errs, "DB_PASSWORD is required and cannot be empty")
	}

	// Validate SSL Mode
	validSSLModes := map[string]bool{
		"disable":     true,
		"require":     true,
		"verify-ca":   true,
		"verify-full": true,
	}
	if !validSSLModes[c.DB.SSLMode] {
		errs = append(errs, fmt.Sprintf("DB_SSL_MODE '%s' is invalid (allowed: disable, require, verify-ca, verify-full)", c.DB.SSLMode))
	}

	// Validate Pool Limits
	if c.DB.MaxConns <= 0 {
		errs = append(errs, fmt.Sprintf("DB_MAX_CONNS '%d' must be greater than 0", c.DB.MaxConns))
	}
	if c.DB.MinConns < 0 {
		errs = append(errs, fmt.Sprintf("DB_MIN_CONNS '%d' cannot be negative", c.DB.MinConns))
	}
	if c.DB.MaxConns < c.DB.MinConns {
		errs = append(errs, fmt.Sprintf("DB_MAX_CONNS (%d) cannot be less than DB_MIN_CONNS (%d)", c.DB.MaxConns, c.DB.MinConns))
	}

	// Validate Timeouts
	if c.DB.MaxConnLifetime <= 0 {
		errs = append(errs, "DB_MAX_CONN_LIFETIME must be greater than 0")
	}
	if c.DB.MaxConnIdleTime <= 0 {
		errs = append(errs, "DB_MAX_CONN_IDLE_TIME must be greater than 0")
	}
	if c.DB.ConnectTimeout <= 0 {
		errs = append(errs, "DB_CONNECT_TIMEOUT must be greater than 0")
	}

	// Validate Auth Config
	if strings.TrimSpace(c.Auth.CookieName) == "" {
		errs = append(errs, "AUTH_COOKIE_NAME cannot be empty")
	}
	if !strings.HasPrefix(c.Auth.CookiePath, "/") {
		errs = append(errs, "AUTH_COOKIE_PATH must start with '/'")
	}
	validSameSite := map[string]bool{"Lax": true, "Strict": true, "None": true}
	if !validSameSite[c.Auth.CookieSameSite] {
		errs = append(errs, fmt.Sprintf("AUTH_COOKIE_SAMESITE '%s' is invalid (allowed: Lax, Strict, None)", c.Auth.CookieSameSite))
	}
	if len(c.Auth.TrustedOrigins) == 0 {
		errs = append(errs, "at least one trusted origin is required")
	}
	if c.Auth.LoginRateLimit <= 0 {
		errs = append(errs, "AUTH_LOGIN_RATE_LIMIT must be greater than 0")
	}
	if c.Auth.LoginRateWindow <= 0 {
		errs = append(errs, "AUTH_LOGIN_RATE_WINDOW must be greater than 0")
	}
	if c.Auth.LoginMaxConcurrent <= 0 {
		errs = append(errs, "AUTH_LOGIN_MAX_CONCURRENT must be greater than 0")
	}
	if c.Auth.RequestTimeout <= 0 {
		errs = append(errs, "AUTH_REQUEST_TIMEOUT must be greater than 0")
	}

	if len(errs) > 0 {
		return errors.New("configuration validation failed: " + strings.Join(errs, "; "))
	}
	return nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func parseBoolOrDefault(val string, defaultVal bool) bool {
	if val == "" {
		return defaultVal
	}
	lower := strings.ToLower(strings.TrimSpace(val))
	return lower == "true" || lower == "1" || lower == "yes"
}

func parseOriginsOrDefault(raw string, defaultVal []string) []string {
	if strings.TrimSpace(raw) == "" {
		return defaultVal
	}
	parts := strings.Split(raw, ",")
	var origins []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	if len(origins) == 0 {
		return defaultVal
	}
	return origins
}

// findPredictableEnvFile checks explicit ENV_FILE, then ./.env, ../.env, or ../../.env.
// It avoids traversing arbitrary directory trees.
func findPredictableEnvFile() string {
	if custom := os.Getenv("ENV_FILE"); custom != "" {
		if _, err := os.Stat(custom); err == nil {
			return custom
		}
	}
	candidates := []string{
		".env",
		filepath.Join("..", ".env"),
		filepath.Join("..", "..", ".env"),
		filepath.Join("..", "..", "..", ".env"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// loadEnvFile reads a simple KEY=VALUE file and sets environment variables if they are not already set.
// Existing environment variables take precedence.
func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Strip optional surrounding quotes
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}

		// Real environment variable takes precedence: only set if unset
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}
