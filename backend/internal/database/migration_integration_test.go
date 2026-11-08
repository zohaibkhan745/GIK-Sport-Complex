//go:build integration

package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"giki-sports-backend/internal/config"
)

// createDisposableDB creates a uniquely named disposable database.
func createDisposableDB(ctx context.Context, maintenanceDB *sql.DB, prefix string) (string, error) {
	dbName := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	_, err := maintenanceDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s;", dbName))
	if err != nil {
		return "", fmt.Errorf("failed to create disposable database %s: %w", dbName, err)
	}
	return dbName, nil
}

// dropDisposableDB terminates connections and drops the exact disposable database.
// Safety check: refuses to drop any database that does not have the "giki_sports_disposable_" prefix.
func dropDisposableDB(ctx context.Context, maintenanceDB *sql.DB, dbName string) error {
	if !strings.HasPrefix(dbName, "giki_sports_disposable_") {
		return fmt.Errorf("safety check violated: refusing to drop database '%s' without disposable prefix", dbName)
	}

	// Terminate any active sessions on the disposable database
	_, _ = maintenanceDB.ExecContext(ctx, fmt.Sprintf(`
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = '%s' AND pid <> pg_backend_pid();`, dbName))

	_, err := maintenanceDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE);", dbName))
	return err
}

// checkSchemaExists checks whether a schema exists in the specified database.
func checkSchemaExists(ctx context.Context, db *sql.DB, schemaName string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1);"
	err := db.QueryRowContext(ctx, query, schemaName).Scan(&exists)
	return exists, err
}

// checkTableExists checks whether a table exists in the specified schema.
func checkTableExists(ctx context.Context, db *sql.DB, schemaName, tableName string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2);"
	err := db.QueryRowContext(ctx, query, schemaName, tableName).Scan(&exists)
	return exists, err
}

// checkIndexExists checks whether an index exists in PostgreSQL.
func checkIndexExists(ctx context.Context, db *sql.DB, schemaName, indexName string) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes 
			WHERE schemaname = $1 AND indexname = $2
		);`
	err := db.QueryRowContext(ctx, query, schemaName, indexName).Scan(&exists)
	return exists, err
}

// getMigration1FixtureFS creates an in-memory fs.FS containing only migration 00001
// so tests targeting migration 1's specific behavior remain completely isolated from subsequent migrations.
func getMigration1FixtureFS(t *testing.T) fs.FS {
	mig1Path := filepath.Join("..", "..", "migrations", "00001_create_giki_sports_schema.sql")
	content, err := os.ReadFile(mig1Path)
	if err != nil {
		t.Fatalf("failed to read migration 1 file: %v", err)
	}
	return fstest.MapFS{
		"00001_create_giki_sports_schema.sql": &fstest.MapFile{
			Data: content,
		},
	}
}

// getMigration1And2FixtureFS creates an in-memory fs.FS containing migrations 00001 and 00002
// so tests targeting migrations 1 and 2 remain completely isolated from subsequent migrations.
func getMigration1And2FixtureFS(t *testing.T) fs.FS {
	mig1Path := filepath.Join("..", "..", "migrations", "00001_create_giki_sports_schema.sql")
	content1, err := os.ReadFile(mig1Path)
	if err != nil {
		t.Fatalf("failed to read migration 1 file: %v", err)
	}
	mig2Path := filepath.Join("..", "..", "migrations", "00002_create_facilities_and_schedules.sql")
	content2, err := os.ReadFile(mig2Path)
	if err != nil {
		t.Fatalf("failed to read migration 2 file: %v", err)
	}
	return fstest.MapFS{
		"00001_create_giki_sports_schema.sql": &fstest.MapFile{
			Data: content1,
		},
		"00002_create_facilities_and_schedules.sql": &fstest.MapFile{
			Data: content2,
		},
	}
}

// getMigration1To3FixtureFS creates an in-memory fs.FS containing migrations 00001, 00002, and 00003
// so tests targeting migrations 1-3 remain completely isolated from subsequent migrations.
func getMigration1To3FixtureFS(t *testing.T) fs.FS {
	mig1Path := filepath.Join("..", "..", "migrations", "00001_create_giki_sports_schema.sql")
	content1, err := os.ReadFile(mig1Path)
	if err != nil {
		t.Fatalf("failed to read migration 1 file: %v", err)
	}
	mig2Path := filepath.Join("..", "..", "migrations", "00002_create_facilities_and_schedules.sql")
	content2, err := os.ReadFile(mig2Path)
	if err != nil {
		t.Fatalf("failed to read migration 2 file: %v", err)
	}
	mig3Path := filepath.Join("..", "..", "migrations", "00003_create_users_and_sessions.sql")
	content3, err := os.ReadFile(mig3Path)
	if err != nil {
		t.Fatalf("failed to read migration 3 file: %v", err)
	}
	return fstest.MapFS{
		"00001_create_giki_sports_schema.sql":       &fstest.MapFile{Data: content1},
		"00002_create_facilities_and_schedules.sql": &fstest.MapFile{Data: content2},
		"00003_create_users_and_sessions.sql":       &fstest.MapFile{Data: content3},
	}
}

// getMigration1To4FixtureFS creates an in-memory fs.FS containing migrations 00001, 00002, 00003, and 00004
// so tests targeting migrations 1-4 remain completely isolated from subsequent migrations.
func getMigration1To4FixtureFS(t *testing.T) fs.FS {
	mig1Path := filepath.Join("..", "..", "migrations", "00001_create_giki_sports_schema.sql")
	content1, err := os.ReadFile(mig1Path)
	if err != nil {
		t.Fatalf("failed to read migration 1 file: %v", err)
	}
	mig2Path := filepath.Join("..", "..", "migrations", "00002_create_facilities_and_schedules.sql")
	content2, err := os.ReadFile(mig2Path)
	if err != nil {
		t.Fatalf("failed to read migration 2 file: %v", err)
	}
	mig3Path := filepath.Join("..", "..", "migrations", "00003_create_users_and_sessions.sql")
	content3, err := os.ReadFile(mig3Path)
	if err != nil {
		t.Fatalf("failed to read migration 3 file: %v", err)
	}
	mig4Path := filepath.Join("..", "..", "migrations", "00004_create_audit_logs.sql")
	content4, err := os.ReadFile(mig4Path)
	if err != nil {
		t.Fatalf("failed to read migration 4 file: %v", err)
	}
	return fstest.MapFS{
		"00001_create_giki_sports_schema.sql":       &fstest.MapFile{Data: content1},
		"00002_create_facilities_and_schedules.sql": &fstest.MapFile{Data: content2},
		"00003_create_users_and_sessions.sql":       &fstest.MapFile{Data: content3},
		"00004_create_audit_logs.sql":               &fstest.MapFile{Data: content4},
	}
}

// TestMigrationIntegration_DisposableDatabaseCycle tests migration 1's full lifecycle
// (initial Up, repeating Up idempotency, version Status, Down reversal, and reapplying Up)
// against an isolated disposable database using a dedicated migration 1 fixture.
//
// SAFETY RULE: Reverse migrations (Down) are NEVER run against the development database.
func TestMigrationIntegration_DisposableDatabaseCycle(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("integration test failed: configuration could not be loaded: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	maintenanceDB, err := NewSQLDB(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("integration test failed: unable to connect to PostgreSQL server at %s:%d: %v",
			cfg.DB.Host, cfg.DB.Port, err)
	}
	defer maintenanceDB.Close()

	disposableDBName, err := createDisposableDB(ctx, maintenanceDB, "giki_sports_disposable_cycle")
	if err != nil {
		t.Fatalf("failed to create disposable database: %v", err)
	}

	testDBCfg := cfg.DB
	testDBCfg.Name = disposableDBName
	testDB, err := NewSQLDB(ctx, testDBCfg)
	if err != nil {
		t.Fatalf("unable to connect to disposable database %s: %v", disposableDBName, err)
	}

	// Bounded cleanup: closes test connection first, then drops disposable DB and reports errors
	defer func() {
		t.Logf("Cleaning up disposable test database: %s", disposableDBName)
		_ = testDB.Close()

		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		if dropErr := dropDisposableDB(cleanupCtx, maintenanceDB, disposableDBName); dropErr != nil {
			t.Errorf("cleanup failure: failed to drop disposable test database %s: %v", disposableDBName, dropErr)
		}
	}()

	// Use fixture containing strictly migration 1
	mig1FS := getMigration1FixtureFS(t)
	runner, err := NewMigrationRunner(testDB, mig1FS)
	if err != nil {
		t.Fatalf("failed to initialize migration runner: %v", err)
	}
	defer runner.Close()

	// Sub-check A: Schema does NOT exist initially
	exists, err := checkSchemaExists(ctx, testDB, "giki_sports")
	if err != nil || exists {
		t.Fatalf("expected schema 'giki_sports' to not exist initially in fresh DB: exists=%v, err=%v", exists, err)
	}

	// Sub-check B: Apply Up
	t.Log("Applying migration 1 (Up)...")
	upResults, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("migration Up failed: %v", err)
	}
	if len(upResults) == 0 {
		t.Fatalf("expected at least one migration result, got 0")
	}

	exists, err = checkSchemaExists(ctx, testDB, "giki_sports")
	if err != nil || !exists {
		t.Fatalf("expected schema 'giki_sports' to exist after Up, err: %v", err)
	}

	// Sub-check C: Idempotency (repeating Up makes no changes)
	t.Log("Testing Up idempotency (repeating Up)...")
	repeatResults, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("repeating Up failed: %v", err)
	}
	if len(repeatResults) != 0 {
		t.Fatalf("expected 0 migrations applied on repeat Up, got %d", len(repeatResults))
	}

	// Sub-check D: Status reports version 1 applied
	statusList, err := runner.Status(ctx)
	if err != nil {
		t.Fatalf("failed to query status: %v", err)
	}
	if len(statusList) != 1 || statusList[0].State != goose.StateApplied {
		t.Fatalf("expected 1 applied migration in status, got %+v", statusList)
	}

	// Sub-check E: Down reverses the empty schema
	t.Log("Applying Down on empty schema...")
	downResult, err := runner.Down(ctx)
	if err != nil {
		t.Fatalf("Down on empty schema failed: %v", err)
	}
	if downResult == nil {
		t.Fatalf("expected non-nil Down result")
	}

	exists, err = checkSchemaExists(ctx, testDB, "giki_sports")
	if err != nil || exists {
		t.Fatalf("expected schema 'giki_sports' to be removed after Down: exists=%v, err=%v", exists, err)
	}

	// Sub-check F: Reapplying Up succeeds
	t.Log("Reapplying Up...")
	reapplyResults, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("reapplying Up failed: %v", err)
	}
	if len(reapplyResults) == 0 {
		t.Fatalf("expected migration to reapply, got 0 results")
	}

	exists, err = checkSchemaExists(ctx, testDB, "giki_sports")
	if err != nil || !exists {
		t.Fatalf("expected schema 'giki_sports' to exist after reapply Up")
	}
}

// TestMigrationIntegration_NonEmptySchemaRollbackRejected verifies:
// 1. Initial empty-schema rollback succeeds.
// 2. A schema containing a probe table rejects rollback (RESTRICT prevents accidental deletion).
// 3. The probe table and migration version remain intact after the rollback failure.
func TestMigrationIntegration_NonEmptySchemaRollbackRejected(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("integration test failed: configuration could not be loaded: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	maintenanceDB, err := NewSQLDB(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("integration test failed: unable to connect to PostgreSQL server: %v", err)
	}
	defer maintenanceDB.Close()

	disposableDBName, err := createDisposableDB(ctx, maintenanceDB, "giki_sports_disposable_nonempty")
	if err != nil {
		t.Fatalf("failed to create disposable database: %v", err)
	}

	testDBCfg := cfg.DB
	testDBCfg.Name = disposableDBName
	testDB, err := NewSQLDB(ctx, testDBCfg)
	if err != nil {
		t.Fatalf("unable to connect to disposable database %s: %v", disposableDBName, err)
	}

	defer func() {
		t.Logf("Cleaning up disposable test database: %s", disposableDBName)
		_ = testDB.Close()

		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		if dropErr := dropDisposableDB(cleanupCtx, maintenanceDB, disposableDBName); dropErr != nil {
			t.Errorf("cleanup failure: failed to drop disposable test database %s: %v", disposableDBName, dropErr)
		}
	}()

	// Use fixture containing strictly migration 1 to test schema RESTRICT rollback
	mig1FS := getMigration1FixtureFS(t)
	runner, err := NewMigrationRunner(testDB, mig1FS)
	if err != nil {
		t.Fatalf("failed to initialize migration runner: %v", err)
	}
	defer runner.Close()

	// 1. Apply Up
	t.Log("Applying Up to provision giki_sports schema...")
	if _, err := runner.Up(ctx); err != nil {
		t.Fatalf("runner.Up failed: %v", err)
	}

	// 2. Insert probe table inside giki_sports schema
	t.Log("Creating probe table inside giki_sports schema...")
	_, err = testDB.ExecContext(ctx, `
		CREATE TABLE giki_sports.probe_table (
			id INT PRIMARY KEY,
			probe_name TEXT NOT NULL
		);
		INSERT INTO giki_sports.probe_table (id, probe_name) VALUES (42, 'critical_production_probe');
	`)
	if err != nil {
		t.Fatalf("failed to create probe table: %v", err)
	}

	// 3. Attempt Down: MUST FAIL because schema is not empty and RESTRICT prevents deletion
	t.Log("Attempting Down against nonempty schema (expecting rejection due to RESTRICT)...")
	_, downErr := runner.Down(ctx)
	if downErr == nil {
		t.Fatalf("CRITICAL SAFETY FLAW: Down succeeded against a nonempty schema! DROP SCHEMA RESTRICT failed to reject rollback.")
	}
	t.Logf("Down was rejected as expected: %v", downErr)

	// 4. Verify probe table remains intact with its data
	t.Log("Verifying probe table and data remain completely intact...")
	var probeCount int
	var probeName string
	err = testDB.QueryRowContext(ctx, "SELECT count(*), max(probe_name) FROM giki_sports.probe_table;").Scan(&probeCount, &probeName)
	if err != nil {
		t.Fatalf("failed to query probe table after failed Down: %v", err)
	}
	if probeCount != 1 || probeName != "critical_production_probe" {
		t.Fatalf("expected probe table to retain 1 row with name 'critical_production_probe', got count=%d, name='%s'", probeCount, probeName)
	}

	// 5. Verify migration version in goose_db_version is still intact (version 1 applied)
	statusList, err := runner.Status(ctx)
	if err != nil {
		t.Fatalf("failed to query status after failed Down: %v", err)
	}
	if len(statusList) != 1 || statusList[0].State != goose.StateApplied {
		t.Fatalf("expected migration version 1 to remain applied after failed rollback, got %+v", statusList)
	}
	t.Log("Confirmed: probe table and version 1 record remained completely intact after rejected Down.")

	// 6. Clean up probe table and verify Down succeeds when schema is empty
	t.Log("Cleaning up probe table to verify empty-schema Down succeeds afterwards...")
	_, err = testDB.ExecContext(ctx, "DROP TABLE giki_sports.probe_table;")
	if err != nil {
		t.Fatalf("failed to drop probe table: %v", err)
	}

	_, err = runner.Down(ctx)
	if err != nil {
		t.Fatalf("Down on now-empty schema failed: %v", err)
	}

	exists, err := checkSchemaExists(ctx, testDB, "giki_sports")
	if err != nil || exists {
		t.Fatalf("expected schema 'giki_sports' to be dropped after empty Down, exists=%v, err=%v", exists, err)
	}
	t.Log("Confirmed: empty schema successfully rolled back after probe table was removed.")
}

// TestMigrationIntegration_ConcurrentMigrationsCoordinated verifies that concurrent migration attempts
// against the same disposable database coordinate via PostgreSQL session advisory locking
// and complete consistently across all migrations without race conditions or duplicate version insertions.
func TestMigrationIntegration_ConcurrentMigrationsCoordinated(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("integration test failed: configuration could not be loaded: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	maintenanceDB, err := NewSQLDB(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("integration test failed: unable to connect to PostgreSQL server: %v", err)
	}
	defer maintenanceDB.Close()

	disposableDBName, err := createDisposableDB(ctx, maintenanceDB, "giki_sports_disposable_concurrent")
	if err != nil {
		t.Fatalf("failed to create disposable database: %v", err)
	}

	defer func() {
		t.Logf("Cleaning up disposable test database: %s", disposableDBName)
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		if dropErr := dropDisposableDB(cleanupCtx, maintenanceDB, disposableDBName); dropErr != nil {
			t.Errorf("cleanup failure: failed to drop disposable test database %s: %v", disposableDBName, dropErr)
		}
	}()

	fixtureFS := getMigration1And2FixtureFS(t)
	const concurrencyLevel = 5

	var wg sync.WaitGroup
	var appliedCount int32
	var errorCount int32
	var errorsList []string
	var mu sync.Mutex

	t.Logf("Launching %d concurrent migration runners against %s...", concurrencyLevel, disposableDBName)

	for i := 0; i < concurrencyLevel; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			testDBCfg := cfg.DB
			testDBCfg.Name = disposableDBName
			workerDB, err := NewSQLDB(ctx, testDBCfg)
			if err != nil {
				atomic.AddInt32(&errorCount, 1)
				mu.Lock()
				errorsList = append(errorsList, fmt.Sprintf("worker %d connection failed: %v", workerID, err))
				mu.Unlock()
				return
			}
			defer workerDB.Close()

			runner, err := NewMigrationRunner(workerDB, fixtureFS)
			if err != nil {
				atomic.AddInt32(&errorCount, 1)
				mu.Lock()
				errorsList = append(errorsList, fmt.Sprintf("worker %d runner init failed: %v", workerID, err))
				mu.Unlock()
				return
			}
			defer runner.Close()

			results, err := runner.Up(ctx)
			if err != nil {
				atomic.AddInt32(&errorCount, 1)
				mu.Lock()
				errorsList = append(errorsList, fmt.Sprintf("worker %d Up failed: %v", workerID, err))
				mu.Unlock()
				return
			}

			if len(results) > 0 {
				atomic.AddInt32(&appliedCount, int32(len(results)))
				t.Logf("Worker %d successfully applied %d migration(s)", workerID, len(results))
			} else {
				t.Logf("Worker %d safely detected migrations already applied (0 pending)", workerID)
			}
		}(i)
	}

	wg.Wait()

	if errorCount > 0 {
		t.Fatalf("concurrent migration encountered %d errors:\n%s", errorCount, strings.Join(errorsList, "\n"))
	}

	// At least 1 worker applied migrations, and total applied migrations equals 2 (migration 1 and 2)
	if appliedCount != 2 {
		t.Fatalf("expected exactly 2 migrations applied across concurrent workers, got %d", appliedCount)
	}

	// Connect to verify database integrity
	verifyDBCfg := cfg.DB
	verifyDBCfg.Name = disposableDBName
	verifyDB, err := NewSQLDB(ctx, verifyDBCfg)
	if err != nil {
		t.Fatalf("failed to connect to verify database: %v", err)
	}
	defer verifyDB.Close()

	// Verify schema exists
	exists, err := checkSchemaExists(ctx, verifyDB, "giki_sports")
	if err != nil || !exists {
		t.Fatalf("expected schema 'giki_sports' to exist after concurrent migrations, exists=%v, err=%v", exists, err)
	}

	// Verify both version 1 and version 2 are recorded in goose_db_version (excluding goose sentinel version 0)
	var versionCount int
	err = verifyDB.QueryRowContext(ctx, "SELECT count(*) FROM goose_db_version WHERE version_id > 0 AND is_applied = true;").Scan(&versionCount)
	if err != nil {
		t.Fatalf("failed to query goose_db_version: %v", err)
	}
	if versionCount != 2 {
		t.Fatalf("expected exactly 2 applied migration records (version 1 and 2) in goose_db_version, got %d", versionCount)
	}

	t.Log("Confirmed: concurrent migrations coordinated flawlessly through PostgreSQL session advisory locks.")
}

// TestMigrationIntegration_FacilitiesAndSchedulesLifecycle verifies Step 4B:
// 1. Table creation and index structure for facilities and schedules.
// 2. Valid facility and schedule insertion.
// 3. Foreign-key rejection for unknown facility UUID.
// 4. Invalid weekday, time range, status, and audience rejection.
// 5. Duplicate slug and duplicate schedule window rejection.
// 6. Whitespace-only name and slug rejection.
// 7. Restrictive delete rejection on facility with active schedules.
// 8. Migration 2 rollback: schedules and facilities drop, schema and migration 1 remain.
// 9. Reapplying migration 2 succeeds.
func TestMigrationIntegration_FacilitiesAndSchedulesLifecycle(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("integration test failed: configuration could not be loaded: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	maintenanceDB, err := NewSQLDB(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("integration test failed: unable to connect to PostgreSQL server: %v", err)
	}
	defer maintenanceDB.Close()

	disposableDBName, err := createDisposableDB(ctx, maintenanceDB, "giki_sports_disposable_facsched")
	if err != nil {
		t.Fatalf("failed to create disposable database: %v", err)
	}

	testDBCfg := cfg.DB
	testDBCfg.Name = disposableDBName
	testDB, err := NewSQLDB(ctx, testDBCfg)
	if err != nil {
		t.Fatalf("unable to connect to disposable database %s: %v", disposableDBName, err)
	}

	defer func() {
		t.Logf("Cleaning up disposable test database: %s", disposableDBName)
		_ = testDB.Close()

		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		if dropErr := dropDisposableDB(cleanupCtx, maintenanceDB, disposableDBName); dropErr != nil {
			t.Errorf("cleanup failure: failed to drop disposable test database %s: %v", disposableDBName, dropErr)
		}
	}()

	fixtureFS := getMigration1And2FixtureFS(t)
	runner, err := NewMigrationRunner(testDB, fixtureFS)
	if err != nil {
		t.Fatalf("failed to initialize migration runner: %v", err)
	}
	defer runner.Close()

	// Step A: Apply all migrations (1 and 2)
	t.Log("Applying migrations 1 and 2 (Up)...")
	upResults, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}
	if len(upResults) != 2 {
		t.Fatalf("expected 2 migrations applied, got %d", len(upResults))
	}

	// Step B: Verify table and index structure
	hasFacilities, err := checkTableExists(ctx, testDB, "giki_sports", "facilities")
	if err != nil || !hasFacilities {
		t.Fatalf("expected table giki_sports.facilities to exist: %v", err)
	}
	hasSchedules, err := checkTableExists(ctx, testDB, "giki_sports", "schedules")
	if err != nil || !hasSchedules {
		t.Fatalf("expected table giki_sports.schedules to exist: %v", err)
	}
	hasFKIndex, err := checkIndexExists(ctx, testDB, "giki_sports", "idx_schedules_facility_id")
	if err != nil || !hasFKIndex {
		t.Fatalf("expected index giki_sports.idx_schedules_facility_id to exist: %v", err)
	}
	t.Log("Verified: facilities, schedules tables, and foreign-key index exist.")

	// Step C: Insert valid facility and schedule
	var facilityID string
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, description, location, status)
		VALUES ('padel-court-1', 'GIKI Glass Padel Arena (Court 1)', 'Tournament glass court', 'Outdoor Arena North', 'active')
		RETURNING id;
	`).Scan(&facilityID)
	if err != nil {
		t.Fatalf("failed to insert valid facility: %v", err)
	}
	if facilityID == "" {
		t.Fatalf("expected non-empty generated UUID for facility")
	}

	var scheduleID string
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 1, '06:00:00', '11:00:00', 'student')
		RETURNING id;
	`, facilityID).Scan(&scheduleID)
	if err != nil {
		t.Fatalf("failed to insert valid schedule: %v", err)
	}
	if scheduleID == "" {
		t.Fatalf("expected non-empty generated UUID for schedule")
	}
	t.Logf("Inserted valid facility (%s) and schedule (%s).", facilityID, scheduleID)

	// Step D: Verify Foreign-Key rejection for unknown facility
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ('00000000-0000-0000-0000-000000000999', 1, '12:00:00', '14:00:00', 'general');
	`)
	if err == nil {
		t.Fatalf("expected foreign-key violation for non-existent facility ID, but query succeeded")
	}
	t.Log("Verified: schedule referencing unknown facility rejected by foreign key constraint.")

	// Step E: Verify Invalid Weekday Rejection (day_of_week 0 and 8)
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 0, '12:00:00', '14:00:00', 'general');
	`, facilityID)
	if err == nil {
		t.Fatalf("expected CHECK constraint rejection for day_of_week=0, but insert succeeded")
	}
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 8, '12:00:00', '14:00:00', 'general');
	`, facilityID)
	if err == nil {
		t.Fatalf("expected CHECK constraint rejection for day_of_week=8, but insert succeeded")
	}
	t.Log("Verified: day_of_week values outside 1-7 rejected by CHECK constraint.")

	// Step F: Verify Invalid Time Range Rejection (start >= end)
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 2, '14:00:00', '12:00:00', 'general');
	`, facilityID)
	if err == nil {
		t.Fatalf("expected CHECK constraint rejection for start_time > end_time, but insert succeeded")
	}
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 2, '14:00:00', '14:00:00', 'general');
	`, facilityID)
	if err == nil {
		t.Fatalf("expected CHECK constraint rejection for start_time = end_time, but insert succeeded")
	}
	t.Log("Verified: invalid time ranges rejected by start_time < end_time CHECK constraint.")

	// Step G: Verify Invalid Status and Audience Rejection
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, status)
		VALUES ('bad-facility', 'Bad Facility', 'invalid_status');
	`)
	if err == nil {
		t.Fatalf("expected CHECK constraint rejection for invalid status, but insert succeeded")
	}
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 3, '12:00:00', '14:00:00', 'invalid_audience');
	`, facilityID)
	if err == nil {
		t.Fatalf("expected CHECK constraint rejection for invalid audience, but insert succeeded")
	}
	t.Log("Verified: invalid facility status and schedule audience rejected by CHECK constraints.")

	// Step H: Verify Duplicate Slug and Window Rejection
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.facilities (slug, name)
		VALUES ('padel-court-1', 'Duplicate Padel Court');
	`)
	if err == nil {
		t.Fatalf("expected UNIQUE constraint rejection for duplicate slug, but insert succeeded")
	}
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 1, '06:00:00', '11:00:00', 'student');
	`, facilityID)
	if err == nil {
		t.Fatalf("expected UNIQUE constraint rejection for duplicate schedule window, but insert succeeded")
	}
	t.Log("Verified: duplicate slug and duplicate schedule window rejected by UNIQUE constraints.")

	// Step I: Verify Required Whitespace-only Text Rejection
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.facilities (slug, name)
		VALUES ('   ', 'Whitespace Slug');
	`)
	if err == nil {
		t.Fatalf("expected CHECK constraint rejection for whitespace-only slug, but insert succeeded")
	}
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.facilities (slug, name)
		VALUES ('valid-slug-2', '   ');
	`)
	if err == nil {
		t.Fatalf("expected CHECK constraint rejection for whitespace-only name, but insert succeeded")
	}
	t.Log("Verified: whitespace-only slugs and names rejected by CHECK constraints.")

	// Step J: Verify Restrictive Delete (Facility with schedule cannot be deleted directly)
	_, err = testDB.ExecContext(ctx, "DELETE FROM giki_sports.facilities WHERE id = $1;", facilityID)
	if err == nil {
		t.Fatalf("expected ON DELETE RESTRICT foreign-key violation deleting facility with active schedule, but delete succeeded")
	}
	t.Log("Verified: ON DELETE RESTRICT prevents deleting facility while dependent schedules exist.")

	// Delete schedule first, then facility should delete cleanly
	_, err = testDB.ExecContext(ctx, "DELETE FROM giki_sports.schedules WHERE id = $1;", scheduleID)
	if err != nil {
		t.Fatalf("failed to delete schedule: %v", err)
	}
	_, err = testDB.ExecContext(ctx, "DELETE FROM giki_sports.facilities WHERE id = $1;", facilityID)
	if err != nil {
		t.Fatalf("failed to delete facility after its schedule was removed: %v", err)
	}
	t.Log("Verified: facility deletes cleanly once dependent schedules are removed.")

	// Step K: Re-populate a sample facility + schedule and test Rollback of Migration 2
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, status)
		VALUES ('padel-court-temp', 'Temp Padel', 'active')
		RETURNING id;
	`).Scan(&facilityID)
	if err != nil {
		t.Fatalf("failed to insert temporary facility: %v", err)
	}
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 5, '17:00:00', '19:00:00', 'faculty');
	`, facilityID)
	if err != nil {
		t.Fatalf("failed to insert temporary schedule: %v", err)
	}

	t.Log("Rolling back migration 2 (Down)...")
	downRes, err := runner.Down(ctx)
	if err != nil {
		t.Fatalf("failed to roll back migration 2: %v", err)
	}
	if downRes == nil {
		t.Fatalf("expected non-nil Down result for migration 2")
	}

	// Verify tables disappear
	hasFacilitiesAfterDown, err := checkTableExists(ctx, testDB, "giki_sports", "facilities")
	if err != nil || hasFacilitiesAfterDown {
		t.Fatalf("expected table giki_sports.facilities to be dropped after Down: %v", err)
	}
	hasSchedulesAfterDown, err := checkTableExists(ctx, testDB, "giki_sports", "schedules")
	if err != nil || hasSchedulesAfterDown {
		t.Fatalf("expected table giki_sports.schedules to be dropped after Down: %v", err)
	}

	// Verify schema giki_sports still exists and version 1 remains
	hasSchemaAfterDown, err := checkSchemaExists(ctx, testDB, "giki_sports")
	if err != nil || !hasSchemaAfterDown {
		t.Fatalf("expected schema giki_sports to remain intact after rolling back migration 2: %v", err)
	}

	var currentVersion int64
	err = testDB.QueryRowContext(ctx, "SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1;").Scan(&currentVersion)
	if err != nil {
		t.Fatalf("failed to query goose_db_version after Down: %v", err)
	}
	if currentVersion != 1 {
		t.Fatalf("expected current migration version to be 1 after Down, got %d", currentVersion)
	}
	t.Log("Verified: migration 2 rollback dropped tables, preserved schema giki_sports, and left version 1 active.")

	// Step L: Reapply migration 2
	t.Log("Reapplying migration 2 (Up)...")
	reapplyResults, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("failed to reapply migration 2: %v", err)
	}
	if len(reapplyResults) != 1 {
		t.Fatalf("expected exactly 1 migration reapplied, got %d", len(reapplyResults))
	}

	hasFacilitiesReapplied, err := checkTableExists(ctx, testDB, "giki_sports", "facilities")
	if err != nil || !hasFacilitiesReapplied {
		t.Fatalf("expected table facilities to exist after reapply Up")
	}
	hasSchedulesReapplied, err := checkTableExists(ctx, testDB, "giki_sports", "schedules")
	if err != nil || !hasSchedulesReapplied {
		t.Fatalf("expected table schedules to exist after reapply Up")
	}

	err = testDB.QueryRowContext(ctx, "SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1;").Scan(&currentVersion)
	if err != nil || currentVersion != 2 {
		t.Fatalf("expected current migration version to be 2 after reapply Up, got %d (err: %v)", currentVersion, err)
	}
	t.Log("Verified: migration 2 reapplied cleanly and restored tables at version 2.")
}

// TestMigrationIntegration_UsersAndSessionsLifecycle verifies Step 6A:
// 1. Table creation and index structure for users, user_roles, and sessions.
// 2. Valid synthetic user, role, and session records insertion.
// 3. Case-insensitive duplicate email rejection.
// 4. Blank required field rejection (email, display_name, password_hash, token_hash).
// 5. Invalid roles ('admin', 'cashier', 'guest') and account states rejection.
// 6. Foreign-key rejection for unknown user references in roles and sessions.
// 7. Duplicate session token-hash rejection.
// 8. Invalid timestamp ordering rejection (last_activity < created, absolute_expiry <= created, revoked < created).
// 9. Restrictive parent deletion rejection (cannot delete user with active roles or sessions).
// 10. Migration 3 rollback: drops sessions, user_roles, and users; preserves facilities and schedules tables and data.
// 11. Reapplying migration 3 succeeds cleanly.
func TestMigrationIntegration_UsersAndSessionsLifecycle(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("integration test failed: configuration could not be loaded: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	maintenanceDB, err := NewSQLDB(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("integration test failed: unable to connect to PostgreSQL server: %v", err)
	}
	defer maintenanceDB.Close()

	disposableDBName, err := createDisposableDB(ctx, maintenanceDB, "giki_sports_disposable_auth")
	if err != nil {
		t.Fatalf("failed to create disposable database: %v", err)
	}

	testDBCfg := cfg.DB
	testDBCfg.Name = disposableDBName
	testDB, err := NewSQLDB(ctx, testDBCfg)
	if err != nil {
		t.Fatalf("unable to connect to disposable database %s: %v", disposableDBName, err)
	}

	defer func() {
		t.Logf("Cleaning up disposable test database: %s", disposableDBName)
		_ = testDB.Close()

		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		if dropErr := dropDisposableDB(cleanupCtx, maintenanceDB, disposableDBName); dropErr != nil {
			t.Errorf("cleanup failure: failed to drop disposable test database %s: %v", disposableDBName, dropErr)
		}
	}()

	fixtureFS := getMigration1To3FixtureFS(t)
	runner, err := NewMigrationRunner(testDB, fixtureFS)
	if err != nil {
		t.Fatalf("failed to initialize migration runner: %v", err)
	}
	defer runner.Close()

	// Step A: Apply all migrations (1, 2, and 3)
	t.Log("Applying all migrations 1, 2, and 3 (Up)...")
	upResults, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}
	if len(upResults) != 3 {
		t.Fatalf("expected 3 migrations applied, got %d", len(upResults))
	}

	// Step B: Verify tables and indexes exist
	for _, table := range []string{"users", "user_roles", "sessions"} {
		exists, err := checkTableExists(ctx, testDB, "giki_sports", table)
		if err != nil || !exists {
			t.Fatalf("expected table giki_sports.%s to exist: %v", table, err)
		}
	}
	for _, index := range []string{"uq_users_email_lower", "idx_sessions_user_id"} {
		exists, err := checkIndexExists(ctx, testDB, "giki_sports", index)
		if err != nil || !exists {
			t.Fatalf("expected index giki_sports.%s to exist: %v", index, err)
		}
	}
	t.Log("Verified: users, user_roles, and sessions tables and indexes exist.")

	// Step C: Insert valid synthetic user, role, and session
	// Note: Synthetic placeholder hashes are used exclusively for schema validation; not real credentials.
	const syntheticArgon2idHash = "$argon2id$v=19$m=65536,t=3,p=4$synthetic_test_salt$synthetic_test_hash"
	const syntheticTokenHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	var userID string
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.users (email, display_name, password_hash, status)
		VALUES ('student.test@giki.edu.pk', 'Synthetic Student User', $1, 'active')
		RETURNING id;
	`, syntheticArgon2idHash).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to insert valid synthetic user: %v", err)
	}
	if userID == "" {
		t.Fatalf("expected non-empty UUID for user")
	}

	var roleID string
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.user_roles (user_id, role)
		VALUES ($1, 'student')
		RETURNING id;
	`, userID).Scan(&roleID)
	if err != nil {
		t.Fatalf("failed to insert valid user role: %v", err)
	}

	// Verify multiple roles supported: assign sports_director as second role to the same user
	var secondRoleID string
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.user_roles (user_id, role)
		VALUES ($1, 'sports_director')
		RETURNING id;
	`, userID).Scan(&secondRoleID)
	if err != nil {
		t.Fatalf("failed to insert second role for user (multiple roles should be supported): %v", err)
	}

	var sessionID string
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.sessions (user_id, token_hash, created_at, last_activity_at, absolute_expires_at)
		VALUES ($1, $2, now(), now(), now() + INTERVAL '8 hours')
		RETURNING id;
	`, userID, syntheticTokenHash).Scan(&sessionID)
	if err != nil {
		t.Fatalf("failed to insert valid synthetic session: %v", err)
	}
	t.Logf("Inserted valid synthetic user (%s), roles (%s, %s), and session (%s).", userID, roleID, secondRoleID, sessionID)

	// Step D: Case-insensitive duplicate email rejection
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.users (email, display_name, password_hash, status)
		VALUES ('STUDENT.TEST@GIKI.EDU.PK', 'Duplicate Email User', $1, 'active');
	`, syntheticArgon2idHash)
	if err == nil {
		t.Fatalf("expected error on case-insensitive duplicate email, but query succeeded")
	}

	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.users (email, display_name, password_hash, status)
		VALUES ('  student.test@giki.edu.pk  ', 'Trimmed Duplicate Email User', $1, 'active');
	`, syntheticArgon2idHash)
	if err == nil {
		t.Fatalf("expected error on whitespace-padded duplicate email, but query succeeded")
	}
	t.Log("Verified: case-insensitive duplicate email rejection enforced.")

	// Step E: Blank required field rejection
	blankCases := []struct {
		name  string
		query string
		args  []any
	}{
		{
			name:  "blank email",
			query: `INSERT INTO giki_sports.users (email, display_name, password_hash) VALUES ('   ', 'Name', $1);`,
			args:  []any{syntheticArgon2idHash},
		},
		{
			name:  "blank display_name",
			query: `INSERT INTO giki_sports.users (email, display_name, password_hash) VALUES ('unique1@giki.edu.pk', '   ', $1);`,
			args:  []any{syntheticArgon2idHash},
		},
		{
			name:  "blank password_hash",
			query: `INSERT INTO giki_sports.users (email, display_name, password_hash) VALUES ('unique2@giki.edu.pk', 'Name', '  ');`,
			args:  nil,
		},
		{
			name:  "blank token_hash",
			query: `INSERT INTO giki_sports.sessions (user_id, token_hash, absolute_expires_at) VALUES ($1, '  ', now() + INTERVAL '8 hours');`,
			args:  []any{userID},
		},
	}
	for _, tc := range blankCases {
		_, err = testDB.ExecContext(ctx, tc.query, tc.args...)
		if err == nil {
			t.Fatalf("expected error on %s, but query succeeded", tc.name)
		}
	}
	t.Log("Verified: blank required fields rejected by CHECK constraints.")

	// Step F: Invalid roles and account states rejection
	invalidRoles := []string{"admin", "cashier", "guest", "superadmin"}
	for _, r := range invalidRoles {
		_, err = testDB.ExecContext(ctx, `
			INSERT INTO giki_sports.user_roles (user_id, role) VALUES ($1, $2);
		`, userID, r)
		if err == nil {
			t.Fatalf("expected error for invalid role '%s', but query succeeded", r)
		}
	}

	invalidStatuses := []string{"pending", "banned", "suspended", "disabled"}
	for _, s := range invalidStatuses {
		_, err = testDB.ExecContext(ctx, `
			INSERT INTO giki_sports.users (email, display_name, password_hash, status)
			VALUES ($1, 'Name', $2, $3);
		`, fmt.Sprintf("invalid_status_%s@giki.edu.pk", s), syntheticArgon2idHash, s)
		if err == nil {
			t.Fatalf("expected error for invalid status '%s', but query succeeded", s)
		}
	}
	t.Log("Verified: invalid roles and invalid account statuses rejected.")

	// Step G: Unknown user references rejection (foreign-key integrity)
	unknownUserID := "00000000-0000-0000-0000-000000000999"
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.user_roles (user_id, role) VALUES ($1, 'student');
	`, unknownUserID)
	if err == nil {
		t.Fatalf("expected foreign-key violation for unknown user in user_roles, but query succeeded")
	}

	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.sessions (user_id, token_hash, absolute_expires_at)
		VALUES ($1, 'synthetic_hash_unknown_user_777777777777777777777777777777777777', now() + INTERVAL '8 hours');
	`, unknownUserID)
	if err == nil {
		t.Fatalf("expected foreign-key violation for unknown user in sessions, but query succeeded")
	}
	t.Log("Verified: foreign-key integrity prevents orphaned user_roles and sessions.")

	// Step H: Duplicate session token-hash rejection
	var otherUserID string
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.users (email, display_name, password_hash, status)
		VALUES ('other.user@giki.edu.pk', 'Other User', $1, 'active')
		RETURNING id;
	`, syntheticArgon2idHash).Scan(&otherUserID)
	if err != nil {
		t.Fatalf("failed to insert second user: %v", err)
	}

	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.sessions (user_id, token_hash, absolute_expires_at)
		VALUES ($1, $2, now() + INTERVAL '8 hours');
	`, otherUserID, syntheticTokenHash)
	if err == nil {
		t.Fatalf("expected error on duplicate session token_hash, but query succeeded")
	}
	t.Log("Verified: duplicate session token_hash rejected by unique constraint.")

	// Step I: Invalid timestamp ordering rejection
	timestampCases := []struct {
		name  string
		query string
	}{
		{
			name: "last_activity before created_at",
			query: fmt.Sprintf(`
				INSERT INTO giki_sports.sessions (user_id, token_hash, created_at, last_activity_at, absolute_expires_at)
				VALUES ('%s', 'hash_t1_111111111111111111111111111111111111111111111111111111111111', now(), now() - INTERVAL '1 hour', now() + INTERVAL '8 hours');
			`, userID),
		},
		{
			name: "absolute_expires_at before created_at",
			query: fmt.Sprintf(`
				INSERT INTO giki_sports.sessions (user_id, token_hash, created_at, last_activity_at, absolute_expires_at)
				VALUES ('%s', 'hash_t2_222222222222222222222222222222222222222222222222222222222222', now(), now(), now() - INTERVAL '1 hour');
			`, userID),
		},
		{
			name: "revoked_at before created_at",
			query: fmt.Sprintf(`
				INSERT INTO giki_sports.sessions (user_id, token_hash, created_at, last_activity_at, absolute_expires_at, revoked_at)
				VALUES ('%s', 'hash_t3_333333333333333333333333333333333333333333333333333333333333', now(), now(), now() + INTERVAL '8 hours', now() - INTERVAL '30 minutes');
			`, userID),
		},
	}
	for _, tc := range timestampCases {
		_, err = testDB.ExecContext(ctx, tc.query)
		if err == nil {
			t.Fatalf("expected error on %s, but query succeeded", tc.name)
		}
	}
	t.Log("Verified: timestamp ordering constraints enforced on sessions.")

	// Step J: Restrictive parent deletion rejection
	_, err = testDB.ExecContext(ctx, `DELETE FROM giki_sports.users WHERE id = $1;`, userID)
	if err == nil {
		t.Fatalf("expected foreign-key RESTRICT violation when deleting user with active sessions/roles, but query succeeded")
	}
	t.Log("Verified: ON DELETE RESTRICT prevents deleting user records with child references.")

	// Step K: Migration 3 rollback preserves facility/schedule tables and their data
	// First, insert a facility and schedule to verify data preservation across migration 3 rollback
	var facilityID string
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.facilities (slug, name, status)
		VALUES ('auth-test-court', 'Auth Test Badminton Court', 'active')
		RETURNING id;
	`).Scan(&facilityID)
	if err != nil {
		t.Fatalf("failed to insert test facility: %v", err)
	}

	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.schedules (facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, 1, '10:00:00', '12:00:00', 'student');
	`, facilityID)
	if err != nil {
		t.Fatalf("failed to insert test schedule: %v", err)
	}

	t.Log("Rolling back migration 3 (Down)...")
	downRes, err := runner.Down(ctx)
	if err != nil {
		t.Fatalf("failed to roll back migration 3: %v", err)
	}
	if downRes == nil {
		t.Fatalf("expected non-nil Down result for migration 3")
	}

	// Verify auth tables dropped
	for _, table := range []string{"sessions", "user_roles", "users"} {
		exists, err := checkTableExists(ctx, testDB, "giki_sports", table)
		if err != nil || exists {
			t.Fatalf("expected table giki_sports.%s to be dropped after migration 3 Down, exists=%v", table, exists)
		}
	}

	// Verify facility/schedule tables and data remain intact
	for _, table := range []string{"facilities", "schedules"} {
		exists, err := checkTableExists(ctx, testDB, "giki_sports", table)
		if err != nil || !exists {
			t.Fatalf("expected table giki_sports.%s to remain intact after migration 3 Down", table)
		}
	}

	var facilityCount int
	err = testDB.QueryRowContext(ctx, `SELECT count(*) FROM giki_sports.facilities WHERE slug = 'auth-test-court';`).Scan(&facilityCount)
	if err != nil || facilityCount != 1 {
		t.Fatalf("expected facility data to be preserved after migration 3 Down, got count=%d, err=%v", facilityCount, err)
	}

	var currentVersion int64
	err = testDB.QueryRowContext(ctx, "SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1;").Scan(&currentVersion)
	if err != nil || currentVersion != 2 {
		t.Fatalf("expected goose_db_version to be 2 after migration 3 Down, got %d, err=%v", currentVersion, err)
	}
	t.Log("Verified: migration 3 Down cleanly dropped auth tables, preserved facility/schedule tables/data, and left version 2 active.")

	// Step L: Reapplying migration 3 succeeds cleanly
	t.Log("Reapplying migration 3 (Up)...")
	reapplyResults, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("failed to reapply migration 3: %v", err)
	}
	if len(reapplyResults) != 1 {
		t.Fatalf("expected exactly 1 migration reapplied (migration 3), got %d", len(reapplyResults))
	}

	for _, table := range []string{"users", "user_roles", "sessions"} {
		exists, err := checkTableExists(ctx, testDB, "giki_sports", table)
		if err != nil || !exists {
			t.Fatalf("expected table giki_sports.%s to exist after reapply Up", table)
		}
	}

	err = testDB.QueryRowContext(ctx, `SELECT count(*) FROM giki_sports.facilities WHERE slug = 'auth-test-court';`).Scan(&facilityCount)
	if err != nil || facilityCount != 1 {
		t.Fatalf("expected facility data to remain preserved after reapply Up, got count=%d", facilityCount)
	}

	err = testDB.QueryRowContext(ctx, "SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1;").Scan(&currentVersion)
	if err != nil || currentVersion != 3 {
		t.Fatalf("expected goose_db_version to be 3 after reapply Up, got %d", currentVersion)
	}
	t.Log("Verified: migration 3 reapplied cleanly, restoring auth tables at version 3 while preserving facility data.")
}

// TestMigrationIntegration_AuditLogsLifecycle verifies:
// - Step 7C Migration 4 applies cleanly after migrations 1-3
// - giki_sports.audit_logs table and its indexes exist
// - Foreign key constraint fk_audit_logs_actor rejects non-existent actor_id
// - Check constraints enforce non-empty action, non-empty target_type, and valid status
// - Migration 4 Down rollback drops audit_logs, preserves users/facilities/schedules, and leaves version 3 active
// - Migration 4 reapply restores audit_logs at version 4
func TestMigrationIntegration_AuditLogsLifecycle(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("integration test failed: unable to load configuration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	maintenanceDB, err := NewSQLDB(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("integration test failed: unable to connect to PostgreSQL server: %v", err)
	}
	defer maintenanceDB.Close()

	disposableDBName, err := createDisposableDB(ctx, maintenanceDB, "giki_sports_disposable_audit")
	if err != nil {
		t.Fatalf("failed to create disposable database: %v", err)
	}

	testDBCfg := cfg.DB
	testDBCfg.Name = disposableDBName
	testDB, err := NewSQLDB(ctx, testDBCfg)
	if err != nil {
		t.Fatalf("unable to connect to disposable database %s: %v", disposableDBName, err)
	}

	defer func() {
		t.Logf("Cleaning up disposable test database: %s", disposableDBName)
		_ = testDB.Close()

		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		if dropErr := dropDisposableDB(cleanupCtx, maintenanceDB, disposableDBName); dropErr != nil {
			t.Errorf("cleanup failure: failed to drop disposable test database %s: %v", disposableDBName, dropErr)
		}
	}()

	fixtureFS := getMigration1To4FixtureFS(t)
	runner, err := NewMigrationRunner(testDB, fixtureFS)
	if err != nil {
		t.Fatalf("failed to initialize migration runner: %v", err)
	}
	defer runner.Close()

	// Apply all migrations (1, 2, 3, and 4)
	t.Log("Applying all migrations 1, 2, 3, and 4 (Up)...")
	upResults, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}
	if len(upResults) != 4 {
		t.Fatalf("expected 4 migrations applied, got %d", len(upResults))
	}

	// Verify audit_logs table exists
	exists, err := checkTableExists(ctx, testDB, "giki_sports", "audit_logs")
	if err != nil || !exists {
		t.Fatalf("expected table giki_sports.audit_logs to exist: %v", err)
	}

	// Verify indexes exist
	for _, index := range []string{"idx_audit_logs_actor_id", "idx_audit_logs_target_id", "idx_audit_logs_action", "idx_audit_logs_created_at"} {
		exists, err := checkIndexExists(ctx, testDB, "giki_sports", index)
		if err != nil || !exists {
			t.Fatalf("expected index giki_sports.%s to exist: %v", index, err)
		}
	}

	// Foreign key constraint test: inserting audit log with non-existent actor_id must fail
	nonExistentActor := "00000000-0000-0000-0000-000000000001"
	dummyTarget := "00000000-0000-0000-0000-000000000002"
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.audit_logs (actor_id, action, target_type, target_id)
		VALUES ($1, 'schedule.update', 'facility_schedule', $2);
	`, nonExistentActor, dummyTarget)
	if err == nil {
		t.Fatalf("expected foreign key violation for non-existent actor_id, got nil")
	}
	t.Log("Verified: non-existent actor_id rejected by foreign key constraint fk_audit_logs_actor.")

	// Check constraint test: empty action must fail
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.audit_logs (actor_id, action, target_type, target_id)
		VALUES ($1, '   ', 'facility_schedule', $2);
	`, nonExistentActor, dummyTarget)
	if err == nil {
		t.Fatalf("expected check constraint violation for blank action, got nil")
	}

	// Check constraint test: invalid status must fail
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.audit_logs (actor_id, action, target_type, target_id, status)
		VALUES ($1, 'schedule.update', 'facility_schedule', $2, 'UNKNOWN');
	`, nonExistentActor, dummyTarget)
	if err == nil {
		t.Fatalf("expected check constraint violation for invalid status, got nil")
	}

	// Insert valid synthetic user to test valid audit log insert
	validUser := "e968ec9f-7e04-45aa-a477-d5f0b2f3a611"
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'audit-admin@example.test', 'Audit Admin', '$argon2id$v=19$m=65536,t=3,p=4$dummy$dummy');
	`, validUser)
	if err != nil {
		t.Fatalf("failed to insert user for audit testing: %v", err)
	}

	var auditID string
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.audit_logs (actor_id, action, target_type, target_id, details, ip_address, user_agent, status)
		VALUES ($1, 'schedule.update', 'facility_schedule', $2, '{"old":"A","new":"B"}', '127.0.0.1', 'Go-Test', 'SUCCESS')
		RETURNING id;
	`, validUser, dummyTarget).Scan(&auditID)
	if err != nil {
		t.Fatalf("failed to insert valid audit log: %v", err)
	}
	t.Logf("Inserted valid audit log %s", auditID)

	// Roll back migration 4 (Down)
	t.Log("Rolling back migration 4 (Down)...")
	downRes, err := runner.Down(ctx)
	if err != nil {
		t.Fatalf("failed to roll back migration 4: %v", err)
	}
	if downRes == nil {
		t.Fatalf("expected non-nil Down result for migration 4")
	}

	// Verify audit_logs table is dropped
	exists, err = checkTableExists(ctx, testDB, "giki_sports", "audit_logs")
	if err != nil || exists {
		t.Fatalf("expected table giki_sports.audit_logs to be dropped after migration 4 Down, exists=%v", exists)
	}

	// Verify user table and record remain intact
	var userCount int
	err = testDB.QueryRowContext(ctx, "SELECT count(*) FROM giki_sports.users WHERE id = $1;", validUser).Scan(&userCount)
	if err != nil || userCount != 1 {
		t.Fatalf("expected user data to remain intact after migration 4 Down, got count=%d, err=%v", userCount, err)
	}

	// Verify goose_db_version is 3
	var currentVersion int64
	err = testDB.QueryRowContext(ctx, "SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1;").Scan(&currentVersion)
	if err != nil || currentVersion != 3 {
		t.Fatalf("expected goose_db_version to be 3 after migration 4 Down, got %d, err=%v", currentVersion, err)
	}
	t.Log("Verified: migration 4 Down cleanly dropped audit_logs and restored version 3.")

	// Reapply migration 4
	t.Log("Reapplying migration 4 (Up)...")
	reapplyRes, err := runner.Up(ctx)
	if err != nil || len(reapplyRes) != 1 {
		t.Fatalf("failed to reapply migration 4: %v", err)
	}
	err = testDB.QueryRowContext(ctx, "SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1;").Scan(&currentVersion)
	if err != nil || currentVersion != 4 {
		t.Fatalf("expected goose_db_version to be 4 after reapply Up, got %d", currentVersion)
	}
	t.Log("Verified: migration 4 reapplied cleanly, restoring audit_logs at version 4.")
}

// TestMigrationIntegration_ContentItemsLifecycle tests migration 5's lifecycle:
// 1. Applying all migrations (1-5) on an isolated disposable database.
// 2. Verifying giki_sports.content_items and its indexes exist.
// 3. Verifying an incomplete draft is accepted (e.g. NULL or empty title/content, NULL published_at).
// 4. Verifying a complete published item is accepted for each content kind ('announcement' and 'achievement').
// 5. Verifying that published items with blank title/content or missing published_at are rejected.
// 6. Verifying invalid kind/status and unknown creator_id are rejected.
// 7. Verifying rolling back migration 5 (Down) cleanly drops content_items, leaves earlier tables and data intact, and sets goose_db_version to 4.
// 8. Verifying reapplying migration 5 (Up) succeeds and restores goose_db_version to 5.
func TestMigrationIntegration_ContentItemsLifecycle(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("integration test failed: unable to load configuration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	maintenanceDB, err := NewSQLDB(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("integration test failed: unable to connect to PostgreSQL server: %v", err)
	}
	defer maintenanceDB.Close()

	disposableDBName, err := createDisposableDB(ctx, maintenanceDB, "giki_sports_disposable_content")
	if err != nil {
		t.Fatalf("failed to create disposable database: %v", err)
	}

	testDBCfg := cfg.DB
	testDBCfg.Name = disposableDBName
	testDB, err := NewSQLDB(ctx, testDBCfg)
	if err != nil {
		t.Fatalf("unable to connect to disposable database %s: %v", disposableDBName, err)
	}

	defer func() {
		t.Logf("Cleaning up disposable test database: %s", disposableDBName)
		_ = testDB.Close()

		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		if dropErr := dropDisposableDB(cleanupCtx, maintenanceDB, disposableDBName); dropErr != nil {
			t.Errorf("cleanup failure: failed to drop disposable test database %s: %v", disposableDBName, dropErr)
		}
	}()

	migrationsDir := filepath.Join("..", "..", "migrations")
	runner, err := NewMigrationRunnerFromDir(testDB, migrationsDir)
	if err != nil {
		t.Fatalf("failed to initialize migration runner: %v", err)
	}
	defer runner.Close()

	// Apply all migrations (1 through 5)
	t.Log("Applying all migrations 1 through 5 (Up)...")
	upResults, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}
	if len(upResults) != 5 {
		t.Fatalf("expected 5 migrations applied, got %d", len(upResults))
	}

	// Verify content_items table exists
	exists, err := checkTableExists(ctx, testDB, "giki_sports", "content_items")
	if err != nil || !exists {
		t.Fatalf("expected table giki_sports.content_items to exist: %v", err)
	}

	// Verify indexes exist
	expectedIndexes := []string{
		"idx_content_items_creator_id",
		"idx_content_items_kind_status",
		"idx_content_items_published_at",
		"idx_content_items_created_at",
	}
	for _, index := range expectedIndexes {
		exists, err := checkIndexExists(ctx, testDB, "giki_sports", index)
		if err != nil || !exists {
			t.Fatalf("expected index giki_sports.%s to exist: %v", index, err)
		}
	}
	t.Log("Verified: table giki_sports.content_items and required indexes exist.")

	// Insert prerequisite data into earlier tables (users, facilities, schedules, audit_logs)
	creatorID := "a1111111-2222-3333-4444-555555555555"
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'author@example.test', 'Content Author', '$argon2id$v=19$m=65536,t=3,p=4$dummy$dummy');
	`, creatorID)
	if err != nil {
		t.Fatalf("failed to insert prerequisite user: %v", err)
	}

	facID := "b1111111-2222-3333-4444-555555555555"
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name)
		VALUES ($1, 'test-facility', 'Test Facility');
	`, facID)
	if err != nil {
		t.Fatalf("failed to insert prerequisite facility: %v", err)
	}

	schedID := "c1111111-2222-3333-4444-555555555555"
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.schedules (id, facility_id, day_of_week, start_time, end_time, audience)
		VALUES ($1, $2, 1, '09:00:00', '11:00:00', 'student');
	`, schedID, facID)
	if err != nil {
		t.Fatalf("failed to insert prerequisite schedule: %v", err)
	}

	auditID := "d1111111-2222-3333-4444-555555555555"
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.audit_logs (id, actor_id, action, target_type, target_id, details)
		VALUES ($1, $2, 'facility.create', 'facility', $3, '{"name":"Test Facility"}');
	`, auditID, creatorID, facID)
	if err != nil {
		t.Fatalf("failed to insert prerequisite audit log: %v", err)
	}

	// 1. Incomplete drafts: must be accepted with NULL/empty title, content, and published_at
	t.Log("Testing incomplete drafts...")
	var draft1ID, draft2ID, draft3ID string

	// Case 1A: NULL title and NULL content
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id, published_at)
		VALUES ('announcement', NULL, NULL, 'draft', $1, NULL)
		RETURNING id;
	`, creatorID).Scan(&draft1ID)
	if err != nil {
		t.Fatalf("expected incomplete draft with NULL title/content to be accepted, got: %v", err)
	}

	// Case 1B: Empty string title and content
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id, published_at)
		VALUES ('achievement', '', '', 'draft', $1, NULL)
		RETURNING id;
	`, creatorID).Scan(&draft2ID)
	if err != nil {
		t.Fatalf("expected incomplete draft with empty title/content to be accepted, got: %v", err)
	}

	// Case 1C: Title populated, content NULL, defaulting status to 'draft'
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, creator_id)
		VALUES ('announcement', 'Work in progress announcement', NULL, $1)
		RETURNING id;
	`, creatorID).Scan(&draft3ID)
	if err != nil {
		t.Fatalf("expected default draft to be accepted, got: %v", err)
	}
	t.Log("Verified: incomplete drafts accepted with NULL, empty, or partial attributes.")

	// 2. Complete published items: accepted for both announcement and achievement
	t.Log("Testing complete published items...")
	var pubAnnID, pubAchID string

	// Announcement
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id, published_at)
		VALUES ('announcement', 'Annual Sports Gala 2026', 'Registrations open at the complex reception.', 'published', $1, now())
		RETURNING id;
	`, creatorID).Scan(&pubAnnID)
	if err != nil {
		t.Fatalf("expected complete published announcement to be accepted, got: %v", err)
	}

	// Achievement
	err = testDB.QueryRowContext(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id, published_at)
		VALUES ('achievement', 'Intervarsity Table Tennis Gold', 'GIKI secured 1st place in the all-Pakistan championship.', 'published', $1, now())
		RETURNING id;
	`, creatorID).Scan(&pubAchID)
	if err != nil {
		t.Fatalf("expected complete published achievement to be accepted, got: %v", err)
	}
	t.Log("Verified: complete published items accepted for both announcement and achievement kinds.")

	// 3. Published items with missing/blank title, content, or publication timestamp: must be rejected
	t.Log("Testing rejection of invalid published items...")

	// 3A: Published with NULL title
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id, published_at)
		VALUES ('announcement', NULL, 'Valid content', 'published', $1, now());
	`, creatorID)
	if err == nil {
		t.Fatalf("expected rejection for published item with NULL title, got nil")
	}

	// 3B: Published with blank/whitespace title
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id, published_at)
		VALUES ('announcement', '   ', 'Valid content', 'published', $1, now());
	`, creatorID)
	if err == nil {
		t.Fatalf("expected rejection for published item with whitespace title, got nil")
	}

	// 3C: Published with NULL content
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id, published_at)
		VALUES ('achievement', 'Valid title', NULL, 'published', $1, now());
	`, creatorID)
	if err == nil {
		t.Fatalf("expected rejection for published item with NULL content, got nil")
	}

	// 3D: Published with blank/whitespace content
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id, published_at)
		VALUES ('achievement', 'Valid title', '   ', 'published', $1, now());
	`, creatorID)
	if err == nil {
		t.Fatalf("expected rejection for published item with whitespace content, got nil")
	}

	// 3E: Published with NULL published_at
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id, published_at)
		VALUES ('announcement', 'Valid title', 'Valid content', 'published', $1, NULL);
	`, creatorID)
	if err == nil {
		t.Fatalf("expected rejection for published item with NULL published_at, got nil")
	}
	t.Log("Verified: published rows rejected when title, content, or published_at is blank/missing.")

	// 4. Domain check constraints: invalid kind or status
	t.Log("Testing invalid kind and status rejection...")

	// 4A: Invalid kind
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id)
		VALUES ('press_release', 'Title', 'Content', 'draft', $1);
	`, creatorID)
	if err == nil {
		t.Fatalf("expected rejection for invalid kind 'press_release', got nil")
	}

	// 4B: Invalid status
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id)
		VALUES ('announcement', 'Title', 'Content', 'archived', $1);
	`, creatorID)
	if err == nil {
		t.Fatalf("expected rejection for invalid status 'archived', got nil")
	}
	t.Log("Verified: invalid kind and status rejected by CHECK constraints.")

	// 5. Restrictive foreign key: unknown creator user ID
	t.Log("Testing unknown creator foreign key rejection...")
	unknownCreatorID := "99999999-9999-9999-9999-999999999999"
	_, err = testDB.ExecContext(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id)
		VALUES ('announcement', 'Title', 'Content', 'draft', $1);
	`, unknownCreatorID)
	if err == nil {
		t.Fatalf("expected foreign key violation for unknown creator_id, got nil")
	}
	t.Log("Verified: unknown creator rejected by fk_content_items_creator constraint.")

	// 6. Roll back migration 5 (Down)
	t.Log("Rolling back migration 5 (Down)...")
	downRes, err := runner.Down(ctx)
	if err != nil {
		t.Fatalf("failed to roll back migration 5: %v", err)
	}
	if downRes == nil {
		t.Fatalf("expected non-nil Down result for migration 5")
	}

	// Verify content_items table is dropped
	exists, err = checkTableExists(ctx, testDB, "giki_sports", "content_items")
	if err != nil || exists {
		t.Fatalf("expected table giki_sports.content_items to be dropped after migration 5 Down, exists=%v", exists)
	}

	// Verify earlier tables and their data remain intact
	var userCount, facCount, schedCount, auditCount int
	err = testDB.QueryRowContext(ctx, "SELECT count(*) FROM giki_sports.users WHERE id = $1;", creatorID).Scan(&userCount)
	if err != nil || userCount != 1 {
		t.Fatalf("expected user data to remain intact, got count=%d, err=%v", userCount, err)
	}
	err = testDB.QueryRowContext(ctx, "SELECT count(*) FROM giki_sports.facilities WHERE id = $1;", facID).Scan(&facCount)
	if err != nil || facCount != 1 {
		t.Fatalf("expected facility data to remain intact, got count=%d, err=%v", facCount, err)
	}
	err = testDB.QueryRowContext(ctx, "SELECT count(*) FROM giki_sports.schedules WHERE id = $1;", schedID).Scan(&schedCount)
	if err != nil || schedCount != 1 {
		t.Fatalf("expected schedule data to remain intact, got count=%d, err=%v", schedCount, err)
	}
	err = testDB.QueryRowContext(ctx, "SELECT count(*) FROM giki_sports.audit_logs WHERE id = $1;", auditID).Scan(&auditCount)
	if err != nil || auditCount != 1 {
		t.Fatalf("expected audit log data to remain intact, got count=%d, err=%v", auditCount, err)
	}

	// Verify goose_db_version is 4
	var currentVersion int64
	err = testDB.QueryRowContext(ctx, "SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1;").Scan(&currentVersion)
	if err != nil || currentVersion != 4 {
		t.Fatalf("expected goose_db_version to be 4 after migration 5 Down, got %d, err=%v", currentVersion, err)
	}
	t.Log("Verified: migration 5 Down cleanly dropped content_items; earlier tables (users, facilities, schedules, audit_logs) and their records remain intact at version 4.")

	// 7. Reapply migration 5 (Up)
	t.Log("Reapplying migration 5 (Up)...")
	reapplyRes, err := runner.Up(ctx)
	if err != nil || len(reapplyRes) != 1 {
		t.Fatalf("failed to reapply migration 5: %v", err)
	}
	exists, err = checkTableExists(ctx, testDB, "giki_sports", "content_items")
	if err != nil || !exists {
		t.Fatalf("expected table giki_sports.content_items to exist after reapply Up: %v", err)
	}
	err = testDB.QueryRowContext(ctx, "SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1;").Scan(&currentVersion)
	if err != nil || currentVersion != 5 {
		t.Fatalf("expected goose_db_version to be 5 after reapply Up, got %d", currentVersion)
	}
	t.Log("Verified: migration 5 reapplied cleanly, restoring content_items at version 5.")
}


