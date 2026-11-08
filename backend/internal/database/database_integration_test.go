//go:build integration

package database

import (
	"context"
	"strings"
	"testing"
	"time"

	"giki-sports-backend/internal/config"
)

// TestPostgresIntegration_ConnectAndQuery verifies real TCP connectivity against the sports complex PostgreSQL container.
// When this integration test is explicitly requested, missing configuration or an unreachable database must FAIL
// rather than silently skip.
func TestPostgresIntegration_ConnectAndQuery(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("integration test failed: configuration could not be loaded: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := NewPool(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("integration test failed: unable to connect to PostgreSQL container at %s:%d: %v",
			cfg.DB.Host, cfg.DB.Port, err)
	}
	defer pool.Close()

	// Execute SELECT 1 and check current database identity
	var currentDB string
	var currentUser string
	var checkVal int
	err = pool.QueryRow(ctx, "SELECT current_database(), current_user, 1;").Scan(&currentDB, &currentUser, &checkVal)
	if err != nil {
		t.Fatalf("integration test failed: query execution failed: %v", err)
	}

	if checkVal != 1 {
		t.Errorf("expected SELECT 1 to return 1, got %d", checkVal)
	}
	if currentDB != cfg.DB.Name {
		t.Errorf("expected database name '%s', got '%s'", cfg.DB.Name, currentDB)
	}
	if currentUser != cfg.DB.User {
		t.Errorf("expected database user '%s', got '%s'", cfg.DB.User, currentUser)
	}
}

// TestPostgresIntegration_UnreachableHostBoundedTimeout verifies that attempting to connect to an unreachable endpoint
// fails cleanly within the configured timeout without leaking credentials.
func TestPostgresIntegration_UnreachableHostBoundedTimeout(t *testing.T) {
	dummyCfg := config.DatabaseConfig{
		Host:            "127.0.0.1",
		Port:            65534, // Unused port on localhost
		Name:            "non_existent_db",
		User:            "test_user",
		Password:        "super_secret_unreachable_password",
		SSLMode:         "disable",
		MaxConns:        2,
		MinConns:        1,
		MaxConnLifetime: 1 * time.Hour,
		MaxConnIdleTime: 30 * time.Minute,
		ConnectTimeout:  1 * time.Second, // Bounded short timeout
	}

	ctx := context.Background()
	start := time.Now()
	pool, err := NewPool(ctx, dummyCfg)
	elapsed := time.Since(start)

	if pool != nil {
		pool.Close()
		t.Fatalf("expected NewPool to fail on unreachable host, but got active pool")
	}
	if err == nil {
		t.Fatalf("expected error connecting to unreachable host, got nil")
	}

	// Must fail within a bounded time (e.g. within 3 seconds for a 1-second timeout)
	if elapsed > 3*time.Second {
		t.Errorf("expected connection failure within bounded timeout (~1s), took %s", elapsed)
	}

	// Must not leak secret password in error message
	if strings.Contains(err.Error(), "super_secret_unreachable_password") {
		t.Fatalf("CRITICAL SECURITY FLAW: database connection error leaked password: %v", err)
	}
}
