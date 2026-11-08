//go:build integration

package seed_test

import (
	"context"
	"database/sql"
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
	"giki-sports-backend/internal/facility"
	"giki-sports-backend/internal/seed"
)

// setupDisposableDB provisions a uniquely named database, runs migrations up to version 2,
// and returns an active *pgxpool.Pool alongside a teardown cleanup function.
func setupDisposableDB(t *testing.T, applyMigrations bool) (*pgxpool.Pool, func()) {
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

	dbName := fmt.Sprintf("giki_sports_disposable_seed_%d", time.Now().UnixNano())
	_, err = maintenanceDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s;", dbName))
	if err != nil {
		maintenanceDB.Close()
		t.Fatalf("failed to create disposable database %s: %v", dbName, err)
	}

	testDBCfg := cfg.DB
	testDBCfg.Name = dbName

	if applyMigrations {
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
	}

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

	return pool, teardown
}

func dropDB(maintenanceDB *sql.DB, dbName string) error {
	if !strings.HasPrefix(dbName, "giki_sports_disposable_") {
		return fmt.Errorf("safety check violated: refusing to drop non-disposable database %s", dbName)
	}

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = maintenanceDB.ExecContext(cleanupCtx, fmt.Sprintf(`
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = '%s' AND pid <> pg_backend_pid();`, dbName))

	_, err := maintenanceDB.ExecContext(cleanupCtx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE);", dbName))
	return err
}

// TestSeedDemo_FirstAndRepeatedExecution verifies:
// - First execution inserts expected facilities and schedules.
// - Repeated execution creates no duplicates (all records skipped).
// - Facility repository can read the inserted records correctly.
func TestSeedDemo_FirstAndRepeatedExecution(t *testing.T) {
	pool, teardown := setupDisposableDB(t, true)
	defer teardown()

	ctx := context.Background()

	// 1. First execution
	summary1, err := seed.ApplyDemo(ctx, pool)
	if err != nil {
		t.Fatalf("first seed execution failed: %v", err)
	}

	if summary1.FacilitiesInserted != 2 || summary1.FacilitiesSkipped != 0 {
		t.Errorf("expected 2 facilities inserted and 0 skipped, got: %+v", summary1)
	}
	if summary1.SchedulesInserted != 5 || summary1.SchedulesSkipped != 0 {
		t.Errorf("expected 5 schedules inserted and 0 skipped, got: %+v", summary1)
	}

	// Verify database row counts
	var facCount, schedCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.facilities;").Scan(&facCount); err != nil {
		t.Fatalf("failed to count facilities: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.schedules;").Scan(&schedCount); err != nil {
		t.Fatalf("failed to count schedules: %v", err)
	}
	if facCount != 2 || schedCount != 5 {
		t.Fatalf("expected 2 facilities and 5 schedules, got fac=%d, sched=%d", facCount, schedCount)
	}

	// Verify facility repository can read the inserted demo records correctly
	repo := facility.NewRepository(pool)
	facilities, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("facility repository List failed: %v", err)
	}
	if len(facilities) != 2 {
		t.Fatalf("expected 2 facilities from repo.List, got %d", len(facilities))
	}

	// Deterministic facility ordering: 'demo-badminton-court' and 'demo-table-tennis'
	badminton := facilities[0]
	if badminton.Slug != "demo-badminton-court" || len(badminton.Schedules) != 3 {
		t.Errorf("unexpected badminton court read model: %+v", badminton)
	}

	tt := facilities[1]
	if tt.Slug != "demo-table-tennis" || len(tt.Schedules) != 2 {
		t.Errorf("unexpected table tennis read model: %+v", tt)
	}

	// 2. Repeated execution
	summary2, err := seed.ApplyDemo(ctx, pool)
	if err != nil {
		t.Fatalf("repeated seed execution failed: %v", err)
	}

	if summary2.FacilitiesInserted != 0 || summary2.FacilitiesSkipped != 2 {
		t.Errorf("expected 0 facilities inserted and 2 skipped on rerun, got: %+v", summary2)
	}
	if summary2.SchedulesInserted != 0 || summary2.SchedulesSkipped != 5 {
		t.Errorf("expected 0 schedules inserted and 5 skipped on rerun, got: %+v", summary2)
	}

	// Verify database counts remain unchanged
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.facilities;").Scan(&facCount); err != nil {
		t.Fatalf("failed to recount facilities: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.schedules;").Scan(&schedCount); err != nil {
		t.Fatalf("failed to recount schedules: %v", err)
	}
	if facCount != 2 || schedCount != 5 {
		t.Fatalf("counts changed on rerun: fac=%d, sched=%d", facCount, schedCount)
	}
}

// TestSeedDemo_PreservesAdministratorEdits verifies:
// - If an administrator modifies facility fields or schedules, a seed rerun
//   does NOT overwrite the modifications or recreate original schedules.
func TestSeedDemo_PreservesAdministratorEdits(t *testing.T) {
	pool, teardown := setupDisposableDB(t, true)
	defer teardown()

	ctx := context.Background()

	// Initial seed
	if _, err := seed.ApplyDemo(ctx, pool); err != nil {
		t.Fatalf("initial seed failed: %v", err)
	}

	// Simulate administrator edits:
	// 1. Rename facility name & location
	_, err := pool.Exec(ctx, `
		UPDATE giki_sports.facilities 
		SET name = '[Custom] Main Badminton Arena', location = 'Renovated East Wing'
		WHERE slug = 'demo-badminton-court';
	`)
	if err != nil {
		t.Fatalf("failed to simulate facility edit: %v", err)
	}

	// 2. Modify an existing schedule's hours
	_, err = pool.Exec(ctx, `
		UPDATE giki_sports.schedules 
		SET start_time = '08:00:00', end_time = '10:00:00'
		WHERE id = 'd0000001-0000-4000-8000-000000000011';
	`)
	if err != nil {
		t.Fatalf("failed to simulate schedule edit: %v", err)
	}

	// 3. Delete one schedule
	_, err = pool.Exec(ctx, `
		DELETE FROM giki_sports.schedules 
		WHERE id = 'd0000001-0000-4000-8000-000000000013';
	`)
	if err != nil {
		t.Fatalf("failed to simulate schedule deletion: %v", err)
	}

	// 4. Insert a new custom administrator schedule
	customSchedID := uuid.MustParse("d0000001-0000-4000-8000-000000000099")
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.schedules (id, facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 'd0000001-0000-4000-8000-000000000001', 7, '10:00:00', '12:00:00', 'staff');
	`, customSchedID)
	if err != nil {
		t.Fatalf("failed to insert custom schedule: %v", err)
	}

	// Run seed again
	summary, err := seed.ApplyDemo(ctx, pool)
	if err != nil {
		t.Fatalf("seed rerun failed: %v", err)
	}

	if summary.FacilitiesInserted != 0 || summary.FacilitiesSkipped != 2 {
		t.Errorf("expected 0 facilities inserted and 2 skipped, got: %+v", summary)
	}
	if summary.SchedulesInserted != 0 || summary.SchedulesSkipped != 5 {
		t.Errorf("expected 0 schedules inserted and 5 skipped, got: %+v", summary)
	}

	// Verify using facility repository:
	repo := facility.NewRepository(pool)
	fac, err := repo.GetBySlug(ctx, "demo-badminton-court")
	if err != nil {
		t.Fatalf("failed to fetch badminton court after rerun: %v", err)
	}

	// 1. Facility fields must remain the administrator's edits
	if fac.Name != "[Custom] Main Badminton Arena" {
		t.Errorf("expected name to be preserved as '[Custom] Main Badminton Arena', got: %q", fac.Name)
	}
	if fac.Location == nil || *fac.Location != "Renovated East Wing" {
		t.Errorf("expected location to be preserved as 'Renovated East Wing', got: %v", fac.Location)
	}

	// 2. Schedules must not be overwritten or recreated:
	// Total schedules for this facility should be 3 (1 edited + 1 untouched + 1 custom added)
	if len(fac.Schedules) != 3 {
		t.Fatalf("expected exactly 3 schedules on edited facility, got %d", len(fac.Schedules))
	}

	// Deleted schedule (day 5, Friday) must NOT have been recreated!
	for _, s := range fac.Schedules {
		if s.DayOfWeek == 5 {
			t.Errorf("CRITICAL BUG: deleted schedule for day 5 was unexpectedly recreated!")
		}
		if s.ID == uuid.MustParse("d0000001-0000-4000-8000-000000000011") {
			if s.StartTime != "08:00:00" || s.EndTime != "10:00:00" {
				t.Errorf("expected edited schedule time '08:00:00'-'10:00:00', got %s-%s", s.StartTime, s.EndTime)
			}
		}
	}
}

// TestSeedDemo_UnrelatedFacilityUntouched verifies:
// - Pre-existing unrelated facilities remain completely untouched by seeding.
func TestSeedDemo_UnrelatedFacilityUntouched(t *testing.T) {
	pool, teardown := setupDisposableDB(t, true)
	defer teardown()

	ctx := context.Background()

	// Insert an unrelated facility with a schedule
	unrelatedID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, description, location, status)
		VALUES ($1, 'olympic-pool', 'Campus Olympic Pool', 'Standard pool', 'Aquatic Center', 'active');
	`, unrelatedID)
	if err != nil {
		t.Fatalf("failed to insert unrelated facility: %v", err)
	}

	unrelatedSchedID := uuid.MustParse("11111111-1111-4111-8111-111111111112")
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.schedules (id, facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, $2, 1, '06:00:00', '08:00:00', 'general');
	`, unrelatedSchedID, unrelatedID)
	if err != nil {
		t.Fatalf("failed to insert unrelated schedule: %v", err)
	}

	// Apply demo seed
	summary, err := seed.ApplyDemo(ctx, pool)
	if err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	if summary.FacilitiesInserted != 2 || summary.SchedulesInserted != 5 {
		t.Errorf("unexpected seed summary: %+v", summary)
	}

	// Check unrelated facility is intact via repository
	repo := facility.NewRepository(pool)
	fac, err := repo.GetBySlug(ctx, "olympic-pool")
	if err != nil {
		t.Fatalf("GetBySlug for unrelated facility failed: %v", err)
	}
	if fac.Name != "Campus Olympic Pool" || len(fac.Schedules) != 1 {
		t.Errorf("unrelated facility modified: %+v", fac)
	}

	// Verify total facility count is 3 (1 unrelated + 2 demo)
	var totalFacs int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.facilities;").Scan(&totalFacs); err != nil {
		t.Fatalf("failed to count facilities: %v", err)
	}
	if totalFacs != 3 {
		t.Errorf("expected 3 facilities total, got %d", totalFacs)
	}
}

// TestSeedDemo_ConflictingFixtureReportedWithoutPartialInserts verifies:
// - A slug collision with a different ID causes an actionable error.
// - An ID collision with a different slug causes an actionable error.
// - In both cases, the transaction aborts and NO partial records are inserted.
func TestSeedDemo_ConflictingFixtureReportedWithoutPartialInserts(t *testing.T) {
	// Case A: Slug collision with different ID
	t.Run("SlugCollisionDifferentID", func(t *testing.T) {
		pool, teardown := setupDisposableDB(t, true)
		defer teardown()

		ctx := context.Background()

		// Insert conflicting facility with demo slug but random UUID
		conflictID := uuid.MustParse("99999999-9999-4999-8999-999999999999")
		_, err := pool.Exec(ctx, `
			INSERT INTO giki_sports.facilities (id, slug, name, status)
			VALUES ($1, 'demo-badminton-court', 'Colliding Court', 'active');
		`, conflictID)
		if err != nil {
			t.Fatalf("failed to insert colliding facility: %v", err)
		}

		// Run seed: must fail with actionable conflict error
		summary, err := seed.ApplyDemo(ctx, pool)
		if err == nil {
			t.Fatalf("expected error on slug collision, got nil error and summary: %+v", summary)
		}
		if !strings.Contains(err.Error(), "fixture conflict") || !strings.Contains(err.Error(), "demo-badminton-court") {
			t.Errorf("expected actionable conflict message, got: %v", err)
		}

		// Verify atomicity & no partial inserts:
		// Only the 1 pre-existing conflicting facility should exist; facility 2 was NOT inserted!
		var facCount, schedCount int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.facilities;").Scan(&facCount); err != nil {
			t.Fatalf("failed to count facilities: %v", err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.schedules;").Scan(&schedCount); err != nil {
			t.Fatalf("failed to count schedules: %v", err)
		}
		if facCount != 1 || schedCount != 0 {
			t.Errorf("transaction atomicity violated: expected 1 facility and 0 schedules, got fac=%d, sched=%d", facCount, schedCount)
		}
	})

	// Case B: ID collision with different slug
	t.Run("IDCollisionDifferentSlug", func(t *testing.T) {
		pool, teardown := setupDisposableDB(t, true)
		defer teardown()

		ctx := context.Background()

		// Insert facility with demo ID 1 but different slug
		fixtureID := uuid.MustParse("d0000001-0000-4000-8000-000000000001")
		_, err := pool.Exec(ctx, `
			INSERT INTO giki_sports.facilities (id, slug, name, status)
			VALUES ($1, 'custom-slug-conflict', 'Colliding ID Facility', 'active');
		`, fixtureID)
		if err != nil {
			t.Fatalf("failed to insert colliding facility: %v", err)
		}

		// Run seed: must fail with actionable conflict error
		summary, err := seed.ApplyDemo(ctx, pool)
		if err == nil {
			t.Fatalf("expected error on ID collision, got nil error and summary: %+v", summary)
		}
		if !strings.Contains(err.Error(), "fixture conflict") || !strings.Contains(err.Error(), fixtureID.String()) {
			t.Errorf("expected actionable conflict message, got: %v", err)
		}

		// Verify atomicity & no partial inserts
		var facCount int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.facilities;").Scan(&facCount); err != nil {
			t.Fatalf("failed to count facilities: %v", err)
		}
		if facCount != 1 {
			t.Errorf("expected exactly 1 facility, got %d", facCount)
		}
	})
}

// TestSeedDemo_MissingMigrationsFails verifies:
// - Seed fails clearly when migration 2 has not been applied.
func TestSeedDemo_MissingMigrationsFails(t *testing.T) {
	pool, teardown := setupDisposableDB(t, false) // false -> no migrations applied
	defer teardown()

	ctx := context.Background()

	_, err := seed.ApplyDemo(ctx, pool)
	if err == nil {
		t.Fatalf("expected error when migrations are missing, got nil")
	}
	if !strings.Contains(err.Error(), "required schema migrations have not been applied") {
		t.Errorf("expected missing migrations message, got: %v", err)
	}
}

// TestSeedDemo_TransactionRollbackOnFailure verifies:
// - If any error occurs midway through ApplyFixtures, the entire transaction rolls back cleanly.
func TestSeedDemo_TransactionRollbackOnFailure(t *testing.T) {
	pool, teardown := setupDisposableDB(t, true)
	defer teardown()

	ctx := context.Background()

	invalidFixtures := []seed.FacilityFixture{
		{
			ID:     uuid.MustParse("a0000001-0000-4000-8000-000000000001"),
			Slug:   "valid-first-fac",
			Name:   "Valid Facility",
			Status: "active",
		},
		{
			ID:     uuid.MustParse("a0000002-0000-4000-8000-000000000002"),
			Slug:   "invalid-second-fac",
			Name:   "Invalid Status Facility",
			Status: "invalid_status_violates_chk", // Violates chk_facilities_status_valid!
		},
	}

	_, err := seed.ApplyFixtures(ctx, pool, invalidFixtures)
	if err == nil {
		t.Fatalf("expected constraint error, got nil")
	}

	// Verify that valid-first-fac was rolled back and does NOT exist
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.facilities;").Scan(&count); err != nil {
		t.Fatalf("failed to count facilities: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 facilities due to transaction rollback, got %d", count)
	}
}
