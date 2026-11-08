//go:build integration

package facility_test

import (
	"context"
	"database/sql"
	"encoding/json"
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
	"giki-sports-backend/internal/facility"
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

	dbName := fmt.Sprintf("giki_sports_disposable_facrepo_%d", time.Now().UnixNano())
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

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = maintenanceDB.ExecContext(cleanupCtx, fmt.Sprintf(`
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = '%s' AND pid <> pg_backend_pid();`, dbName))

	_, err := maintenanceDB.ExecContext(cleanupCtx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE);", dbName))
	return err
}


// TestFacilityRepository_EmptyDatabase verifies return behavior when no facilities exist.
func TestFacilityRepository_EmptyDatabase(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	// 1. List on empty database must return non-nil empty collection with nil error
	facilities, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("expected nil error on empty List, got: %v", err)
	}
	if facilities == nil {
		t.Fatalf("expected non-nil empty slice for empty database, got nil")
	}
	if len(facilities) != 0 {
		t.Fatalf("expected 0 facilities, got %d", len(facilities))
	}

	// 2. GetBySlug for unknown slug must return ErrNotFound
	fac, err := repo.GetBySlug(ctx, "non-existent-facility")
	if fac != nil {
		t.Fatalf("expected nil facility for unknown slug, got %+v", fac)
	}
	if !errors.Is(err, facility.ErrNotFound) {
		t.Fatalf("expected errors.Is(err, facility.ErrNotFound), got %v", err)
	}
}

// TestFacilityRepository_ListAndAssociations verifies:
// - Multiple facilities with multiple schedules
// - Facilities without schedules are still returned (non-nil empty schedules slice)
// - Correct association without duplicate parent facilities
// - Deterministic ordering (facilities by name ASC, slug ASC; schedules by day ASC, time ASC)
func TestFacilityRepository_ListAndAssociations(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	// Seed synthetic fixtures
	// Facility A: Padel Court (with 2 schedules)
	var idA string
	err := pool.QueryRow(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, description, location, status)
		VALUES ('padel-court-1', 'Arena Padel Court', 'Glass padel court', 'Outdoor North', 'active')
		RETURNING id;
	`).Scan(&idA)
	if err != nil {
		t.Fatalf("failed to insert facility A: %v", err)
	}

	// Facility B: Swimming Pool (with 0 schedules)
	var idB string
	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, description, location, status)
		VALUES ('olympic-pool', 'Campus Olympic Pool', NULL, NULL, 'maintenance')
		RETURNING id;
	`).Scan(&idB)
	if err != nil {
		t.Fatalf("failed to insert facility B: %v", err)
	}

	// Facility C: Badminton Hall (with 3 schedules)
	var idC string
	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, description, location, status)
		VALUES ('badminton-hall', 'Main Badminton Championship Hall', 'BWF court', 'Indoor Wing', 'active')
		RETURNING id;
	`).Scan(&idC)
	if err != nil {
		t.Fatalf("failed to insert facility C: %v", err)
	}

	// Schedules for Facility A:
	// Day 2 (Tue) 16:00-18:00, Day 1 (Mon) 08:00-11:00 (inserted out of order to verify ORDER BY)
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES 
			($1, 2, '16:00:00', '18:00:00', 'faculty'),
			($1, 1, '08:00:00', '11:00:00', 'student');
	`, idA)
	if err != nil {
		t.Fatalf("failed to insert schedules for facility A: %v", err)
	}

	// Schedules for Facility C:
	// Day 5 (Fri) 15:00-18:00, Day 3 (Wed) 10:00-12:00, Day 1 (Mon) 18:00-20:00
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES 
			($1, 5, '15:00:00', '18:00:00', 'student'),
			($1, 3, '10:00:00', '12:00:00', 'women_only'),
			($1, 1, '18:00:00', '20:00:00', 'general');
	`, idC)
	if err != nil {
		t.Fatalf("failed to insert schedules for facility C: %v", err)
	}

	// Execute List
	facilities, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("repo.List failed: %v", err)
	}

	// Assert exactly 3 facilities returned without duplicate parents
	if len(facilities) != 3 {
		t.Fatalf("expected 3 facilities, got %d", len(facilities))
	}

	// Assert deterministic facility ordering:
	// 'Arena Padel Court' -> 'Campus Olympic Pool' -> 'Main Badminton Championship Hall'
	expectedSlugs := []string{"padel-court-1", "olympic-pool", "badminton-hall"}
	for i, expected := range expectedSlugs {
		if facilities[i].Slug != expected {
			t.Errorf("expected facility [%d] slug to be %q, got %q", i, expected, facilities[i].Slug)
		}
	}

	// Assert Facility A schedules (2 schedules, ordered: Mon 08:00-11:00, Tue 16:00-18:00)
	facA := facilities[0]
	if len(facA.Schedules) != 2 {
		t.Fatalf("expected facility A to have 2 schedules, got %d", len(facA.Schedules))
	}
	if facA.Schedules[0].DayOfWeek != 1 || facA.Schedules[0].StartTime != "08:00:00" || facA.Schedules[0].EndTime != "11:00:00" {
		t.Errorf("unexpected schedule[0] for facility A: %+v", facA.Schedules[0])
	}
	if facA.Schedules[1].DayOfWeek != 2 || facA.Schedules[1].StartTime != "16:00:00" || facA.Schedules[1].EndTime != "18:00:00" {
		t.Errorf("unexpected schedule[1] for facility A: %+v", facA.Schedules[1])
	}

	// Assert Facility B (no schedules -> must return non-nil empty slice)
	facB := facilities[1]
	if facB.Schedules == nil {
		t.Errorf("expected non-nil schedules slice for facility B")
	}
	if len(facB.Schedules) != 0 {
		t.Errorf("expected 0 schedules for facility B, got %d", len(facB.Schedules))
	}

	// Assert Facility C schedules (3 schedules, ordered: Mon day 1, Wed day 3, Fri day 5)
	facC := facilities[2]
	if len(facC.Schedules) != 3 {
		t.Fatalf("expected facility C to have 3 schedules, got %d", len(facC.Schedules))
	}
	if facC.Schedules[0].DayOfWeek != 1 || facC.Schedules[1].DayOfWeek != 3 || facC.Schedules[2].DayOfWeek != 5 {
		t.Errorf("unexpected day ordering for facility C schedules: %+v", facC.Schedules)
	}
}

// TestFacilityRepository_GetBySlug_And_NullableFields verifies:
// - Exact slug lookup
// - Representation of non-null and NULL fields (description, location)
// - Correct error on unknown slug
func TestFacilityRepository_GetBySlug_And_NullableFields(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	// 1. Insert facility with full non-null fields
	var id1 string
	err := pool.QueryRow(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, description, location, status)
		VALUES ('tennis-court', 'Tennis Arena', 'US Open acrylic surface', 'South Perimeter Ground', 'active')
		RETURNING id;
	`).Scan(&id1)
	if err != nil {
		t.Fatalf("failed to insert facility 1: %v", err)
	}

	// 2. Insert facility with NULL description and NULL location
	var id2 string
	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, description, location, status)
		VALUES ('squash-courts', 'Glass Squash Courts', NULL, NULL, 'closed')
		RETURNING id;
	`).Scan(&id2)
	if err != nil {
		t.Fatalf("failed to insert facility 2: %v", err)
	}

	// Lookup facility 1
	f1, err := repo.GetBySlug(ctx, "tennis-court")
	if err != nil {
		t.Fatalf("GetBySlug for tennis-court failed: %v", err)
	}
	if f1.Slug != "tennis-court" || f1.Name != "Tennis Arena" {
		t.Errorf("unexpected facility 1 attributes: %+v", f1)
	}
	if f1.Description == nil || *f1.Description != "US Open acrylic surface" {
		t.Errorf("expected non-null description, got %v", f1.Description)
	}
	if f1.Location == nil || *f1.Location != "South Perimeter Ground" {
		t.Errorf("expected non-null location, got %v", f1.Location)
	}
	if f1.Status != facility.StatusActive {
		t.Errorf("expected status 'active', got %v", f1.Status)
	}

	// Lookup facility 2 (nullable fields must be nil pointers)
	f2, err := repo.GetBySlug(ctx, "squash-courts")
	if err != nil {
		t.Fatalf("GetBySlug for squash-courts failed: %v", err)
	}
	if f2.Description != nil {
		t.Errorf("expected nil description for squash-courts, got %q", *f2.Description)
	}
	if f2.Location != nil {
		t.Errorf("expected nil location for squash-courts, got %q", *f2.Location)
	}
	if f2.Status != facility.StatusClosed {
		t.Errorf("expected status 'closed', got %v", f2.Status)
	}

	// Lookup unknown slug
	fUnknown, err := repo.GetBySlug(ctx, "does-not-exist")
	if fUnknown != nil {
		t.Errorf("expected nil facility for unknown slug, got %+v", fUnknown)
	}
	if !errors.Is(err, facility.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestFacilityRepository_EndTime24AndOvernightWindow verifies:
// - End time '24:00:00' is preserved without truncation, date attachment, or wrapping to '00:00:00'
// - Overnight window represented by two daily rows (Saturday evening + Sunday morning)
func TestFacilityRepository_EndTime24AndOvernightWindow(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	var facilityID string
	err := pool.QueryRow(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, description, location, status)
		VALUES ('night-futsal', 'Night Futsal Arena', 'Floodlit turf', 'Central Courtyard', 'active')
		RETURNING id;
	`).Scan(&facilityID)
	if err != nil {
		t.Fatalf("failed to insert facility: %v", err)
	}

	// Overnight schedule:
	// Window 1: Saturday (day 6), 22:00:00 to 24:00:00
	// Window 2: Sunday (day 7), 00:00:00 to 02:00:00
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES 
			($1, 6, '22:00:00', '24:00:00', 'general'),
			($1, 7, '00:00:00', '02:00:00', 'general');
	`, facilityID)
	if err != nil {
		t.Fatalf("failed to insert overnight schedule rows: %v", err)
	}

	fac, err := repo.GetBySlug(ctx, "night-futsal")
	if err != nil {
		t.Fatalf("GetBySlug failed: %v", err)
	}

	if len(fac.Schedules) != 2 {
		t.Fatalf("expected 2 schedule rows for overnight window, got %d", len(fac.Schedules))
	}

	// Check Saturday window: EndTime must be preserved exactly as "24:00:00"
	satWindow := fac.Schedules[0]
	if satWindow.DayOfWeek != 6 {
		t.Errorf("expected day 6 (Saturday), got %d", satWindow.DayOfWeek)
	}
	if satWindow.StartTime != "22:00:00" {
		t.Errorf("expected StartTime '22:00:00', got %q", satWindow.StartTime)
	}
	if satWindow.EndTime != "24:00:00" {
		t.Errorf("CRITICAL ERROR: expected EndTime '24:00:00', got %q", satWindow.EndTime)
	}

	// Check Sunday window: StartTime "00:00:00", EndTime "02:00:00"
	sunWindow := fac.Schedules[1]
	if sunWindow.DayOfWeek != 7 {
		t.Errorf("expected day 7 (Sunday), got %d", sunWindow.DayOfWeek)
	}
	if sunWindow.StartTime != "00:00:00" {
		t.Errorf("expected StartTime '00:00:00', got %q", sunWindow.StartTime)
	}
	if sunWindow.EndTime != "02:00:00" {
		t.Errorf("expected EndTime '02:00:00', got %q", sunWindow.EndTime)
	}
}

// TestFacilityRepository_SQLInjectionSafety verifies that SQL-like slug inputs are treated strictly as data.
func TestFacilityRepository_SQLInjectionSafety(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	// Seed one valid facility
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, status)
		VALUES ('valid-facility', 'Valid Facility', 'active');
	`)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	sqlPayloads := []string{
		"' OR '1'='1",
		"valid-facility' OR '1'='1",
		"'; DROP TABLE giki_sports.facilities; --",
		"admin'--",
		"UNION SELECT id, slug, name, NULL, NULL, 'active', now(), now() FROM giki_sports.facilities--",
	}

	for _, payload := range sqlPayloads {
		fac, err := repo.GetBySlug(ctx, payload)
		if fac != nil {
			t.Errorf("SECURITY VULNERABILITY: SQL injection payload %q returned facility: %+v", payload, fac)
		}
		if !errors.Is(err, facility.ErrNotFound) {
			t.Errorf("expected ErrNotFound for payload %q, got: %v", payload, err)
		}
	}

	// Verify table still exists and contains the valid facility
	fac, err := repo.GetBySlug(ctx, "valid-facility")
	if err != nil || fac == nil {
		t.Fatalf("valid facility unexpectedly missing after SQL payload tests: %v", err)
	}
}

// TestFacilityRepository_CanceledContext verifies that repository calls honor context cancellation.
func TestFacilityRepository_CanceledContext(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel context immediately

	// List with canceled context must fail
	facilities, err := repo.List(canceledCtx)
	if err == nil {
		t.Errorf("expected error calling List with canceled context, got nil")
	}
	if facilities != nil {
		t.Errorf("expected nil slice on canceled context, got %v", facilities)
	}

	// GetBySlug with canceled context must fail
	fac, err := repo.GetBySlug(canceledCtx, "any-slug")
	if err == nil {
		t.Errorf("expected error calling GetBySlug with canceled context, got nil")
	}
	if fac != nil {
		t.Errorf("expected nil facility on canceled context, got %v", fac)
	}
}

// -----------------------------------------------------------------------------
// Step 7B: UpdateSchedule Integration Tests
// -----------------------------------------------------------------------------

// TestFacilityRepository_UpdateSchedule_SyntheticRowAndTimetable verifies:
// - A synthetic schedule row can be updated to new valid timetable parameters
// - The returned *Schedule preserves id, facility_id, and created_at
// - updated_at is refreshed
// - The read repository (GetBySlug) returns the new timetable immediately
func TestFacilityRepository_UpdateSchedule_SyntheticRowAndTimetable(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	// Seed synthetic facility
	var facID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, status)
		VALUES ('badminton-hall', 'Main Badminton Hall', 'active')
		RETURNING id;
	`).Scan(&facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	// Seed synthetic schedule: Monday 09:00:00 to 12:00:00 (student)
	var schedID uuid.UUID
	var origCreatedAt, origUpdatedAt time.Time
	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 1, '09:00:00', '12:00:00', 'student')
		RETURNING id, created_at, updated_at;
	`, facID).Scan(&schedID, &origCreatedAt, &origUpdatedAt)
	if err != nil {
		t.Fatalf("failed to seed schedule: %v", err)
	}

	// Small pause so updated_at can be strictly after created_at
	time.Sleep(10 * time.Millisecond)

	// Update schedule to Tuesday 10:00:00 to 13:30:00 (faculty)
	updated, err := repo.UpdateSchedule(ctx, "badminton-hall", schedID, facility.UpdateScheduleInput{
		DayOfWeek: 2,
		StartTime: "10:00:00",
		EndTime:   "13:30:00",
		Audience:  "faculty",
	})
	if err != nil {
		t.Fatalf("UpdateSchedule failed unexpectedly: %v", err)
	}

	// 1. Verify returned schedule attributes
	if updated.ID != schedID {
		t.Errorf("expected schedule ID %s, got %s", schedID, updated.ID)
	}
	if updated.FacilityID != facID {
		t.Errorf("expected facility ID %s, got %s", facID, updated.FacilityID)
	}
	if updated.DayOfWeek != 2 {
		t.Errorf("expected dayOfWeek 2, got %d", updated.DayOfWeek)
	}
	if updated.StartTime != "10:00:00" {
		t.Errorf("expected startTime '10:00:00', got %q", updated.StartTime)
	}
	if updated.EndTime != "13:30:00" {
		t.Errorf("expected endTime '13:30:00', got %q", updated.EndTime)
	}
	if updated.Audience != "faculty" {
		t.Errorf("expected audience 'faculty', got %q", updated.Audience)
	}
	if !updated.CreatedAt.Equal(origCreatedAt) {
		t.Errorf("created_at was mutated: expected %v, got %v", origCreatedAt, updated.CreatedAt)
	}
	if !updated.UpdatedAt.After(origCreatedAt) {
		t.Errorf("updated_at was not updated: expected > %v, got %v", origCreatedAt, updated.UpdatedAt)
	}

	// 2. Confirm existing read repository returns the updated timetable
	readFac, err := repo.GetBySlug(ctx, "badminton-hall")
	if err != nil {
		t.Fatalf("failed to read facility after update: %v", err)
	}
	if len(readFac.Schedules) != 1 {
		t.Fatalf("expected 1 schedule in timetable, got %d", len(readFac.Schedules))
	}
	s := readFac.Schedules[0]
	if s.ID != schedID || s.DayOfWeek != 2 || s.StartTime != "10:00:00" || s.EndTime != "13:30:00" || s.Audience != "faculty" {
		t.Errorf("read timetable does not match updated schedule: %+v", s)
	}
}

// TestFacilityRepository_UpdateSchedule_PreservesUnrelatedRows verifies:
// - Updating one schedule does not modify unrelated schedules under the same facility
// - Does not modify schedules belonging to other facilities
// - Does not modify facility attributes
func TestFacilityRepository_UpdateSchedule_PreservesUnrelatedRows(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	// Facility A with two schedules (A1 and A2)
	var facA_ID uuid.UUID
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('fac-a', 'Facility A', 'active') RETURNING id;").Scan(&facA_ID)

	var schedA1_ID, schedA2_ID uuid.UUID
	var a2CreatedAt, a2UpdatedAt time.Time
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience) VALUES ($1, 1, '08:00:00', '10:00:00', 'student') RETURNING id;", facA_ID).Scan(&schedA1_ID)
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience) VALUES ($1, 1, '14:00:00', '16:00:00', 'student') RETURNING id, created_at, updated_at;", facA_ID).Scan(&schedA2_ID, &a2CreatedAt, &a2UpdatedAt)

	// Facility B with one schedule (B1)
	var facB_ID, schedB1_ID uuid.UUID
	var b1CreatedAt, b1UpdatedAt time.Time
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('fac-b', 'Facility B', 'active') RETURNING id;").Scan(&facB_ID)
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience) VALUES ($1, 3, '18:00:00', '20:00:00', 'general') RETURNING id, created_at, updated_at;", facB_ID).Scan(&schedB1_ID, &b1CreatedAt, &b1UpdatedAt)

	// Update Schedule A1
	_, err := repo.UpdateSchedule(ctx, "fac-a", schedA1_ID, facility.UpdateScheduleInput{
		DayOfWeek: 5,
		StartTime: "09:00:00",
		EndTime:   "11:00:00",
		Audience:  "faculty",
	})
	if err != nil {
		t.Fatalf("failed to update schedule A1: %v", err)
	}

	// Verify Schedule A2 is completely untouched
	var a2Day int
	var a2Start, a2End, a2Aud string
	var a2CurrUpdated time.Time
	err = pool.QueryRow(ctx, "SELECT day_of_week, to_char(start_time, 'HH24:MI:SS'), to_char(end_time, 'HH24:MI:SS'), audience, updated_at FROM giki_sports.schedules WHERE id = $1;", schedA2_ID).
		Scan(&a2Day, &a2Start, &a2End, &a2Aud, &a2CurrUpdated)
	if err != nil {
		t.Fatalf("failed to query schedule A2: %v", err)
	}
	if a2Day != 1 || a2Start != "14:00:00" || a2End != "16:00:00" || a2Aud != "student" || !a2CurrUpdated.Equal(a2UpdatedAt) {
		t.Errorf("unrelated schedule A2 was mutated: day=%d, start=%s, end=%s, aud=%s, updated=%v", a2Day, a2Start, a2End, a2Aud, a2CurrUpdated)
	}

	// Verify Schedule B1 is completely untouched
	var b1Day int
	var b1Start, b1End, b1Aud string
	var b1CurrUpdated time.Time
	err = pool.QueryRow(ctx, "SELECT day_of_week, to_char(start_time, 'HH24:MI:SS'), to_char(end_time, 'HH24:MI:SS'), audience, updated_at FROM giki_sports.schedules WHERE id = $1;", schedB1_ID).
		Scan(&b1Day, &b1Start, &b1End, &b1Aud, &b1CurrUpdated)
	if err != nil {
		t.Fatalf("failed to query schedule B1: %v", err)
	}
	if b1Day != 3 || b1Start != "18:00:00" || b1End != "20:00:00" || b1Aud != "general" || !b1CurrUpdated.Equal(b1UpdatedAt) {
		t.Errorf("unrelated schedule B1 was mutated: day=%d, start=%s, end=%s, aud=%s, updated=%v", b1Day, b1Start, b1End, b1Aud, b1CurrUpdated)
	}
}

// TestFacilityRepository_UpdateSchedule_MissingOrMismatchedPair verifies:
// - Unknown facility slug returns ErrScheduleNotFound
// - Unknown schedule ID returns ErrScheduleNotFound
// - Mismatched pair (valid facility slug, but schedule belongs to another facility) returns ErrScheduleNotFound
// - Zero rows are modified on mismatch
func TestFacilityRepository_UpdateSchedule_MissingOrMismatchedPair(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	// Seed Facility A and Facility B
	var facA_ID, facB_ID, schedB_ID uuid.UUID
	var bOrigUpdatedAt time.Time
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('fac-a', 'Facility A', 'active') RETURNING id;").Scan(&facA_ID)
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('fac-b', 'Facility B', 'active') RETURNING id;").Scan(&facB_ID)
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience) VALUES ($1, 2, '10:00:00', '12:00:00', 'student') RETURNING id, updated_at;", facB_ID).Scan(&schedB_ID, &bOrigUpdatedAt)

	validInput := facility.UpdateScheduleInput{
		DayOfWeek: 4,
		StartTime: "15:00:00",
		EndTime:   "17:00:00",
		Audience:  "faculty",
	}

	// 1. Unknown facility slug with existing schedule ID
	_, err := repo.UpdateSchedule(ctx, "unknown-facility", schedB_ID, validInput)
	if !errors.Is(err, facility.ErrScheduleNotFound) {
		t.Errorf("expected ErrScheduleNotFound for unknown facility slug, got: %v", err)
	}

	// 2. Existing facility slug with non-existent schedule ID
	_, err = repo.UpdateSchedule(ctx, "fac-a", uuid.New(), validInput)
	if !errors.Is(err, facility.ErrScheduleNotFound) {
		t.Errorf("expected ErrScheduleNotFound for non-existent schedule ID, got: %v", err)
	}

	// 3. Mismatched pair: facility slug 'fac-a' with schedule ID belonging to 'fac-b'
	_, err = repo.UpdateSchedule(ctx, "fac-a", schedB_ID, validInput)
	if !errors.Is(err, facility.ErrScheduleNotFound) {
		t.Errorf("expected ErrScheduleNotFound for mismatched facility/schedule pair, got: %v", err)
	}

	// 4. Verify schedule B was NOT modified
	var bDay int
	var bCurrUpdated time.Time
	_ = pool.QueryRow(ctx, "SELECT day_of_week, updated_at FROM giki_sports.schedules WHERE id = $1;", schedB_ID).Scan(&bDay, &bCurrUpdated)
	if bDay != 2 || !bCurrUpdated.Equal(bOrigUpdatedAt) {
		t.Errorf("schedule B was mutated despite mismatched slug: day=%d, updated_at=%v", bDay, bCurrUpdated)
	}
}

// TestFacilityRepository_UpdateSchedule_InvalidAndDuplicateWindows verifies:
// - Validation failures leave the existing row intact
// - Exact duplicate window updates violate unique constraint and leave the row intact
func TestFacilityRepository_UpdateSchedule_InvalidAndDuplicateWindows(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	var facID, sched1ID, sched2ID uuid.UUID
	var s2OrigUpdated time.Time
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('tennis-court', 'Tennis Court', 'active') RETURNING id;").Scan(&facID)
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience) VALUES ($1, 1, '09:00:00', '12:00:00', 'student') RETURNING id;", facID).Scan(&sched1ID)
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience) VALUES ($1, 1, '14:00:00', '17:00:00', 'student') RETURNING id, updated_at;", facID).Scan(&sched2ID, &s2OrigUpdated)

	// 1. Invalid window (start_time >= end_time)
	_, err := repo.UpdateSchedule(ctx, "tennis-court", sched2ID, facility.UpdateScheduleInput{
		DayOfWeek: 1,
		StartTime: "16:00:00",
		EndTime:   "12:00:00", // Start after end!
		Audience:  "student",
	})
	if !errors.Is(err, facility.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got: %v", err)
	}

	// 2. Verify Schedule 2 remains intact after invalid update
	var s2Start string
	_ = pool.QueryRow(ctx, "SELECT to_char(start_time, 'HH24:MI:SS') FROM giki_sports.schedules WHERE id = $1;", sched2ID).Scan(&s2Start)
	if s2Start != "14:00:00" {
		t.Errorf("schedule 2 mutated after invalid update: start_time=%s", s2Start)
	}

	// 3. Exact duplicate window (attempt to update Schedule 2 to match Schedule 1's window exactly)
	_, err = repo.UpdateSchedule(ctx, "tennis-court", sched2ID, facility.UpdateScheduleInput{
		DayOfWeek: 1,
		StartTime: "09:00:00",
		EndTime:   "12:00:00",
		Audience:  "student",
	})
	if !errors.Is(err, facility.ErrDuplicateSchedule) {
		t.Errorf("expected ErrDuplicateSchedule for collision, got: %v", err)
	}

	// 4. Verify Schedule 2 still remains intact after duplicate collision
	var s2CurrUpdated time.Time
	_ = pool.QueryRow(ctx, "SELECT to_char(start_time, 'HH24:MI:SS'), updated_at FROM giki_sports.schedules WHERE id = $1;", sched2ID).Scan(&s2Start, &s2CurrUpdated)
	if s2Start != "14:00:00" || !s2CurrUpdated.Equal(s2OrigUpdated) {
		t.Errorf("schedule 2 mutated after duplicate collision: start=%s, updated=%v", s2Start, s2CurrUpdated)
	}
}

// TestFacilityRepository_UpdateSchedule_EndTime24RoundTrip verifies:
// - Updating a schedule window with end_time '24:00:00' succeeds
// - Survives the read repository round-trip without date or timezone corruption
func TestFacilityRepository_UpdateSchedule_EndTime24RoundTrip(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	var facID, schedID uuid.UUID
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('pool-arena', 'Swimming Pool', 'active') RETURNING id;").Scan(&facID)
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience) VALUES ($1, 6, '16:00:00', '20:00:00', 'general') RETURNING id;", facID).Scan(&schedID)

	// Update to end at 24:00:00
	updated, err := repo.UpdateSchedule(ctx, "pool-arena", schedID, facility.UpdateScheduleInput{
		DayOfWeek: 6,
		StartTime: "18:00:00",
		EndTime:   "24:00:00",
		Audience:  "general",
	})
	if err != nil {
		t.Fatalf("failed to update schedule to 24:00:00: %v", err)
	}
	if updated.EndTime != "24:00:00" {
		t.Errorf("expected returned endTime '24:00:00', got %q", updated.EndTime)
	}

	// Verify via GetBySlug read repository
	fac, err := repo.GetBySlug(ctx, "pool-arena")
	if err != nil {
		t.Fatalf("GetBySlug failed: %v", err)
	}
	if len(fac.Schedules) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(fac.Schedules))
	}
	if fac.Schedules[0].EndTime != "24:00:00" {
		t.Errorf("read repository returned %q instead of '24:00:00'", fac.Schedules[0].EndTime)
	}
}

// TestFacilityRepository_UpdateScheduleWithAudit_Success verifies:
// - A schedule update and its administrative audit log commit together in a single transaction.
// - giki_sports.schedules reflects the new window parameters and updated_at.
// - giki_sports.audit_logs records actor, action, target, IP, user-agent, and JSON details with previous and new states.
func TestFacilityRepository_UpdateScheduleWithAudit_Success(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	// Seed actor in giki_sports.users
	actorID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'director@giki.edu.pk', 'Sports Director', '$argon2id$v=19$m=65536,t=3,p=4$dummy$dummy');
	`, actorID)
	if err != nil {
		t.Fatalf("failed to insert actor user: %v", err)
	}

	// Seed facility and initial schedule
	var facID, schedID uuid.UUID
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('audit-court', 'Audit Badminton Court', 'active') RETURNING id;").Scan(&facID)
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience) VALUES ($1, 1, '09:00:00', '11:00:00', 'student') RETURNING id;", facID).Scan(&schedID)

	updateInput := facility.UpdateScheduleInput{
		DayOfWeek: 3,
		StartTime: "14:00:00",
		EndTime:   "16:30:00",
		Audience:  "women_only",
	}

	auditParams := facility.AuditParams{
		ActorID:   actorID,
		IPAddress: "192.168.1.100",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
	}

	res, err := repo.UpdateScheduleWithAudit(ctx, "audit-court", schedID, updateInput, auditParams)
	if err != nil {
		t.Fatalf("UpdateScheduleWithAudit failed: %v", err)
	}

	if res.ID != schedID {
		t.Errorf("expected schedule ID %s, got %s", schedID, res.ID)
	}
	if res.DayOfWeek != 3 || res.StartTime != "14:00:00" || res.EndTime != "16:30:00" || res.Audience != "women_only" {
		t.Errorf("returned schedule does not match updated values: %+v", res)
	}

	// Verify database schedule row
	var dbDay int
	var dbStart, dbEnd, dbAudience string
	err = pool.QueryRow(ctx, `
		SELECT day_of_week, to_char(start_time, 'HH24:MI:SS'), to_char(end_time, 'HH24:MI:SS'), audience
		FROM giki_sports.schedules
		WHERE id = $1;
	`, schedID).Scan(&dbDay, &dbStart, &dbEnd, &dbAudience)
	if err != nil {
		t.Fatalf("failed to query schedule row: %v", err)
	}
	if dbDay != 3 || dbStart != "14:00:00" || dbEnd != "16:30:00" || dbAudience != "women_only" {
		t.Errorf("schedule table does not reflect update: day=%d, start=%s, end=%s, audience=%s", dbDay, dbStart, dbEnd, dbAudience)
	}

	// Verify database audit_logs row
	var auditID, dbActorID, action, targetType, status, ipAddress, userAgent string
	var dbTargetID uuid.UUID
	var detailsBytes []byte
	err = pool.QueryRow(ctx, `
		SELECT id, actor_id, action, target_type, target_id, details, ip_address, user_agent, status
		FROM giki_sports.audit_logs
		WHERE target_id = $1;
	`, schedID).Scan(&auditID, &dbActorID, &action, &targetType, &dbTargetID, &detailsBytes, &ipAddress, &userAgent, &status)
	if err != nil {
		t.Fatalf("failed to query audit log row: %v", err)
	}

	if dbActorID != actorID.String() {
		t.Errorf("expected audit actor_id %s, got %s", actorID, dbActorID)
	}
	if action != "schedule.update" {
		t.Errorf("expected action 'schedule.update', got %q", action)
	}
	if targetType != "facility_schedule" {
		t.Errorf("expected target_type 'facility_schedule', got %q", targetType)
	}
	if dbTargetID != schedID {
		t.Errorf("expected target_id %s, got %s", schedID, dbTargetID)
	}
	if status != "SUCCESS" {
		t.Errorf("expected status 'SUCCESS', got %q", status)
	}
	if ipAddress != "192.168.1.100" {
		t.Errorf("expected ip_address '192.168.1.100', got %q", ipAddress)
	}
	if userAgent != "Mozilla/5.0 (Windows NT 10.0; Win64; x64)" {
		t.Errorf("expected user_agent 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)', got %q", userAgent)
	}

	// Verify audit details contents (old_window and new_window)
	var details map[string]interface{}
	if err := json.Unmarshal(detailsBytes, &details); err != nil {
		t.Fatalf("failed to unmarshal audit details JSON: %v", err)
	}

	if details["facility_slug"] != "audit-court" {
		t.Errorf("expected facility_slug 'audit-court', got %v", details["facility_slug"])
	}
	if details["facility_id"] != facID.String() {
		t.Errorf("expected facility_id %s, got %v", facID.String(), details["facility_id"])
	}

	oldWindow, ok := details["old_window"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected old_window to be a map, got %T", details["old_window"])
	}
	if int(oldWindow["day_of_week"].(float64)) != 1 || oldWindow["start_time"] != "09:00:00" ||
		oldWindow["end_time"] != "11:00:00" || oldWindow["audience"] != "student" {
		t.Errorf("old_window does not match expected values: %+v", oldWindow)
	}

	newWindow, ok := details["new_window"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected new_window to be a map, got %T", details["new_window"])
	}
	if int(newWindow["day_of_week"].(float64)) != 3 || newWindow["start_time"] != "14:00:00" ||
		newWindow["end_time"] != "16:30:00" || newWindow["audience"] != "women_only" {
		t.Errorf("new_window does not match expected values: %+v", newWindow)
	}
}

// TestFacilityRepository_UpdateScheduleWithAudit_RollbackOnAuditFailure verifies:
// - Step 7C core requirement: when the audit insert fails (e.g. non-existent actor_id violating foreign key),
//   the entire transaction is rolled back.
// - The schedule row in giki_sports.schedules remains completely unchanged (timestamps and window untouched).
// - Zero audit log entries are committed.
func TestFacilityRepository_UpdateScheduleWithAudit_RollbackOnAuditFailure(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	// Seed facility and initial schedule
	var facID, schedID uuid.UUID
	var origUpdatedAt time.Time
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('squash-arena', 'Squash Arena', 'active') RETURNING id;").Scan(&facID)
	_ = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience) 
		VALUES ($1, 2, '10:00:00', '12:00:00', 'student') 
		RETURNING id, updated_at;
	`, facID).Scan(&schedID, &origUpdatedAt)

	// Proposed update that would change the schedule
	updateInput := facility.UpdateScheduleInput{
		DayOfWeek: 5,
		StartTime: "18:00:00",
		EndTime:   "21:00:00",
		Audience:  "faculty",
	}

	// Intentionally provide a non-existent ActorID to trigger foreign key violation fk_audit_logs_actor
	nonExistentActorID := uuid.New()
	auditParams := facility.AuditParams{
		ActorID:   nonExistentActorID,
		IPAddress: "10.0.0.1",
		UserAgent: "Test-Harness",
	}

	// Attempt atomic update
	res, err := repo.UpdateScheduleWithAudit(ctx, "squash-arena", schedID, updateInput, auditParams)
	if err == nil {
		t.Fatalf("expected UpdateScheduleWithAudit to fail due to foreign key violation on audit log, but got nil error")
	}
	if res != nil {
		t.Fatalf("expected nil result on failure, got %+v", res)
	}

	if !strings.Contains(err.Error(), "failed to write audit log") {
		t.Errorf("expected error message to mention audit log failure, got: %v", err)
	}
	t.Logf("Received expected audit insert failure: %v", err)

	// Verify that the schedule window in giki_sports.schedules is 100% UNCHANGED
	var dbDay int
	var dbStart, dbEnd, dbAudience string
	var dbUpdatedAt time.Time
	err = pool.QueryRow(ctx, `
		SELECT day_of_week, to_char(start_time, 'HH24:MI:SS'), to_char(end_time, 'HH24:MI:SS'), audience, updated_at
		FROM giki_sports.schedules
		WHERE id = $1;
	`, schedID).Scan(&dbDay, &dbStart, &dbEnd, &dbAudience, &dbUpdatedAt)
	if err != nil {
		t.Fatalf("failed to query schedule after failed transaction: %v", err)
	}

	if dbDay != 2 || dbStart != "10:00:00" || dbEnd != "12:00:00" || dbAudience != "student" {
		t.Errorf("CRITICAL ATOMICITY VIOLATION: schedule row was mutated despite audit failure! day=%d, start=%s, end=%s, audience=%s",
			dbDay, dbStart, dbEnd, dbAudience)
	}
	if !dbUpdatedAt.Equal(origUpdatedAt) {
		t.Errorf("CRITICAL ATOMICITY VIOLATION: schedule updated_at changed from %v to %v despite audit failure!",
			origUpdatedAt, dbUpdatedAt)
	}
	t.Log("Verified: schedule row and updated_at timestamp stayed 100% untouched after audit failure.")

	// Verify that zero audit logs exist
	var auditCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
	if err != nil {
		t.Fatalf("failed to count audit logs: %v", err)
	}
	if auditCount != 0 {
		t.Errorf("expected 0 audit logs in database, found %d", auditCount)
	}
	t.Log("Verified: 0 audit logs inserted (transaction cleanly rolled back).")
}

// TestFacilityRepository_UpdateScheduleWithAudit_RollbackOnScheduleError verifies:
// - If the schedule update fails (e.g. duplicate window collision), no audit log is written.
func TestFacilityRepository_UpdateScheduleWithAudit_RollbackOnScheduleError(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	// Seed actor in giki_sports.users
	actorID := uuid.New()
	_, _ = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'admin@giki.edu.pk', 'Admin User', '$argon2id$v=19$m=65536,t=3,p=4$dummy$dummy');
	`, actorID)

	var facID, s1ID, s2ID uuid.UUID
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('table-tennis', 'Table Tennis Hall', 'active') RETURNING id;").Scan(&facID)
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience) VALUES ($1, 1, '09:00:00', '12:00:00', 'student') RETURNING id;", facID).Scan(&s1ID)
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience) VALUES ($1, 1, '14:00:00', '17:00:00', 'student') RETURNING id;", facID).Scan(&s2ID)

	// Attempt to update s2 to collide with s1
	_, err := repo.UpdateScheduleWithAudit(ctx, "table-tennis", s2ID, facility.UpdateScheduleInput{
		DayOfWeek: 1,
		StartTime: "09:00:00",
		EndTime:   "12:00:00",
		Audience:  "student",
	}, facility.AuditParams{ActorID: actorID})

	if !errors.Is(err, facility.ErrDuplicateSchedule) {
		t.Fatalf("expected ErrDuplicateSchedule, got: %v", err)
	}

	// Verify no audit log was created
	var auditCount int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
	if auditCount != 0 {
		t.Errorf("expected 0 audit logs after duplicate schedule collision, found %d", auditCount)
	}
}

// TestFacilityRepository_UpdateScheduleWithAudit_NotFound verifies:
// - Non-existent facility slug or schedule ID returns ErrScheduleNotFound without touching audit logs.
func TestFacilityRepository_UpdateScheduleWithAudit_NotFound(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	actorID := uuid.New()
	_, _ = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'admin2@giki.edu.pk', 'Admin Two', '$argon2id$v=19$m=65536,t=3,p=4$dummy$dummy');
	`, actorID)

	_, err := repo.UpdateScheduleWithAudit(ctx, "nonexistent-slug", uuid.New(), facility.UpdateScheduleInput{
		DayOfWeek: 1,
		StartTime: "09:00:00",
		EndTime:   "12:00:00",
		Audience:  "student",
	}, facility.AuditParams{ActorID: actorID})

	if !errors.Is(err, facility.ErrScheduleNotFound) {
		t.Fatalf("expected ErrScheduleNotFound, got: %v", err)
	}

	var auditCount int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
	if auditCount != 0 {
		t.Errorf("expected 0 audit logs on not found, found %d", auditCount)
	}
}

// TestFacilityRepository_DeleteScheduleWithAudit_Success verifies:
// - Step 8A core requirement: single transaction deletes the selected schedule row and creates
//   a matching 'schedule.delete' audit log entry capturing previous values.
// - Only the selected schedule row is removed; other schedules belonging to the facility remain intact.
// - The audit log entry remains fully intact and readable after the schedule row is gone.
func TestFacilityRepository_DeleteScheduleWithAudit_Success(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	// Seed actor in giki_sports.users
	actorID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'director@giki.edu.pk', 'Sports Director', '$argon2id$v=19$m=65536,t=3,p=4$dummy$dummy');
	`, actorID)
	if err != nil {
		t.Fatalf("failed to insert actor: %v", err)
	}

	// Seed facility
	var facID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, description, location, status)
		VALUES ('del-badminton', 'Delete Badminton Court', 'Test Arena', 'Hall B', 'active')
		RETURNING id;
	`).Scan(&facID)
	if err != nil {
		t.Fatalf("failed to insert facility: %v", err)
	}

	// Seed two schedules: s1 to be deleted, s2 to be retained
	var s1ID, s2ID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 1, '09:00:00', '12:00:00', 'student')
		RETURNING id;
	`, facID).Scan(&s1ID)
	if err != nil {
		t.Fatalf("failed to insert schedule 1: %v", err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 3, '14:00:00', '16:00:00', 'faculty')
		RETURNING id;
	`, facID).Scan(&s2ID)
	if err != nil {
		t.Fatalf("failed to insert schedule 2: %v", err)
	}

	auditParams := facility.AuditParams{
		ActorID:   actorID,
		IPAddress: "192.168.1.50",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) SportsTest",
	}

	// Execute audited deletion
	err = repo.DeleteScheduleWithAudit(ctx, "del-badminton", s1ID, auditParams)
	if err != nil {
		t.Fatalf("DeleteScheduleWithAudit failed: %v", err)
	}

	// Verify s1 is deleted from giki_sports.schedules
	var s1Count int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.schedules WHERE id = $1;", s1ID).Scan(&s1Count)
	if err != nil {
		t.Fatalf("failed to query s1 count: %v", err)
	}
	if s1Count != 0 {
		t.Errorf("expected schedule %s to be deleted, but found %d rows", s1ID, s1Count)
	}

	// Verify s2 remains untouched in giki_sports.schedules
	var s2Count int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.schedules WHERE id = $1;", s2ID).Scan(&s2Count)
	if err != nil {
		t.Fatalf("failed to query s2 count: %v", err)
	}
	if s2Count != 1 {
		t.Errorf("expected schedule %s to remain intact, but found %d rows", s2ID, s2Count)
	}

	// Verify exactly 1 audit log entry was created
	var auditCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
	if err != nil {
		t.Fatalf("failed to count audit logs: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected exactly 1 audit log, found %d", auditCount)
	}

	// Verify audit log record fields
	var auditID, dbActorID, dbTargetID uuid.UUID
	var action, targetType, status, ipAddress, userAgent string
	var detailsBytes []byte
	var createdAt time.Time
	err = pool.QueryRow(ctx, `
		SELECT id, actor_id, action, target_type, target_id, details, ip_address, user_agent, status, created_at
		FROM giki_sports.audit_logs
		WHERE target_id = $1;
	`, s1ID).Scan(
		&auditID,
		&dbActorID,
		&action,
		&targetType,
		&dbTargetID,
		&detailsBytes,
		&ipAddress,
		&userAgent,
		&status,
		&createdAt,
	)
	if err != nil {
		t.Fatalf("failed to query audit log for deleted schedule: %v", err)
	}

	if dbActorID != actorID {
		t.Errorf("expected actor_id %s, got %s", actorID, dbActorID)
	}
	if action != "schedule.delete" {
		t.Errorf("expected action 'schedule.delete', got %q", action)
	}
	if targetType != "facility_schedule" {
		t.Errorf("expected target_type 'facility_schedule', got %q", targetType)
	}
	if dbTargetID != s1ID {
		t.Errorf("expected target_id %s, got %s", s1ID, dbTargetID)
	}
	if status != "SUCCESS" {
		t.Errorf("expected status 'SUCCESS', got %q", status)
	}
	if ipAddress != "192.168.1.50" {
		t.Errorf("expected ip_address '192.168.1.50', got %q", ipAddress)
	}
	if userAgent != "Mozilla/5.0 (Windows NT 10.0; Win64; x64) SportsTest" {
		t.Errorf("expected user_agent 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) SportsTest', got %q", userAgent)
	}

	// Verify that audit log details preserves previous schedule values and is completely readable after deletion
	var details map[string]interface{}
	if err := json.Unmarshal(detailsBytes, &details); err != nil {
		t.Fatalf("failed to unmarshal audit details JSON: %v", err)
	}

	if details["facility_slug"] != "del-badminton" {
		t.Errorf("expected facility_slug 'del-badminton', got %v", details["facility_slug"])
	}
	if details["facility_id"] != facID.String() {
		t.Errorf("expected facility_id %s, got %v", facID.String(), details["facility_id"])
	}

	oldWindow, ok := details["old_window"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected old_window in audit details, got %T", details["old_window"])
	}
	if int(oldWindow["day_of_week"].(float64)) != 1 ||
		oldWindow["start_time"] != "09:00:00" ||
		oldWindow["end_time"] != "12:00:00" ||
		oldWindow["audience"] != "student" {
		t.Errorf("old_window parameters do not match deleted schedule: %+v", oldWindow)
	}

	t.Log("Verified: schedule deleted, remaining schedules untouched, and audit record permanently readable.")
}

// TestFacilityRepository_DeleteScheduleWithAudit_MismatchedAndNotFound verifies:
// - Mismatched facility and schedule returns ErrScheduleNotFound and changes nothing.
// - Non-existent facility slug returns ErrScheduleNotFound and changes nothing.
// - Non-existent schedule ID returns ErrScheduleNotFound and changes nothing.
// - Empty slug or nil schedule ID returns ErrScheduleNotFound.
// - 0 audit logs are written across all rejected attempts.
func TestFacilityRepository_DeleteScheduleWithAudit_MismatchedAndNotFound(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	actorID := uuid.New()
	_, _ = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'director2@giki.edu.pk', 'Director Two', '$argon2id$v=19$m=65536,t=3,p=4$dummy$dummy');
	`, actorID)

	// Facility 1 with s1
	var fac1ID, fac2ID, s1ID, s2ID uuid.UUID
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('court-alpha', 'Court Alpha', 'active') RETURNING id;").Scan(&fac1ID)
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience) VALUES ($1, 1, '10:00:00', '12:00:00', 'student') RETURNING id;", fac1ID).Scan(&s1ID)

	// Facility 2 with s2
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('court-beta', 'Court Beta', 'active') RETURNING id;").Scan(&fac2ID)
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience) VALUES ($1, 2, '14:00:00', '16:00:00', 'faculty') RETURNING id;", fac2ID).Scan(&s2ID)

	auditParams := facility.AuditParams{ActorID: actorID}

	// 1. Mismatched facility and schedule: slug 'court-beta' with s1ID (belongs to court-alpha)
	err := repo.DeleteScheduleWithAudit(ctx, "court-beta", s1ID, auditParams)
	if !errors.Is(err, facility.ErrScheduleNotFound) {
		t.Fatalf("expected ErrScheduleNotFound for mismatched facility/schedule, got: %v", err)
	}

	// 2. Non-existent slug with valid schedule ID
	err = repo.DeleteScheduleWithAudit(ctx, "nonexistent-court", s1ID, auditParams)
	if !errors.Is(err, facility.ErrScheduleNotFound) {
		t.Fatalf("expected ErrScheduleNotFound for nonexistent slug, got: %v", err)
	}

	// 3. Valid slug with non-existent schedule ID
	err = repo.DeleteScheduleWithAudit(ctx, "court-alpha", uuid.New(), auditParams)
	if !errors.Is(err, facility.ErrScheduleNotFound) {
		t.Fatalf("expected ErrScheduleNotFound for nonexistent schedule ID, got: %v", err)
	}

	// 4. Blank slug
	err = repo.DeleteScheduleWithAudit(ctx, "   ", s1ID, auditParams)
	if !errors.Is(err, facility.ErrScheduleNotFound) {
		t.Fatalf("expected ErrScheduleNotFound for blank slug, got: %v", err)
	}

	// 5. Nil schedule ID
	err = repo.DeleteScheduleWithAudit(ctx, "court-alpha", uuid.Nil, auditParams)
	if !errors.Is(err, facility.ErrScheduleNotFound) {
		t.Fatalf("expected ErrScheduleNotFound for nil schedule ID, got: %v", err)
	}

	// Verify both schedules are 100% intact
	var totalSchedules int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.schedules;").Scan(&totalSchedules)
	if totalSchedules != 2 {
		t.Errorf("expected 2 schedules to remain intact, found %d", totalSchedules)
	}

	// Verify 0 audit logs written
	var totalAudits int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&totalAudits)
	if totalAudits != 0 {
		t.Errorf("expected 0 audit logs after rejected attempts, found %d", totalAudits)
	}

	t.Log("Verified: mismatched and not found deletion attempts reject cleanly without side-effects.")
}

// TestFacilityRepository_DeleteScheduleWithAudit_RollbackOnAuditFailure verifies:
// - Step 8A core atomicity requirement: when audit insert fails, the transaction rolls back.
// - The schedule row is NOT deleted and remains completely unchanged in the database.
// - Zero audit log entries are committed.
func TestFacilityRepository_DeleteScheduleWithAudit_RollbackOnAuditFailure(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	// Seed facility and schedule
	var facID, schedID uuid.UUID
	var origCreatedAt, origUpdatedAt time.Time
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('rollback-court', 'Rollback Court', 'active') RETURNING id;").Scan(&facID)
	_ = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 4, '15:00:00', '18:00:00', 'women_only')
		RETURNING id, created_at, updated_at;
	`, facID).Scan(&schedID, &origCreatedAt, &origUpdatedAt)

	// Intentionally provide a non-existent ActorID to trigger foreign key violation fk_audit_logs_actor
	nonExistentActorID := uuid.New()
	auditParams := facility.AuditParams{
		ActorID:   nonExistentActorID,
		IPAddress: "127.0.0.1",
		UserAgent: "Test-Harness",
	}

	// Attempt deletion
	err := repo.DeleteScheduleWithAudit(ctx, "rollback-court", schedID, auditParams)
	if err == nil {
		t.Fatalf("expected DeleteScheduleWithAudit to fail due to foreign key violation on audit log, but got nil")
	}

	if !strings.Contains(err.Error(), "failed to write audit log") {
		t.Errorf("expected error message to mention audit log failure, got: %v", err)
	}
	t.Logf("Received expected audit insert failure: %v", err)

	// Verify that the schedule row is STILL PRESENT in giki_sports.schedules
	var dbDay int
	var dbStart, dbEnd, dbAudience string
	var dbCreatedAt, dbUpdatedAt time.Time
	err = pool.QueryRow(ctx, `
		SELECT day_of_week, to_char(start_time, 'HH24:MI:SS'), to_char(end_time, 'HH24:MI:SS'), audience, created_at, updated_at
		FROM giki_sports.schedules
		WHERE id = $1;
	`, schedID).Scan(&dbDay, &dbStart, &dbEnd, &dbAudience, &dbCreatedAt, &dbUpdatedAt)
	if err != nil {
		t.Fatalf("CRITICAL ATOMICITY VIOLATION: schedule row was deleted despite audit failure! Query error: %v", err)
	}

	if dbDay != 4 || dbStart != "15:00:00" || dbEnd != "18:00:00" || dbAudience != "women_only" {
		t.Errorf("Schedule row corrupted: day=%d, start=%s, end=%s, audience=%s", dbDay, dbStart, dbEnd, dbAudience)
	}
	if !dbCreatedAt.Equal(origCreatedAt) || !dbUpdatedAt.Equal(origUpdatedAt) {
		t.Errorf("Schedule timestamps mutated despite rollback!")
	}
	t.Log("Verified: schedule row stayed 100% intact after audit insert failure.")

	// Verify zero audit logs committed
	var auditCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
	if err != nil {
		t.Fatalf("failed to count audit logs: %v", err)
	}
	if auditCount != 0 {
		t.Errorf("expected 0 audit logs, found %d", auditCount)
	}
	t.Log("Verified: 0 audit logs committed (transaction cleanly rolled back).")
}

// TestFacilityRepository_CreateScheduleWithAudit_Success verifies:
// - Step 9A core requirement: create one weekly schedule window for a facility with an empty timetable.
// - PostgreSQL generates a new schedule UUID and persists timestamps.
// - Read-back through GetBySlug verifies the facility timetable contains the newly created schedule.
// - Exactly one matching 'schedule.create' audit log entry is committed in the same transaction
//   recording the actor, new schedule ID, facility context, and saved window values.
func TestFacilityRepository_CreateScheduleWithAudit_Success(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	// Seed actor in giki_sports.users
	actorID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'director@giki.edu.pk', 'Sports Director', '$argon2id$v=19$m=65536,t=3,p=4$dummy$dummy');
	`, actorID)
	if err != nil {
		t.Fatalf("failed to insert actor user: %v", err)
	}

	// Seed facility with an empty timetable
	var facID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, status) 
		VALUES ('empty-court', 'Empty Badminton Court', 'active') 
		RETURNING id;
	`).Scan(&facID)
	if err != nil {
		t.Fatalf("failed to insert facility: %v", err)
	}

	// Verify facility timetable starts empty
	facBefore, err := repo.GetBySlug(ctx, "empty-court")
	if err != nil {
		t.Fatalf("GetBySlug before create failed: %v", err)
	}
	if len(facBefore.Schedules) != 0 {
		t.Fatalf("expected 0 schedules initially, found %d", len(facBefore.Schedules))
	}

	createInput := facility.CreateScheduleInput{
		DayOfWeek: 1, // Monday
		StartTime: "11:00:00",
		EndTime:   "13:00:00",
		Audience:  "student",
	}

	auditParams := facility.AuditParams{
		ActorID:   actorID,
		IPAddress: "192.168.1.100",
		UserAgent: "SportsTest/1.0",
	}

	// Execute CreateScheduleWithAudit
	created, err := repo.CreateScheduleWithAudit(ctx, "empty-court", createInput, auditParams)
	if err != nil {
		t.Fatalf("CreateScheduleWithAudit failed: %v", err)
	}

	if created == nil {
		t.Fatal("expected non-nil created Schedule, got nil")
	}
	if created.ID == uuid.Nil {
		t.Errorf("expected PostgreSQL to generate a non-nil UUID for schedule ID, got %s", created.ID)
	}
	if created.FacilityID != facID {
		t.Errorf("expected FacilityID %s, got %s", facID, created.FacilityID)
	}
	if created.DayOfWeek != 1 {
		t.Errorf("expected DayOfWeek 1, got %d", created.DayOfWeek)
	}
	if created.StartTime != "11:00:00" {
		t.Errorf("expected StartTime '11:00:00', got %q", created.StartTime)
	}
	if created.EndTime != "13:00:00" {
		t.Errorf("expected EndTime '13:00:00', got %q", created.EndTime)
	}
	if created.Audience != "student" {
		t.Errorf("expected Audience 'student', got %q", created.Audience)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Errorf("expected non-zero timestamps, got created=%v, updated=%v", created.CreatedAt, created.UpdatedAt)
	}

	// Verify read-back through GetBySlug
	facAfter, err := repo.GetBySlug(ctx, "empty-court")
	if err != nil {
		t.Fatalf("GetBySlug after create failed: %v", err)
	}
	if len(facAfter.Schedules) != 1 {
		t.Fatalf("expected 1 schedule in timetable after create, found %d", len(facAfter.Schedules))
	}
	s := facAfter.Schedules[0]
	if s.ID != created.ID || s.DayOfWeek != 1 || s.StartTime != "11:00:00" || s.EndTime != "13:00:00" || s.Audience != "student" {
		t.Errorf("read-back schedule mismatch: %+v", s)
	}

	// Verify audit log entry in giki_sports.audit_logs
	var auditID, dbActorID, dbTargetID uuid.UUID
	var action, targetType, status, ipAddress, userAgent string
	var detailsBytes []byte
	var auditCreatedAt time.Time
	err = pool.QueryRow(ctx, `
		SELECT id, actor_id, action, target_type, target_id, details, ip_address, user_agent, status, created_at
		FROM giki_sports.audit_logs
		WHERE target_id = $1;
	`, created.ID).Scan(
		&auditID,
		&dbActorID,
		&action,
		&targetType,
		&dbTargetID,
		&detailsBytes,
		&ipAddress,
		&userAgent,
		&status,
		&auditCreatedAt,
	)
	if err != nil {
		t.Fatalf("failed to query audit log for created schedule: %v", err)
	}

	if dbActorID != actorID {
		t.Errorf("expected actor_id %s, got %s", actorID, dbActorID)
	}
	if action != "schedule.create" {
		t.Errorf("expected action 'schedule.create', got %q", action)
	}
	if targetType != "facility_schedule" {
		t.Errorf("expected target_type 'facility_schedule', got %q", targetType)
	}
	if dbTargetID != created.ID {
		t.Errorf("expected target_id %s, got %s", created.ID, dbTargetID)
	}
	if status != "SUCCESS" {
		t.Errorf("expected status 'SUCCESS', got %q", status)
	}
	if ipAddress != "192.168.1.100" {
		t.Errorf("expected ip_address '192.168.1.100', got %q", ipAddress)
	}
	if userAgent != "SportsTest/1.0" {
		t.Errorf("expected user_agent 'SportsTest/1.0', got %q", userAgent)
	}

	var details map[string]interface{}
	if err := json.Unmarshal(detailsBytes, &details); err != nil {
		t.Fatalf("failed to unmarshal audit details JSON: %v", err)
	}

	if details["facility_slug"] != "empty-court" {
		t.Errorf("expected details.facility_slug 'empty-court', got %v", details["facility_slug"])
	}
	if details["facility_id"] != facID.String() {
		t.Errorf("expected details.facility_id %s, got %v", facID, details["facility_id"])
	}
	if details["schedule_id"] != created.ID.String() {
		t.Errorf("expected details.schedule_id %s, got %v", created.ID, details["schedule_id"])
	}

	savedWindow, ok := details["saved_window"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected details.saved_window to be object, got %T", details["saved_window"])
	}
	if int(savedWindow["day_of_week"].(float64)) != 1 {
		t.Errorf("expected saved_window.day_of_week 1, got %v", savedWindow["day_of_week"])
	}
	if savedWindow["start_time"] != "11:00:00" {
		t.Errorf("expected saved_window.start_time '11:00:00', got %v", savedWindow["start_time"])
	}
	if savedWindow["end_time"] != "13:00:00" {
		t.Errorf("expected saved_window.end_time '13:00:00', got %v", savedWindow["end_time"])
	}
	if savedWindow["audience"] != "student" {
		t.Errorf("expected saved_window.audience 'student', got %v", savedWindow["audience"])
	}
}

// TestFacilityRepository_CreateScheduleWithAudit_EndOfDay24 verifies:
// - End time "24:00:00" is explicitly accepted as end of day.
// - Read-back through GetBySlug returns "24:00:00" string representation without truncation.
// - Audit log details captures "24:00:00" in saved_window.
func TestFacilityRepository_CreateScheduleWithAudit_EndOfDay24(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	actorID := uuid.New()
	_, _ = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'director@giki.edu.pk', 'Sports Director', '$argon2id$v=19$m=65536,t=3,p=4$dummy$dummy');
	`, actorID)

	var facID uuid.UUID
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('midnight-court', 'Midnight Court', 'active') RETURNING id;").Scan(&facID)

	createInput := facility.CreateScheduleInput{
		DayOfWeek: 5, // Friday
		StartTime: "20:00:00",
		EndTime:   "24:00:00",
		Audience:  "general",
	}

	auditParams := facility.AuditParams{
		ActorID:   actorID,
		IPAddress: "10.0.0.1",
		UserAgent: "TestAgent",
	}

	created, err := repo.CreateScheduleWithAudit(ctx, "midnight-court", createInput, auditParams)
	if err != nil {
		t.Fatalf("CreateScheduleWithAudit with 24:00:00 failed: %v", err)
	}

	if created.EndTime != "24:00:00" {
		t.Errorf("expected EndTime '24:00:00', got %q", created.EndTime)
	}

	// Verify read-back
	fac, err := repo.GetBySlug(ctx, "midnight-court")
	if err != nil {
		t.Fatalf("GetBySlug failed: %v", err)
	}
	if len(fac.Schedules) != 1 {
		t.Fatalf("expected 1 schedule, found %d", len(fac.Schedules))
	}
	if fac.Schedules[0].EndTime != "24:00:00" {
		t.Errorf("read-back expected '24:00:00', got %q", fac.Schedules[0].EndTime)
	}

	// Verify audit log details
	var detailsBytes []byte
	err = pool.QueryRow(ctx, "SELECT details FROM giki_sports.audit_logs WHERE target_id = $1;", created.ID).Scan(&detailsBytes)
	if err != nil {
		t.Fatalf("failed to query audit log: %v", err)
	}

	var details map[string]interface{}
	_ = json.Unmarshal(detailsBytes, &details)
	savedWindow := details["saved_window"].(map[string]interface{})
	if savedWindow["end_time"] != "24:00:00" {
		t.Errorf("audit log expected '24:00:00', got %v", savedWindow["end_time"])
	}
}

// TestFacilityRepository_CreateScheduleWithAudit_UnknownFacility verifies:
// - Unknown facility slug returns ErrNotFound and performs zero inserts.
// - Blank facility slug returns ErrNotFound.
// - Zero schedules and zero audit logs are inserted into the database.
func TestFacilityRepository_CreateScheduleWithAudit_UnknownFacility(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	actorID := uuid.New()
	_, _ = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'director@giki.edu.pk', 'Sports Director', '$argon2id$v=19$m=65536,t=3,p=4$dummy$dummy');
	`, actorID)

	validInput := facility.CreateScheduleInput{
		DayOfWeek: 2,
		StartTime: "10:00:00",
		EndTime:   "12:00:00",
		Audience:  "student",
	}
	auditParams := facility.AuditParams{ActorID: actorID}

	// 1. Non-existent slug
	_, err := repo.CreateScheduleWithAudit(ctx, "nonexistent-court", validInput, auditParams)
	if !errors.Is(err, facility.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existent slug, got: %v", err)
	}

	// 2. Blank slug
	_, err = repo.CreateScheduleWithAudit(ctx, "   ", validInput, auditParams)
	if !errors.Is(err, facility.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for blank slug, got: %v", err)
	}

	// Verify database remains empty
	var schedCount, auditCount int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.schedules;").Scan(&schedCount)
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)

	if schedCount != 0 {
		t.Errorf("expected 0 schedules, found %d", schedCount)
	}
	if auditCount != 0 {
		t.Errorf("expected 0 audit logs, found %d", auditCount)
	}
}

// TestFacilityRepository_CreateScheduleWithAudit_InvalidInput verifies:
// - Validation errors are returned before executing any database write.
// - Errors include invalid weekdays, unsupported audiences, start >= end, and 24:00:00 as start.
// - Zero schedules and zero audit logs are created.
func TestFacilityRepository_CreateScheduleWithAudit_InvalidInput(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	actorID := uuid.New()
	_, _ = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'director@giki.edu.pk', 'Sports Director', '$argon2id$v=19$m=65536,t=3,p=4$dummy$dummy');
	`, actorID)

	var facID uuid.UUID
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('valid-court', 'Valid Court', 'active') RETURNING id;").Scan(&facID)

	auditParams := facility.AuditParams{ActorID: actorID}

	testCases := []struct {
		name  string
		input facility.CreateScheduleInput
	}{
		{
			name: "weekday 0",
			input: facility.CreateScheduleInput{
				DayOfWeek: 0,
				StartTime: "09:00:00",
				EndTime:   "11:00:00",
				Audience:  "student",
			},
		},
		{
			name: "weekday 8",
			input: facility.CreateScheduleInput{
				DayOfWeek: 8,
				StartTime: "09:00:00",
				EndTime:   "11:00:00",
				Audience:  "student",
			},
		},
		{
			name: "unsupported audience",
			input: facility.CreateScheduleInput{
				DayOfWeek: 1,
				StartTime: "09:00:00",
				EndTime:   "11:00:00",
				Audience:  "vip_members",
			},
		},
		{
			name: "start equals end",
			input: facility.CreateScheduleInput{
				DayOfWeek: 1,
				StartTime: "10:00:00",
				EndTime:   "10:00:00",
				Audience:  "student",
			},
		},
		{
			name: "start after end",
			input: facility.CreateScheduleInput{
				DayOfWeek: 1,
				StartTime: "14:00:00",
				EndTime:   "12:00:00",
				Audience:  "student",
			},
		},
		{
			name: "24:00:00 as start time",
			input: facility.CreateScheduleInput{
				DayOfWeek: 1,
				StartTime: "24:00:00",
				EndTime:   "24:00:00",
				Audience:  "general",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.CreateScheduleWithAudit(ctx, "valid-court", tc.input, auditParams)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !errors.Is(err, facility.ErrInvalidInput) {
				t.Errorf("expected ErrInvalidInput for %s, got: %v", tc.name, err)
			}
		})
	}

	// Verify no data was written
	var schedCount, auditCount int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.schedules;").Scan(&schedCount)
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)

	if schedCount != 0 {
		t.Errorf("expected 0 schedules, found %d", schedCount)
	}
	if auditCount != 0 {
		t.Errorf("expected 0 audit logs, found %d", auditCount)
	}
}

// TestFacilityRepository_CreateScheduleWithAudit_DuplicateRejection verifies:
// - Attempting to insert an exact duplicate window (same facility, day, start, end, audience)
//   triggers uq_schedules_window and returns ErrDuplicateSchedule.
// - Exactly 1 schedule remains in the database.
// - Exactly 1 audit log remains (from initial successful creation).
func TestFacilityRepository_CreateScheduleWithAudit_DuplicateRejection(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	actorID := uuid.New()
	_, _ = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'director@giki.edu.pk', 'Sports Director', '$argon2id$v=19$m=65536,t=3,p=4$dummy$dummy');
	`, actorID)

	var facID uuid.UUID
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('dupe-court', 'Dupe Court', 'active') RETURNING id;").Scan(&facID)

	input := facility.CreateScheduleInput{
		DayOfWeek: 2,
		StartTime: "10:00:00",
		EndTime:   "12:00:00",
		Audience:  "student",
	}
	auditParams := facility.AuditParams{ActorID: actorID}

	// First insert succeeds
	first, err := repo.CreateScheduleWithAudit(ctx, "dupe-court", input, auditParams)
	if err != nil {
		t.Fatalf("first creation failed: %v", err)
	}
	if first == nil {
		t.Fatal("expected non-nil schedule for first creation")
	}

	// Duplicate insert fails with ErrDuplicateSchedule
	second, err := repo.CreateScheduleWithAudit(ctx, "dupe-court", input, auditParams)
	if err == nil {
		t.Fatal("expected duplicate creation to fail, got nil error")
	}
	if second != nil {
		t.Errorf("expected nil result on duplicate failure, got: %+v", second)
	}
	if !errors.Is(err, facility.ErrDuplicateSchedule) {
		t.Fatalf("expected ErrDuplicateSchedule, got: %v", err)
	}

	// Verify exactly 1 schedule and 1 audit log exist in database
	var schedCount, auditCount int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.schedules WHERE facility_id = $1;", facID).Scan(&schedCount)
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)

	if schedCount != 1 {
		t.Errorf("expected exactly 1 schedule, found %d", schedCount)
	}
	if auditCount != 1 {
		t.Errorf("expected exactly 1 audit log (initial creation only), found %d", auditCount)
	}
}

// TestFacilityRepository_CreateScheduleWithAudit_RollbackOnAuditFailure verifies:
// - Step 9A core requirement: when audit insert fails (e.g. non-existent actor UUID violating foreign key),
//   the entire transaction rolls back.
// - Zero schedules are saved in giki_sports.schedules.
// - Zero audit log entries are saved in giki_sports.audit_logs.
func TestFacilityRepository_CreateScheduleWithAudit_RollbackOnAuditFailure(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	repo := facility.NewRepository(pool)
	ctx := context.Background()

	var facID uuid.UUID
	_ = pool.QueryRow(ctx, "INSERT INTO giki_sports.facilities (slug, name, status) VALUES ('rollback-court', 'Rollback Court', 'active') RETURNING id;").Scan(&facID)

	createInput := facility.CreateScheduleInput{
		DayOfWeek: 3,
		StartTime: "14:00:00",
		EndTime:   "16:00:00",
		Audience:  "faculty",
	}

	// Non-existent actor UUID triggers fk_audit_logs_actor violation
	nonExistentActorID := uuid.New()
	auditParams := facility.AuditParams{
		ActorID:   nonExistentActorID,
		IPAddress: "127.0.0.1",
		UserAgent: "TestRunner",
	}

	res, err := repo.CreateScheduleWithAudit(ctx, "rollback-court", createInput, auditParams)
	if err == nil {
		t.Fatal("expected CreateScheduleWithAudit to fail due to foreign key violation on audit log, got nil error")
	}
	if res != nil {
		t.Fatalf("expected nil result on failure, got %+v", res)
	}
	if !strings.Contains(err.Error(), "failed to write audit log") {
		t.Errorf("expected error to mention audit log failure, got: %v", err)
	}

	// Verify ATOMICITY: schedule row was rolled back and does not exist
	var schedCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.schedules WHERE facility_id = $1;", facID).Scan(&schedCount)
	if err != nil {
		t.Fatalf("failed to query schedules: %v", err)
	}
	if schedCount != 0 {
		t.Fatalf("CRITICAL ATOMICITY VIOLATION: schedule was committed despite audit failure! Count: %d", schedCount)
	}
	t.Log("Verified: 0 schedules exist (schedule was rolled back upon audit failure).")

	// Verify zero audit logs committed
	var auditCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs;").Scan(&auditCount)
	if err != nil {
		t.Fatalf("failed to query audit logs: %v", err)
	}
	if auditCount != 0 {
		t.Errorf("expected 0 audit logs, found %d", auditCount)
	}
	t.Log("Verified: 0 audit logs exist.")
}
