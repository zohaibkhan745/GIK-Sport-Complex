//go:build integration

package padel

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/database"
)

func createDisposableDB(ctx context.Context, maintenanceDB *sql.DB, prefix string) (string, error) {
	dbName := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	_, err := maintenanceDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s;", dbName))
	if err != nil {
		return "", fmt.Errorf("failed to create disposable database %s: %w", dbName, err)
	}
	return dbName, nil
}

func dropDisposableDB(ctx context.Context, maintenanceDB *sql.DB, dbName string) error {
	if !strings.HasPrefix(dbName, "giki_sports_disposable_padel_repo") {
		return fmt.Errorf("safety check violated: refusing to drop database '%s' without expected prefix", dbName)
	}

	_, _ = maintenanceDB.ExecContext(ctx, fmt.Sprintf(`
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = '%s' AND pid <> pg_backend_pid();`, dbName))

	_, err := maintenanceDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE);", dbName))
	return err
}

func setupPadelIntegrationTest(t *testing.T) (*PostgresRepository, *pgxpool.Pool, func()) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load configuration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	maintenanceDB, err := database.NewSQLDB(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("failed to connect to maintenance db: %v", err)
	}

	dbName, err := createDisposableDB(ctx, maintenanceDB, "giki_sports_disposable_padel_repo")
	if err != nil {
		maintenanceDB.Close()
		t.Fatalf("failed to create disposable db: %v", err)
	}

	testDBCfg := cfg.DB
	testDBCfg.Name = dbName
	testDBCfg.MaxConns = 60

	migDB, err := database.NewSQLDB(ctx, testDBCfg)
	if err != nil {
		_ = dropDisposableDB(ctx, maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to connect migDB: %v", err)
	}

	migrationsDir := filepath.Join("..", "..", "migrations")
	runner, err := database.NewMigrationRunnerFromDir(migDB, migrationsDir)
	if err != nil {
		migDB.Close()
		_ = dropDisposableDB(ctx, maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to create migration runner: %v", err)
	}

	_, err = runner.Up(ctx)
	if err != nil {
		migDB.Close()
		_ = dropDisposableDB(ctx, maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to apply migrations 1-7: %v", err)
	}
	migDB.Close()

	pool, err := database.NewPool(ctx, testDBCfg)
	if err != nil {
		_ = dropDisposableDB(ctx, maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to create pgxpool: %v", err)
	}

	repo := NewRepository(pool)

	cleanup := func() {
		pool.Close()
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_ = dropDisposableDB(dropCtx, maintenanceDB, dbName)
		maintenanceDB.Close()
	}

	return repo, pool, cleanup
}

func TestPadelRepository_FindSlotsAndBatchCreate(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Seed test facility
	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000001")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-court-1', 'Campus Padel Court 1', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Hour)
	start1 := now.Add(24 * time.Hour)
	end1 := start1.Add(60 * time.Minute)
	start2 := end1
	end2 := start2.Add(60 * time.Minute)

	slots := []Slot{
		{
			ID:         uuid.New(),
			FacilityID: facID,
			StartTime:  start1,
			EndTime:    end1,
			Status:     SlotStatusAvailable,
			PricePaisa: 100000,
		},
		{
			ID:         uuid.New(),
			FacilityID: facID,
			StartTime:  start2,
			EndTime:    end2,
			Status:     SlotStatusAvailable,
			PricePaisa: 120000,
		},
	}

	t.Run("batch create inserts new slots", func(t *testing.T) {
		created, skipped, err := repo.BatchCreateSlots(ctx, slots)
		if err != nil {
			t.Fatalf("batch create failed: %v", err)
		}
		if created != 2 || skipped != 0 {
			t.Fatalf("expected created=2, skipped=0; got created=%d, skipped=%d", created, skipped)
		}
	})

	t.Run("batch create on conflict skips duplicates cleanly", func(t *testing.T) {
		created, skipped, err := repo.BatchCreateSlots(ctx, slots)
		if err != nil {
			t.Fatalf("batch create on duplicate failed: %v", err)
		}
		if created != 0 || skipped != 2 {
			t.Fatalf("expected created=0, skipped=2; got created=%d, skipped=%d", created, skipped)
		}
	})

	t.Run("find slots retrieves stored slots ordered by start_time", func(t *testing.T) {
		found, err := repo.FindSlots(ctx, facID, start1.Add(-1*time.Hour), end2.Add(1*time.Hour))
		if err != nil {
			t.Fatalf("find slots failed: %v", err)
		}
		if len(found) != 2 {
			t.Fatalf("expected 2 slots, got %d", len(found))
		}
		if found[0].PricePaisa != 100000 || found[1].PricePaisa != 120000 {
			t.Errorf("prices do not match expected: %d, %d", found[0].PricePaisa, found[1].PricePaisa)
		}
	})
}

func TestPadelRepository_LazyHoldAndCleanExpiredHolds(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000001")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-court-1', 'Campus Padel Court 1', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	userID := uuid.MustParse("a0000000-0000-0000-0000-000000000001")
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice@example.test', 'Alice Student', 'fakehash', 'active');
	`, userID)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	slotID := uuid.New()
	now := time.Now().UTC()
	start := now.Add(2 * time.Hour)
	end := start.Add(60 * time.Minute)
	pastHoldExpiry := now.Add(-2 * time.Minute)

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.padel_slots (id, facility_id, start_time, end_time, status, holder_id, hold_expires_at, price_paisa)
		VALUES ($1, $2, $3, $4, 'HELD', $5, $6, 100000);
	`, slotID, facID, start, end, userID, pastHoldExpiry)
	if err != nil {
		t.Fatalf("failed to insert held slot: %v", err)
	}

	t.Run("querying slot shows effective status AVAILABLE via lazy expiration", func(t *testing.T) {
		s, err := repo.GetSlotByID(ctx, slotID)
		if err != nil {
			t.Fatalf("failed to get slot: %v", err)
		}
		if s.Status != SlotStatusHeld {
			t.Errorf("expected raw status in DB to be HELD, got %s", s.Status)
		}
		if s.EffectiveStatus(now) != SlotStatusAvailable {
			t.Errorf("expected effective status to evaluate to AVAILABLE, got %s", s.EffectiveStatus(now))
		}
	})

	t.Run("CleanExpiredHolds resets slot to AVAILABLE in DB", func(t *testing.T) {
		cleanedCount, err := repo.CleanExpiredHolds(ctx, now)
		if err != nil {
			t.Fatalf("clean expired holds failed: %v", err)
		}
		if cleanedCount != 1 {
			t.Fatalf("expected 1 hold cleaned, got %d", cleanedCount)
		}

		s, err := repo.GetSlotByID(ctx, slotID)
		if err != nil {
			t.Fatalf("failed to get slot after clean: %v", err)
		}
		if s.Status != SlotStatusAvailable {
			t.Errorf("expected DB status to be AVAILABLE, got %s", s.Status)
		}
		if s.HolderID != nil || s.HoldExpiresAt != nil {
			t.Errorf("expected holderID and holdExpiresAt to be cleared")
		}
	})
}

func TestPadelRepository_UpdateSlotPriceAndStatus(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000001")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-court-1', 'Campus Padel Court 1', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	slotID := uuid.New()
	start := time.Now().UTC().Add(5 * time.Hour)
	end := start.Add(60 * time.Minute)

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.padel_slots (id, facility_id, start_time, end_time, status, price_paisa)
		VALUES ($1, $2, $3, $4, 'AVAILABLE', 100000);
	`, slotID, facID, start, end)
	if err != nil {
		t.Fatalf("failed to insert available slot: %v", err)
	}

	t.Run("update price and status to BLOCKED", func(t *testing.T) {
		newPrice := int64(140000)
		newStatus := SlotStatusBlocked
		updated, err := repo.UpdateSlot(ctx, slotID, &newPrice, &newStatus)
		if err != nil {
			t.Fatalf("update slot failed: %v", err)
		}
		if updated.PricePaisa != 140000 || updated.Status != SlotStatusBlocked {
			t.Errorf("expected price 140000 and status BLOCKED, got %d, %s", updated.PricePaisa, updated.Status)
		}
	})

	t.Run("updating booked slot is rejected with ErrSlotBooked", func(t *testing.T) {
		// Set status to BOOKED directly
		_, err := pool.Exec(ctx, `UPDATE giki_sports.padel_slots SET status = 'BOOKED' WHERE id = $1;`, slotID)
		if err != nil {
			t.Fatalf("failed to set slot to BOOKED: %v", err)
		}

		newPrice := int64(200000)
		_, err = repo.UpdateSlot(ctx, slotID, &newPrice, nil)
		if err != ErrSlotBooked {
			t.Errorf("expected ErrSlotBooked when updating booked slot, got: %v", err)
		}
	})
}

func TestPadelRepository_HoldSlot_ConcurrencyAndExclusivity(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Seed facility
	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000001")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-court-1', 'Campus Padel Court 1', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	// 2. Seed Alice and Bob
	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000001")
	bobID := uuid.MustParse("a0000000-0000-0000-0000-000000000002")
	for _, u := range []struct {
		id    uuid.UUID
		email string
		name  string
	}{
		{aliceID, "alice@example.test", "Alice Student"},
		{bobID, "bob@example.test", "Bob Student"},
	} {
		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
			VALUES ($1, $2, $3, 'fakehash', 'active');
		`, u.id, u.email, u.name)
		if err != nil {
			t.Fatalf("failed to seed user %s: %v", u.name, err)
		}
	}

	// 3. Create test slot
	slotID := uuid.New()
	now := time.Now().UTC()
	start := now.Add(24 * time.Hour)
	end := start.Add(60 * time.Minute)

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.padel_slots (id, facility_id, start_time, end_time, status, price_paisa)
		VALUES ($1, $2, $3, $4, 'AVAILABLE', 100000);
	`, slotID, facID, start, end)
	if err != nil {
		t.Fatalf("failed to insert slot: %v", err)
	}

	t.Run("Alice successfully acquires hold on available slot", func(t *testing.T) {
		held, err := repo.HoldSlot(ctx, slotID, aliceID, now, DefaultHoldDuration)
		if err != nil {
			t.Fatalf("Alice failed to hold slot: %v", err)
		}
		if held.Status != SlotStatusHeld {
			t.Errorf("expected status HELD, got %s", held.Status)
		}
		if held.HolderID == nil || *held.HolderID != aliceID {
			t.Errorf("expected holder Alice, got %v", held.HolderID)
		}
		if held.HoldExpiresAt == nil || !held.HoldExpiresAt.After(now) {
			t.Errorf("expected hold_expires_at after now")
		}
	})

	t.Run("Bob is rejected with ErrSlotConflict while Alice's hold is active", func(t *testing.T) {
		_, err := repo.HoldSlot(ctx, slotID, bobID, now.Add(1*time.Minute), DefaultHoldDuration)
		if err != ErrSlotConflict {
			t.Errorf("expected ErrSlotConflict for Bob, got: %v", err)
		}
	})

	t.Run("Alice re-holding extends her hold timestamp", func(t *testing.T) {
		renewTime := now.Add(2 * time.Minute)
		renewed, err := repo.HoldSlot(ctx, slotID, aliceID, renewTime, DefaultHoldDuration)
		if err != nil {
			t.Fatalf("Alice re-hold failed: %v", err)
		}
		expectedExpiry := renewTime.Add(DefaultHoldDuration)
		if renewed.HoldExpiresAt == nil || !renewed.HoldExpiresAt.Truncate(time.Millisecond).Equal(expectedExpiry.Truncate(time.Millisecond)) {
			t.Errorf("expected expiry %v, got %v", expectedExpiry, renewed.HoldExpiresAt)
		}
	})

	t.Run("Bob cannot release Alice's hold", func(t *testing.T) {
		_, err := repo.ReleaseHold(ctx, slotID, bobID)
		if err != ErrHoldNotOwned {
			t.Errorf("expected ErrHoldNotOwned, got: %v", err)
		}
	})

	t.Run("Alice releases hold, returning slot to AVAILABLE", func(t *testing.T) {
		released, err := repo.ReleaseHold(ctx, slotID, aliceID)
		if err != nil {
			t.Fatalf("Alice failed to release hold: %v", err)
		}
		if released.Status != SlotStatusAvailable {
			t.Errorf("expected status AVAILABLE, got %s", released.Status)
		}
		if released.HolderID != nil || released.HoldExpiresAt != nil {
			t.Errorf("expected holder and expiry to be nil after release")
		}
	})

	t.Run("Lazy expiration allows new contender to acquire hold without background cleaner", func(t *testing.T) {
		// Put slot into HELD state with past expiry
		pastTime := now.Add(-10 * time.Minute)
		pastExpiry := pastTime.Add(5 * time.Minute)
		_, err := pool.Exec(ctx, `
			UPDATE giki_sports.padel_slots
			SET status = 'HELD', holder_id = $1, hold_expires_at = $2
			WHERE id = $3;
		`, aliceID, pastExpiry, slotID)
		if err != nil {
			t.Fatalf("failed to set expired hold: %v", err)
		}

		// Bob requests hold at current time -> lazy expiration grants hold to Bob!
		heldByBob, err := repo.HoldSlot(ctx, slotID, bobID, now, DefaultHoldDuration)
		if err != nil {
			t.Fatalf("Bob failed to acquire hold on lazily expired slot: %v", err)
		}
		if heldByBob.HolderID == nil || *heldByBob.HolderID != bobID {
			t.Errorf("expected holder Bob, got %v", heldByBob.HolderID)
		}
		if heldByBob.Status != SlotStatusHeld {
			t.Errorf("expected status HELD, got %s", heldByBob.Status)
		}
	})

	t.Run("TC-QR-01-Hold: 50 concurrent contenders for one slot produces exactly 1 winner and 49 conflicts", func(t *testing.T) {
		// Reset slot to AVAILABLE
		_, err := pool.Exec(ctx, `
			UPDATE giki_sports.padel_slots
			SET status = 'AVAILABLE', holder_id = NULL, hold_expires_at = NULL
			WHERE id = $1;
		`, slotID)
		if err != nil {
			t.Fatalf("failed to reset slot: %v", err)
		}

		// Seed 50 distinct contender users
		contenders := make([]uuid.UUID, 50)
		for i := 0; i < 50; i++ {
			cID := uuid.New()
			contenders[i] = cID
			_, err = pool.Exec(ctx, `
				INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
				VALUES ($1, $2, $3, 'fakehash', 'active');
			`, cID, fmt.Sprintf("contender%d@example.test", i), fmt.Sprintf("Contender %d", i))
			if err != nil {
				t.Fatalf("failed to seed contender %d: %v", i, err)
			}
		}

		var successes int64
		var conflicts int64
		var otherErrors int64

		var wg sync.WaitGroup
		startGate := make(chan struct{})

		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(contenderID uuid.UUID) {
				defer wg.Done()
				<-startGate // Synchronize simultaneous execution

				holdCtx, holdCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer holdCancel()

				_, err := repo.HoldSlot(holdCtx, slotID, contenderID, now, DefaultHoldDuration)
				if err == nil {
					atomic.AddInt64(&successes, 1)
				} else if err == ErrSlotConflict {
					atomic.AddInt64(&conflicts, 1)
				} else {
					atomic.AddInt64(&otherErrors, 1)
					t.Logf("contender %s received unexpected error: %v", contenderID, err)
				}
			}(contenders[i])
		}

		// Release all 50 goroutines simultaneously
		close(startGate)
		wg.Wait()

		if successes != 1 {
			t.Errorf("TC-QR-01-Hold violation: expected exactly 1 successful hold, got %d", successes)
		}
		if conflicts != 49 {
			t.Errorf("TC-QR-01-Hold violation: expected exactly 49 conflicts, got %d", conflicts)
		}
		if otherErrors != 0 {
			t.Errorf("unexpected errors among contenders: %d", otherErrors)
		}

		// Verify database state: status is HELD and holder_id is populated
		var dbStatus string
		var dbHolder uuid.UUID
		err = pool.QueryRow(ctx, "SELECT status, holder_id FROM giki_sports.padel_slots WHERE id = $1;", slotID).Scan(&dbStatus, &dbHolder)
		if err != nil {
			t.Fatalf("failed to query slot after race: %v", err)
		}
		if dbStatus != "HELD" {
			t.Errorf("expected slot in DB to be HELD, got %s", dbStatus)
		}
		if dbHolder == uuid.Nil {
			t.Errorf("expected valid holder UUID in DB, got nil")
		}
		t.Logf("Verified (TC-QR-01-Hold): Exactly 1 contender won the hold (holder: %s), 49 contenders received ErrSlotConflict.", dbHolder)
	})
}
