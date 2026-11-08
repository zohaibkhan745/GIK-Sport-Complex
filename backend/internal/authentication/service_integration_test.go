//go:build integration

package authentication_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"giki-sports-backend/internal/auth"
	"giki-sports-backend/internal/authentication"
	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/database"
	"giki-sports-backend/internal/session"
	"giki-sports-backend/internal/user"
)

// setupDisposableDB provisions a uniquely named database, executes migrations,
// and returns an active connection pool alongside a teardown cleanup function.
func setupDisposableDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("integration test failed: unable to load configuration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	maintenanceDB, err := database.NewSQLDB(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("integration test failed: unable to connect to PostgreSQL server: %v", err)
	}

	dbName := fmt.Sprintf("giki_sports_disposable_authsvc_%d", time.Now().UnixNano())
	_, err = maintenanceDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s;", dbName))
	if err != nil {
		maintenanceDB.Close()
		t.Fatalf("failed to create disposable database %s: %v", dbName, err)
	}

	testDBCfg := cfg.DB
	testDBCfg.Name = dbName
	migSQLDB, err := database.NewSQLDB(ctx, testDBCfg)
	if err != nil {
		dropDB(maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to connect to disposable database for migrations: %v", err)
	}

	migrationsDir := filepath.Join("..", "..", "migrations")
	runner, err := database.NewMigrationRunnerFromDir(migSQLDB, migrationsDir)
	if err != nil {
		migSQLDB.Close()
		dropDB(maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to initialize migration runner: %v", err)
	}

	if _, err := runner.Up(ctx); err != nil {
		runner.Close()
		migSQLDB.Close()
		dropDB(maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to apply migrations to disposable database: %v", err)
	}
	runner.Close()
	migSQLDB.Close()

	pool, err := database.NewPool(ctx, testDBCfg)
	if err != nil {
		dropDB(maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to initialize pgx connection pool for repository: %v", err)
	}

	teardown := func() {
		t.Logf("Cleaning up disposable test database: %s", dbName)
		pool.Close()

		if err := dropDB(maintenanceDB, dbName); err != nil {
			t.Errorf("cleanup failure: failed to drop disposable test database %s: %v", dbName, err)
		}
		maintenanceDB.Close()
	}

	return pool, teardown
}

func dropDB(maintenanceDB *sql.DB, dbName string) error {
	if !strings.HasPrefix(dbName, "giki_sports_disposable_") {
		return fmt.Errorf("safety check violated: refusing to drop non-disposable database %s", dbName)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = maintenanceDB.ExecContext(ctx, fmt.Sprintf(`
		SELECT pg_terminate_backend(pg_stat_activity.pid)
		FROM pg_stat_activity
		WHERE pg_stat_activity.datname = '%s'
		  AND pid <> pg_backend_pid();
	`, dbName))

	_, err := maintenanceDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s;", dbName))
	return err
}

func countDevRecords(t *testing.T) (int, int) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load dev config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	devPool, err := database.NewPool(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("failed to connect to dev database: %v", err)
	}
	defer devPool.Close()

	var userCount, sessCount int
	err = devPool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.users;").Scan(&userCount)
	if err != nil {
		t.Fatalf("failed to count users in dev database: %v", err)
	}
	err = devPool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.sessions;").Scan(&sessCount)
	if err != nil {
		t.Fatalf("failed to count sessions in dev database: %v", err)
	}
	return userCount, sessCount
}

func insertSyntheticDirector(t *testing.T, pool *pgxpool.Pool, password string) uuid.UUID {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hash, err := auth.Hash(password)
	if err != nil {
		t.Fatalf("failed to generate argon2id hash: %v", err)
	}

	userID := uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd")

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'director.sports@giki.edu.pk', 'Sports Director', $2, 'active');
	`, userID, hash)
	if err != nil {
		t.Fatalf("failed to insert synthetic user: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.user_roles (user_id, role)
		VALUES ($1, 'faculty'), ($1, 'sports_director');
	`, userID)
	if err != nil {
		t.Fatalf("failed to insert user roles: %v", err)
	}

	return userID
}

func TestAuthenticationService_Integration(t *testing.T) {
	initialUsers, initialSessions := countDevRecords(t)

	pool, teardown := setupDisposableDB(t)
	defer teardown()

	const testPassword = "DirectorSecurePassword123!"
	directorID := insertSyntheticDirector(t, pool, testPassword)

	t0 := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	fakeClock := session.NewTestClock(t0)

	userRepo := user.NewRepository(pool)
	sessionRepo := session.NewRepositoryWithClock(pool, fakeClock)

	authSvc, err := authentication.NewService(userRepo, sessionRepo)
	if err != nil {
		t.Fatalf("failed to initialize authentication service: %v", err)
	}

	ctx := context.Background()

	var activeRawToken string
	var sessionID uuid.UUID

	t.Run("LoginWithValidCredentials_Succeeds_AndStoresHashNotToken", func(t *testing.T) {
		loginRes, err := authSvc.Login(ctx, "Director.Sports@GIKI.edu.pk  ", testPassword)
		if err != nil {
			t.Fatalf("expected successful login, got %v", err)
		}

		if loginRes.RawToken == "" {
			t.Fatal("expected non-empty raw session token")
		}
		activeRawToken = loginRes.RawToken
		sessionID = loginRes.Session.ID

		if loginRes.User.ID != directorID {
			t.Errorf("expected user ID %s, got %s", directorID, loginRes.User.ID)
		}
		if loginRes.User.Email != "director.sports@giki.edu.pk" {
			t.Errorf("expected normalized email, got %s", loginRes.User.Email)
		}
		if len(loginRes.User.Roles) != 2 || loginRes.User.Roles[1] != user.RoleSportsDirector {
			t.Errorf("expected roles [faculty, sports_director], got %v", loginRes.User.Roles)
		}

		// Verify database row in giki_sports.sessions:
		// Stored token_hash must equal sha256(rawToken), NEVER the raw token string
		var storedHash string
		err = pool.QueryRow(ctx, "SELECT token_hash FROM giki_sports.sessions WHERE id = $1;", sessionID).Scan(&storedHash)
		if err != nil {
			t.Fatalf("failed to query stored session hash: %v", err)
		}

		expectedHash, err := auth.HashSessionToken(activeRawToken)
		if err != nil {
			t.Fatalf("failed to hash raw token: %v", err)
		}

		if storedHash != expectedHash {
			t.Errorf("stored hash mismatch: expected %s, got %s", expectedHash, storedHash)
		}
		if storedHash == activeRawToken {
			t.Fatal("SECURITY VIOLATION: raw token was persisted directly into the database!")
		}
	})

	t.Run("ValidateSession_ReturnsPrincipalWithCurrentRoles", func(t *testing.T) {
		principal, err := authSvc.ValidateSession(ctx, activeRawToken)
		if err != nil {
			t.Fatalf("expected session validation to succeed, got %v", err)
		}

		if principal.User.ID != directorID {
			t.Errorf("expected principal user ID %s, got %s", directorID, principal.User.ID)
		}
		if len(principal.User.Roles) != 2 {
			t.Errorf("expected 2 roles, got %d", len(principal.User.Roles))
		}
		if principal.Session.ID != sessionID {
			t.Errorf("expected session ID %s, got %s", sessionID, principal.Session.ID)
		}
	})

	t.Run("LoginRejections_WrongPassword_And_UnknownUser", func(t *testing.T) {
		// Wrong password
		_, err := authSvc.Login(ctx, "director.sports@giki.edu.pk", "WrongPassword!")
		if !errors.Is(err, authentication.ErrInvalidCredentials) {
			t.Errorf("expected ErrInvalidCredentials for wrong password, got %v", err)
		}

		// Unknown user
		_, err = authSvc.Login(ctx, "unknown.user@giki.edu.pk", testPassword)
		if !errors.Is(err, authentication.ErrInvalidCredentials) {
			t.Errorf("expected ErrInvalidCredentials for unknown user, got %v", err)
		}
	})

	t.Run("IdleExpiryViaInjectedClockRejectsValidation", func(t *testing.T) {
		// Advance clock past 30-minute idle threshold without thread sleeps
		fakeClock.Advance(session.IdleTimeout + time.Minute)

		principal, err := authSvc.ValidateSession(ctx, activeRawToken)
		if !errors.Is(err, authentication.ErrInvalidSession) {
			t.Errorf("expected ErrInvalidSession after idle expiry, got %v", err)
		}
		if principal != nil {
			t.Errorf("expected nil principal on expired session, got %+v", principal)
		}
	})

	t.Run("Logout_RevokesSession_AndRejectsSubsequentValidation", func(t *testing.T) {
		// Reset clock to fresh time and login again
		fakeClock.Set(time.Date(2026, 10, 18, 14, 0, 0, 0, time.UTC))
		loginRes2, err := authSvc.Login(ctx, "director.sports@giki.edu.pk", testPassword)
		if err != nil {
			t.Fatalf("fresh login failed: %v", err)
		}

		// Confirm valid initially
		_, err = authSvc.ValidateSession(ctx, loginRes2.RawToken)
		if err != nil {
			t.Fatalf("expected fresh session to be valid, got %v", err)
		}

		// Logout
		err = authSvc.Logout(ctx, loginRes2.RawToken)
		if err != nil {
			t.Fatalf("logout failed: %v", err)
		}

		// Subsequent validation must fail with ErrInvalidSession
		_, err = authSvc.ValidateSession(ctx, loginRes2.RawToken)
		if !errors.Is(err, authentication.ErrInvalidSession) {
			t.Errorf("expected ErrInvalidSession after logout, got %v", err)
		}

		// Repeated logout must succeed idempotently
		err = authSvc.Logout(ctx, loginRes2.RawToken)
		if err != nil {
			t.Errorf("repeated logout should be idempotent success, got %v", err)
		}
	})

	// Final safety check: development database remained completely untouched
	finalUsers, finalSessions := countDevRecords(t)
	if finalUsers != initialUsers || finalSessions != initialSessions {
		t.Fatalf("safety violation: dev records modified! users: %d->%d, sessions: %d->%d", initialUsers, finalUsers, initialSessions, finalSessions)
	}
}
