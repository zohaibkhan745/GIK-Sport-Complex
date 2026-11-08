//go:build integration

package user_test

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

	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/database"
	"giki-sports-backend/internal/user"
)

// setupDisposableDB provisions a uniquely named database, runs all migrations,
// and returns an active *pgxpool.Pool alongside a teardown cleanup function.
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

	dbName := fmt.Sprintf("giki_sports_disposable_userrepo_%d", time.Now().UnixNano())
	_, err = maintenanceDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s;", dbName))
	if err != nil {
		maintenanceDB.Close()
		t.Fatalf("failed to create disposable database %s: %v", dbName, err)
	}

	// Connect via *sql.DB to run migrations
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

	// Initialize pgxpool.Pool for repository testing
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

// insertSyntheticFixtures inserts deterministic test accounts into the disposable database.
// NOTE: Synthetic password hashes are test fixtures satisfying schema constraints,
// not evidence of password security.
func insertSyntheticFixtures(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Fixture 1: Active user with multiple roles (inserted in non-alphabetical order to verify sorting)
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES (
			'11111111-1111-1111-1111-111111111111',
			'faculty.director@giki.edu.pk',
			'Dr. Faculty Director',
			'$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$c29tZWhhc2gx',
			'active'
		);
		INSERT INTO giki_sports.user_roles (user_id, role)
		VALUES 
			('11111111-1111-1111-1111-111111111111', 'sports_director'),
			('11111111-1111-1111-1111-111111111111', 'faculty');
	`)
	if err != nil {
		t.Fatalf("failed to insert fixture 1: %v", err)
	}

	// Fixture 2: Inactive user with 1 role
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES (
			'22222222-2222-2222-2222-222222222222',
			'inactive.student@giki.edu.pk',
			'Inactive Student',
			'$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$c29tZWhhc2gy',
			'inactive'
		);
		INSERT INTO giki_sports.user_roles (user_id, role)
		VALUES 
			('22222222-2222-2222-2222-222222222222', 'student');
	`)
	if err != nil {
		t.Fatalf("failed to insert fixture 2: %v", err)
	}

	// Fixture 3: Active user with no roles
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES (
			'33333333-3333-3333-3333-333333333333',
			'noroles.user@giki.edu.pk',
			'User With No Roles',
			'$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$c29tZWhhc2gz',
			'active'
		);
	`)
	if err != nil {
		t.Fatalf("failed to insert fixture 3: %v", err)
	}
}

func countDevUsers(t *testing.T) int {
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

	var count int
	err = devPool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.users;").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count users in dev database: %v", err)
	}
	return count
}

func TestPostgresRepository_Integration(t *testing.T) {
	// Baseline check: record development database row count before running tests
	initialDevUserCount := countDevUsers(t)

	pool, teardown := setupDisposableDB(t)
	defer teardown()

	insertSyntheticFixtures(t, pool)
	repo := user.NewRepository(pool)
	ctx := context.Background()

	t.Run("ExistingLookupByIDAndEmail", func(t *testing.T) {
		uid := uuid.MustParse("11111111-1111-1111-1111-111111111111")

		// Lookup by ID
		byID, err := repo.GetByID(ctx, uid)
		if err != nil {
			t.Fatalf("GetByID failed: %v", err)
		}
		if byID.Email != "faculty.director@giki.edu.pk" {
			t.Errorf("expected email faculty.director@giki.edu.pk, got %s", byID.Email)
		}
		if byID.DisplayName != "Dr. Faculty Director" {
			t.Errorf("expected display name Dr. Faculty Director, got %s", byID.DisplayName)
		}
		if byID.Status != user.StatusActive {
			t.Errorf("expected status active, got %s", byID.Status)
		}

		// Lookup by Email
		byEmail, err := repo.GetByEmail(ctx, "faculty.director@giki.edu.pk")
		if err != nil {
			t.Fatalf("GetByEmail failed: %v", err)
		}
		if byEmail.ID != uid {
			t.Errorf("expected ID %s, got %s", uid, byEmail.ID)
		}
	})

	t.Run("CaseAndWhitespaceEmailNormalization", func(t *testing.T) {
		// Mixed case, tabs, and leading/trailing whitespace
		inputEmail := "  \tFaculty.Director@GIKI.EDU.PK \n "
		u, err := repo.GetByEmail(ctx, inputEmail)
		if err != nil {
			t.Fatalf("GetByEmail with unnormalized input failed: %v", err)
		}
		if u.Email != "faculty.director@giki.edu.pk" {
			t.Errorf("expected normalized email in result, got %s", u.Email)
		}
	})

	t.Run("UnknownUserBehavior", func(t *testing.T) {
		// Unknown email
		_, err := repo.GetByEmail(ctx, "nonexistent@giki.edu.pk")
		if err == nil {
			t.Fatal("expected ErrNotFound for unknown email, got nil")
		}
		if !errors.Is(err, user.ErrNotFound) {
			t.Errorf("expected errors.Is(err, user.ErrNotFound), got %v", err)
		}

		// Unknown UUID
		randomUUID := uuid.New()
		_, err = repo.GetByID(ctx, randomUUID)
		if err == nil {
			t.Fatal("expected ErrNotFound for unknown UUID, got nil")
		}
		if !errors.Is(err, user.ErrNotFound) {
			t.Errorf("expected errors.Is(err, user.ErrNotFound), got %v", err)
		}

		// Nil UUID
		_, err = repo.GetByID(ctx, uuid.Nil)
		if err == nil {
			t.Fatal("expected ErrNotFound for uuid.Nil, got nil")
		}
		if !errors.Is(err, user.ErrNotFound) {
			t.Errorf("expected errors.Is(err, user.ErrNotFound), got %v", err)
		}

		// Empty email
		_, err = repo.GetByEmail(ctx, "   ")
		if err == nil {
			t.Fatal("expected ErrNotFound for empty email, got nil")
		}
		if !errors.Is(err, user.ErrNotFound) {
			t.Errorf("expected errors.Is(err, user.ErrNotFound), got %v", err)
		}
	})

	t.Run("MultipleRolesAndDeterministicOrdering", func(t *testing.T) {
		u, err := repo.GetByEmail(ctx, "faculty.director@giki.edu.pk")
		if err != nil {
			t.Fatalf("GetByEmail failed: %v", err)
		}

		if len(u.Roles) != 2 {
			t.Fatalf("expected exactly 2 roles, got %d", len(u.Roles))
		}

		// Alphabetical deterministic order: "faculty" < "sports_director"
		expectedOrder := []user.Role{user.RoleFaculty, user.RoleSportsDirector}
		for i, r := range expectedOrder {
			if u.Roles[i] != r {
				t.Errorf("role at index %d = %q, expected %q", i, u.Roles[i], r)
			}
		}
	})

	t.Run("UserWithNoRolesReturnsEmptyNonNilSlice", func(t *testing.T) {
		u, err := repo.GetByEmail(ctx, "noroles.user@giki.edu.pk")
		if err != nil {
			t.Fatalf("GetByEmail failed: %v", err)
		}

		if u.Roles == nil {
			t.Fatal("expected non-nil slice for roles, got nil")
		}
		if len(u.Roles) != 0 {
			t.Errorf("expected 0 roles, got %d", len(u.Roles))
		}
	})

	t.Run("InactiveAccountStatusRemainsVisible", func(t *testing.T) {
		// An inactive user remains readable; status reflects Inactive
		u, err := repo.GetByEmail(ctx, "inactive.student@giki.edu.pk")
		if err != nil {
			t.Fatalf("GetByEmail failed for inactive user: %v", err)
		}

		if u.Status != user.StatusInactive {
			t.Errorf("expected status inactive, got %s", u.Status)
		}
		if len(u.Roles) != 1 || u.Roles[0] != user.RoleStudent {
			t.Errorf("expected [student] role, got %v", u.Roles)
		}
	})

	t.Run("CredentialLookupReturnsSyntheticHashInternallyWhileOrdinaryReadsExcludeIt", func(t *testing.T) {
		const expectedHash = "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$c29tZWhhc2gx"

		// 1. Ordinary GetByEmail: struct User does not contain password hash field
		ordinaryUser, err := repo.GetByEmail(ctx, "faculty.director@giki.edu.pk")
		if err != nil {
			t.Fatalf("GetByEmail failed: %v", err)
		}
		if ordinaryUser.DisplayName != "Dr. Faculty Director" {
			t.Errorf("expected Dr. Faculty Director, got %s", ordinaryUser.DisplayName)
		}

		// 2. Credential lookup GetCredentialsByEmail returns the stored hash
		creds, err := repo.GetCredentialsByEmail(ctx, "faculty.director@giki.edu.pk")
		if err != nil {
			t.Fatalf("GetCredentialsByEmail failed: %v", err)
		}
		if creds.PasswordHash != expectedHash {
			t.Errorf("expected synthetic hash %q, got %q", expectedHash, creds.PasswordHash)
		}
		if creds.User.ID != ordinaryUser.ID {
			t.Errorf("expected user ID %s, got %s", ordinaryUser.ID, creds.User.ID)
		}

		// 3. Credential lookup with unknown email returns ErrNotFound
		_, err = repo.GetCredentialsByEmail(ctx, "unknown@giki.edu.pk")
		if !errors.Is(err, user.ErrNotFound) {
			t.Errorf("expected ErrNotFound for unknown email in credentials lookup, got %v", err)
		}
	})

	t.Run("SQLInjectionEmailTreatedAsLiteralData", func(t *testing.T) {
		maliciousInputs := []string{
			"' OR '1'='1",
			"admin'--",
			"faculty.director@giki.edu.pk' OR 1=1--",
			"'; DROP TABLE giki_sports.users;--",
		}

		for _, input := range maliciousInputs {
			_, err := repo.GetByEmail(ctx, input)
			if err == nil {
				t.Errorf("expected error for SQL injection input %q, got nil", input)
			}
			if !errors.Is(err, user.ErrNotFound) {
				t.Errorf("expected ErrNotFound for SQL injection input %q, got %v", input, err)
			}
		}
	})

	t.Run("CanceledContextReturnsError", func(t *testing.T) {
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel() // immediately canceled

		_, err := repo.GetByEmail(canceledCtx, "faculty.director@giki.edu.pk")
		if err == nil {
			t.Fatal("expected error on canceled context, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})

	// Post-check: verify development database rows were not altered by integration tests
	finalDevUserCount := countDevUsers(t)
	if finalDevUserCount != initialDevUserCount {
		t.Fatalf("safety violation: development database user count changed from %d to %d", initialDevUserCount, finalDevUserCount)
	}
}
