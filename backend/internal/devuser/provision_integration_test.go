//go:build integration

package devuser_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"giki-sports-backend/internal/auth"
	"giki-sports-backend/internal/authentication"
	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/database"
	"giki-sports-backend/internal/devuser"
	"giki-sports-backend/internal/session"
	"giki-sports-backend/internal/user"
)

func setupDisposableDB(t *testing.T) (*pgxpool.Pool, string, func()) {
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

	dbName := fmt.Sprintf("giki_sports_disposable_devuser_%d", time.Now().UnixNano())
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
		t.Fatalf("failed to initialize pgx connection pool: %v", err)
	}

	teardown := func() {
		t.Logf("Cleaning up disposable test database: %s", dbName)
		pool.Close()

		if err := dropDB(maintenanceDB, dbName); err != nil {
			t.Errorf("cleanup failure: failed to drop disposable test database %s: %v", dbName, err)
		}
		maintenanceDB.Close()
	}

	return pool, dbName, teardown
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

func TestDevUserProvision_Integration(t *testing.T) {
	pool, disposableDBName, teardown := setupDisposableDB(t)
	defer teardown()

	ctx := context.Background()

	// Initial safety check: Record read-only baseline of development database
	initialUsers, initialSessions := countDevRecords(t)

	directorPassword := "test-director-pass"
	studentPassword := "test-student-pass"

	// =========================================================================
	// TEST 1: Both presets create the correct account and exactly the intended role
	// =========================================================================
	t.Run("BothPresets_CreateCorrectAccountAndExactRole", func(t *testing.T) {
		resDir, err := devuser.Provision(ctx, pool, devuser.PresetDirector, directorPassword)
		if err != nil {
			t.Fatalf("failed to provision director preset: %v", err)
		}
		if resDir.Email != "director@example.test" {
			t.Errorf("expected email 'director@example.test', got %q", resDir.Email)
		}
		if resDir.Role != user.RoleSportsDirector {
			t.Errorf("expected role %q, got %q", user.RoleSportsDirector, resDir.Role)
		}
		if resDir.AlreadyExists {
			t.Errorf("expected AlreadyExists to be false for new director account")
		}

		resStu, err := devuser.Provision(ctx, pool, devuser.PresetStudent, studentPassword)
		if err != nil {
			t.Fatalf("failed to provision student preset: %v", err)
		}
		if resStu.Email != "student@example.test" {
			t.Errorf("expected email 'student@example.test', got %q", resStu.Email)
		}
		if resStu.Role != user.RoleStudent {
			t.Errorf("expected role %q, got %q", user.RoleStudent, resStu.Role)
		}
		if resStu.AlreadyExists {
			t.Errorf("expected AlreadyExists to be false for new student account")
		}

		// Verify roles directly in PostgreSQL
		checkRoles := func(email string, expectedRole user.Role) {
			var count int
			var actualRole string
			err := pool.QueryRow(ctx, `
				SELECT count(*), COALESCE(min(r.role), '')
				FROM giki_sports.users u
				JOIN giki_sports.user_roles r ON u.id = r.user_id
				WHERE LOWER(TRIM(u.email)) = $1;
			`, email).Scan(&count, &actualRole)
			if err != nil {
				t.Fatalf("failed to query roles for %s: %v", email, err)
			}
			if count != 1 {
				t.Errorf("expected exactly 1 role for %s, got %d", email, count)
			}
			if actualRole != string(expectedRole) {
				t.Errorf("expected role %q for %s, got %q", expectedRole, email, actualRole)
			}
		}

		checkRoles("director@example.test", user.RoleSportsDirector)
		checkRoles("student@example.test", user.RoleStudent)
	})

	// =========================================================================
	// TEST 2: The stored hash verifies with the existing Argon2id helper
	// =========================================================================
	t.Run("StoredHash_VerifiesWithArgon2idHelper", func(t *testing.T) {
		var storedHash string
		err := pool.QueryRow(ctx, `
			SELECT password_hash FROM giki_sports.users WHERE email = 'director@example.test';
		`).Scan(&storedHash)
		if err != nil {
			t.Fatalf("failed to query stored password hash: %v", err)
		}

		if !strings.HasPrefix(storedHash, "$argon2id$v=19$m=65536,t=3,p=4$") {
			t.Errorf("stored hash does not match production Argon2id RFC 9106 format: %q", storedHash)
		}

		matched, err := auth.Verify(directorPassword, storedHash)
		if err != nil {
			t.Fatalf("auth.Verify failed with error: %v", err)
		}
		if !matched {
			t.Errorf("auth.Verify returned false for correct password!")
		}

		// Negative check with wrong password
		matchedWrong, err := auth.Verify("test-wrong-pass", storedHash)
		if err != nil {
			t.Fatalf("auth.Verify failed on wrong password check: %v", err)
		}
		if matchedWrong {
			t.Errorf("auth.Verify returned true for incorrect password!")
		}
	})

	// =========================================================================
	// TEST 3: An existing account remains unchanged on rerun
	// =========================================================================
	t.Run("ExistingAccount_RemainsUnchangedOnRerun", func(t *testing.T) {
		var (
			originalID        string
			originalHash      string
			originalStatus    string
			originalCreatedAt time.Time
		)
		err := pool.QueryRow(ctx, `
			SELECT id, password_hash, status, created_at
			FROM giki_sports.users
			WHERE email = 'director@example.test';
		`).Scan(&originalID, &originalHash, &originalStatus, &originalCreatedAt)
		if err != nil {
			t.Fatalf("failed to query original user record: %v", err)
		}

		// Rerun provisioning with a completely different password
		resRerun, err := devuser.Provision(ctx, pool, devuser.PresetDirector, "test-rerun-pass")
		if err != nil {
			t.Fatalf("rerun provisioning failed: %v", err)
		}
		if !resRerun.AlreadyExists {
			t.Fatalf("expected AlreadyExists to be true on rerun")
		}
		if resRerun.UserID.String() != originalID {
			t.Errorf("expected user ID %s, got %s", originalID, resRerun.UserID)
		}
		if len(resRerun.ExistingRoles) != 1 || resRerun.ExistingRoles[0] != user.RoleSportsDirector {
			t.Errorf("expected existing role 'sports_director', got %v", resRerun.ExistingRoles)
		}

		// Verify database row was NOT modified
		var (
			currentHash      string
			currentStatus    string
			currentCreatedAt time.Time
		)
		err = pool.QueryRow(ctx, `
			SELECT password_hash, status, created_at
			FROM giki_sports.users
			WHERE email = 'director@example.test';
		`).Scan(&currentHash, &currentStatus, &currentCreatedAt)
		if err != nil {
			t.Fatalf("failed to query current user record: %v", err)
		}

		if currentHash != originalHash {
			t.Fatalf("CRITICAL SECURITY FLAW: Rerun altered existing password_hash in database!")
		}
		if currentStatus != originalStatus {
			t.Errorf("status changed from %s to %s", originalStatus, currentStatus)
		}
		if !currentCreatedAt.Equal(originalCreatedAt) {
			t.Errorf("created_at changed on rerun")
		}

		// Original password must still verify
		matchedOriginal, _ := auth.Verify(directorPassword, currentHash)
		if !matchedOriginal {
			t.Errorf("original password no longer verifies after rerun!")
		}
		// New attempted password must NOT verify
		matchedNew, _ := auth.Verify("test-rerun-pass", currentHash)
		if matchedNew {
			t.Errorf("new password verified when existing account should have been unchanged!")
		}
	})

	// =========================================================================
	// TEST 4: A role-insert failure rolls back the user insert
	// =========================================================================
	t.Run("RoleInsertFailure_RollsBackUserInsert", func(t *testing.T) {
		failEmail := "fail.test@example.test"
		// Pass an invalid role that violates the chk_user_roles_role_valid check constraint
		_, err := devuser.ProvisionUser(ctx, pool, failEmail, "Fail User", user.Role("invalid_role_value"), "test-fail-pass")
		if err == nil {
			t.Fatalf("expected error when inserting invalid role, got nil")
		}

		// Verify that the user insert was rolled back atomically
		var count int
		err = pool.QueryRow(ctx, `
			SELECT count(*) FROM giki_sports.users WHERE email = $1;
		`, failEmail).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query users: %v", err)
		}
		if count != 0 {
			t.Fatalf("CRITICAL: User row was persisted despite role insert failure (transaction rollback failed!)")
		}
	})

	// =========================================================================
	// TEST 5: The created account can log in through the existing auth service
	// =========================================================================
	t.Run("CreatedAccount_CanLoginThroughAuthService", func(t *testing.T) {
		userRepo := user.NewRepository(pool)
		sessionRepo := session.NewRepository(pool)
		authService, err := authentication.NewService(userRepo, sessionRepo)
		if err != nil {
			t.Fatalf("failed to construct auth service: %v", err)
		}

		// 1. Successful student login
		loginRes, err := authService.Login(ctx, "student@example.test", studentPassword)
		if err != nil {
			t.Fatalf("authService.Login failed for student: %v", err)
		}
		if loginRes.User.Email != "student@example.test" {
			t.Errorf("expected user email 'student@example.test', got %q", loginRes.User.Email)
		}
		if len(loginRes.User.Roles) != 1 || loginRes.User.Roles[0] != user.RoleStudent {
			t.Errorf("expected role 'student', got %v", loginRes.User.Roles)
		}
		if loginRes.RawToken == "" {
			t.Fatalf("expected non-empty RawToken from successful login")
		}

		// Verify session was created in DB
		var sessCount int
		err = pool.QueryRow(ctx, `
			SELECT count(*) FROM giki_sports.sessions WHERE user_id = $1 AND revoked_at IS NULL;
		`, loginRes.User.ID).Scan(&sessCount)
		if err != nil {
			t.Fatalf("failed to query sessions: %v", err)
		}
		if sessCount != 1 {
			t.Errorf("expected 1 active session in DB, got %d", sessCount)
		}

		// 2. Reject wrong password through auth service
		_, err = authService.Login(ctx, "student@example.test", "test-wrong-pass")
		if err != authentication.ErrInvalidCredentials {
			t.Errorf("expected ErrInvalidCredentials for wrong password, got: %v", err)
		}
	})

	// =========================================================================
	// TEST 6: Target validation rejects disallowed configurations
	// =========================================================================
	t.Run("TargetValidation_RejectsDisallowedConfigurations", func(t *testing.T) {
		// Non-loopback host
		if err := devuser.ValidateTarget("10.0.0.1", "giki_sports_complex"); err == nil {
			t.Errorf("expected error for non-loopback host 10.0.0.1, got nil")
		}

		// Disallowed database name
		if err := devuser.ValidateTarget("127.0.0.1", "giki_wallet_db"); err == nil {
			t.Errorf("expected error for disallowed database name giki_wallet_db, got nil")
		}

		// Allowed disposable database when passed explicitly
		if err := devuser.ValidateTarget("127.0.0.1", disposableDBName, disposableDBName); err != nil {
			t.Errorf("expected target validation to pass for disposable DB %q, got: %v", disposableDBName, err)
		}
	})

	// Final safety check: Primary development database remained completely unmutated
	assertDevDBUntouched(t, initialUsers, initialSessions)
}

