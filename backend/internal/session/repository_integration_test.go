//go:build integration

package session_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/database"
	"giki-sports-backend/internal/session"
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

	dbName := fmt.Sprintf("giki_sports_disposable_sessrepo_%d", time.Now().UnixNano())
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

// waitForBlockedSession explicitly verifies via pg_stat_activity that a database backend
// connection is actively waiting on a row lock held by blockerPID.
// Does NOT use arbitrary sleeps as evidence of ordering.
func waitForBlockedSession(ctx context.Context, pool *pgxpool.Pool, blockerPID uint32, timeout time.Duration) (uint32, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var waitingPID uint32
		err := pool.QueryRow(ctx, `
			SELECT pid
			FROM pg_stat_activity
			WHERE $1 = ANY(pg_blocking_pids(pid))
			  AND wait_event_type = 'Lock'
			LIMIT 1;
		`, blockerPID).Scan(&waitingPID)
		if err == nil {
			return waitingPID, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, err
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	return 0, fmt.Errorf("timeout (%v) waiting for transaction to block on lock held by PID %d", timeout, blockerPID)
}

func insertSyntheticUsers(t *testing.T, pool *pgxpool.Pool) (activeID, inactiveID uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	activeID = uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	inactiveID = uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES 
			('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', 'active@giki.edu.pk', 'Active User', '$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$c29tZWhhc2gx', 'active'),
			('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', 'inactive@giki.edu.pk', 'Inactive User', '$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$c29tZWhhc2gy', 'inactive');
	`)
	if err != nil {
		t.Fatalf("failed to insert synthetic users: %v", err)
	}

	return activeID, inactiveID
}

func TestPostgresSessionRepository_Integration(t *testing.T) {
	initialUsers, initialSessions := countDevRecords(t)

	pool, teardown := setupDisposableDB(t)
	defer teardown()

	activeUserID, inactiveUserID := insertSyntheticUsers(t, pool)
	repo := session.NewRepository(pool)
	ctx := context.Background()

	t.Run("ActiveUserCreationAndExactEightHourLifetime", func(t *testing.T) {
		tokenHash := strings.Repeat("1", 64)
		t0 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

		s, err := repo.CreateAt(ctx, activeUserID, tokenHash, t0)
		if err != nil {
			t.Fatalf("CreateAt failed for active user: %v", err)
		}

		if s.UserID != activeUserID {
			t.Errorf("expected UserID %s, got %s", activeUserID, s.UserID)
		}
		if !s.CreatedAt.Equal(t0) {
			t.Errorf("expected CreatedAt %v, got %v", t0, s.CreatedAt)
		}
		if !s.LastActivityAt.Equal(t0) {
			t.Errorf("expected LastActivityAt %v, got %v", t0, s.LastActivityAt)
		}
		expectedAbsExpiry := t0.Add(8 * time.Hour)
		if !s.AbsoluteExpiresAt.Equal(expectedAbsExpiry) {
			t.Errorf("expected AbsoluteExpiresAt %v, got %v", expectedAbsExpiry, s.AbsoluteExpiresAt)
		}
	})

	t.Run("UnknownAndInactiveUserCreationRejection", func(t *testing.T) {
		tokenHash := strings.Repeat("2", 64)
		t0 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

		// 1. Unknown user
		_, err := repo.CreateAt(ctx, uuid.New(), tokenHash, t0)
		if !errors.Is(err, session.ErrUserNotFoundOrInactive) {
			t.Errorf("expected ErrUserNotFoundOrInactive for unknown user, got %v", err)
		}

		// 2. Inactive user
		_, err = repo.CreateAt(ctx, inactiveUserID, tokenHash, t0)
		if !errors.Is(err, session.ErrUserNotFoundOrInactive) {
			t.Errorf("expected ErrUserNotFoundOrInactive for inactive user, got %v", err)
		}

		// 3. Nil UUID
		_, err = repo.CreateAt(ctx, uuid.Nil, tokenHash, t0)
		if !errors.Is(err, session.ErrUserNotFoundOrInactive) {
			t.Errorf("expected ErrUserNotFoundOrInactive for uuid.Nil, got %v", err)
		}
	})

	t.Run("SuccessfulValidationUpdatesActivityWithoutExtendingAbsoluteExpiry", func(t *testing.T) {
		tokenHash := strings.Repeat("3", 64)
		t0 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

		s, err := repo.CreateAt(ctx, activeUserID, tokenHash, t0)
		if err != nil {
			t.Fatalf("CreateAt failed: %v", err)
		}

		t1 := t0.Add(15 * time.Minute)
		validated, err := repo.ValidateAndTouchAt(ctx, tokenHash, t1)
		if err != nil {
			t.Fatalf("ValidateAndTouchAt failed: %v", err)
		}

		if !validated.LastActivityAt.Equal(t1) {
			t.Errorf("expected LastActivityAt updated to %v, got %v", t1, validated.LastActivityAt)
		}
		if !validated.AbsoluteExpiresAt.Equal(s.AbsoluteExpiresAt) {
			t.Errorf("absolute expiry was modified: expected %v, got %v", s.AbsoluteExpiresAt, validated.AbsoluteExpiresAt)
		}

		// Confirm in database
		rec, err := repo.GetSessionRecord(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetSessionRecord failed: %v", err)
		}
		if !rec.LastActivityAt.Equal(t1) {
			t.Errorf("persisted LastActivityAt expected %v, got %v", t1, rec.LastActivityAt)
		}
	})

	t.Run("IdleExpiryBoundaries_Before_Exact_After_AndLeavesActivityUnchanged", func(t *testing.T) {
		tokenHash := strings.Repeat("4", 64)
		t0 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

		_, err := repo.CreateAt(ctx, activeUserID, tokenHash, t0)
		if err != nil {
			t.Fatalf("CreateAt failed: %v", err)
		}

		// Initial touch at 10:10:00 (new base activity)
		tBase := t0.Add(10 * time.Minute)
		_, err = repo.ValidateAndTouchAt(ctx, tokenHash, tBase)
		if err != nil {
			t.Fatalf("touch at tBase failed: %v", err)
		}

		// 1. Just before idle expiry (1 microsecond before 30 min idle threshold)
		tBefore := tBase.Add(30*time.Minute - time.Microsecond)
		sBefore, err := repo.ValidateAndTouchAt(ctx, tokenHash, tBefore)
		if err != nil {
			t.Fatalf("expected validation just before idle expiry to succeed, got %v", err)
		}
		if !sBefore.LastActivityAt.Equal(tBefore) {
			t.Errorf("expected last activity updated to %v, got %v", tBefore, sBefore.LastActivityAt)
		}

		// Current last_activity_at in DB is now tBefore
		recBaseline, err := repo.GetSessionRecord(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetSessionRecord failed: %v", err)
		}

		// 2. Exactly at idle expiry (30 min after tBefore)
		tExact := tBefore.Add(30 * time.Minute)
		_, errExact := repo.ValidateAndTouchAt(ctx, tokenHash, tExact)
		if !errors.Is(errExact, session.ErrInvalidSession) {
			t.Errorf("expected ErrInvalidSession exactly at idle expiry, got %v", errExact)
		}

		// Verify rejected validation left activity UNCHANGED
		recAfterExact, err := repo.GetSessionRecord(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetSessionRecord failed: %v", err)
		}
		if !recAfterExact.LastActivityAt.Equal(recBaseline.LastActivityAt) {
			t.Errorf("rejected validation altered activity timestamp: expected %v, got %v", recBaseline.LastActivityAt, recAfterExact.LastActivityAt)
		}

		// 3. After idle expiry (1 microsecond after 30 min)
		tAfter := tBefore.Add(30*time.Minute + time.Microsecond)
		_, errAfter := repo.ValidateAndTouchAt(ctx, tokenHash, tAfter)
		if !errors.Is(errAfter, session.ErrInvalidSession) {
			t.Errorf("expected ErrInvalidSession after idle expiry, got %v", errAfter)
		}

		recAfterAfter, err := repo.GetSessionRecord(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetSessionRecord failed: %v", err)
		}
		if !recAfterAfter.LastActivityAt.Equal(recBaseline.LastActivityAt) {
			t.Errorf("rejected validation altered activity timestamp: expected %v, got %v", recBaseline.LastActivityAt, recAfterAfter.LastActivityAt)
		}
	})

	t.Run("AbsoluteExpiryBoundaries_Before_Exact_After", func(t *testing.T) {
		tokenHash := strings.Repeat("5", 64)
		t0 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

		s, err := repo.CreateAt(ctx, activeUserID, tokenHash, t0)
		if err != nil {
			t.Fatalf("CreateAt failed: %v", err)
		}

		absExpiry := s.AbsoluteExpiresAt // t0 + 8 hours

		// Keep session actively touched every 25 minutes (within 30m idle window) until 7h 45m
		curr := t0
		for curr.Before(t0.Add(7*time.Hour + 45*time.Minute)) {
			curr = curr.Add(25 * time.Minute)
			_, err = repo.ValidateAndTouchAt(ctx, tokenHash, curr)
			if err != nil {
				t.Fatalf("keep-alive touch at %v failed: %v", curr, err)
			}
		}

		// 1. Just before absolute expiry (1 microsecond before 8 hours)
		tBeforeAbs := absExpiry.Add(-time.Microsecond)
		_, errBefore := repo.ValidateAndTouchAt(ctx, tokenHash, tBeforeAbs)
		if errBefore != nil {
			t.Fatalf("expected validation just before absolute expiry to succeed, got %v", errBefore)
		}

		// 2. Exactly at absolute expiry (exactly 8 hours)
		_, errExactAbs := repo.ValidateAndTouchAt(ctx, tokenHash, absExpiry)
		if !errors.Is(errExactAbs, session.ErrInvalidSession) {
			t.Errorf("expected ErrInvalidSession exactly at absolute expiry, got %v", errExactAbs)
		}

		// 3. After absolute expiry
		_, errAfterAbs := repo.ValidateAndTouchAt(ctx, tokenHash, absExpiry.Add(time.Microsecond))
		if !errors.Is(errAfterAbs, session.ErrInvalidSession) {
			t.Errorf("expected ErrInvalidSession after absolute expiry, got %v", errAfterAbs)
		}
	})

	t.Run("Revocation_RepeatedRevocation_PreservesOriginalTimestamp_AndRejectsLaterValidation", func(t *testing.T) {
		tokenHash := strings.Repeat("6", 64)
		t0 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

		_, err := repo.CreateAt(ctx, activeUserID, tokenHash, t0)
		if err != nil {
			t.Fatalf("CreateAt failed: %v", err)
		}

		// First revocation at t0 + 5m
		tRevoke1 := t0.Add(5 * time.Minute)
		err = repo.RevokeAt(ctx, tokenHash, tRevoke1)
		if err != nil {
			t.Fatalf("RevokeAt failed: %v", err)
		}

		rec1, err := repo.GetSessionRecord(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetSessionRecord failed: %v", err)
		}
		if rec1.RevokedAt == nil || !rec1.RevokedAt.Equal(tRevoke1) {
			t.Errorf("expected RevokedAt %v, got %v", tRevoke1, rec1.RevokedAt)
		}

		// Repeated revocation at t0 + 15m (must preserve tRevoke1)
		tRevoke2 := t0.Add(15 * time.Minute)
		err = repo.RevokeAt(ctx, tokenHash, tRevoke2)
		if err != nil {
			t.Fatalf("repeated RevokeAt failed: %v", err)
		}

		rec2, err := repo.GetSessionRecord(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetSessionRecord failed: %v", err)
		}
		if !rec2.RevokedAt.Equal(tRevoke1) {
			t.Errorf("repeated revocation changed timestamp: expected %v, got %v", tRevoke1, rec2.RevokedAt)
		}

		// Subsequent validation must be rejected
		_, errVal := repo.ValidateAndTouchAt(ctx, tokenHash, t0.Add(6*time.Minute))
		if !errors.Is(errVal, session.ErrInvalidSession) {
			t.Errorf("expected ErrInvalidSession for revoked session, got %v", errVal)
		}
	})

	t.Run("InactiveAccountRejectionOnValidation", func(t *testing.T) {
		tokenHash := strings.Repeat("7", 64)
		t0 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

		// Create user specifically for deactivation test
		targetUserID := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")
		_, err := pool.Exec(ctx, `
			INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
			VALUES ('cccccccc-cccc-cccc-cccc-cccccccccccc', 'deactivateme@giki.edu.pk', 'Deactivate Me', '$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$c29tZWhhc2gz', 'active');
		`)
		if err != nil {
			t.Fatalf("failed to insert user: %v", err)
		}

		_, err = repo.CreateAt(ctx, targetUserID, tokenHash, t0)
		if err != nil {
			t.Fatalf("CreateAt failed: %v", err)
		}

		// Valid while active
		_, err = repo.ValidateAndTouchAt(ctx, tokenHash, t0.Add(5*time.Minute))
		if err != nil {
			t.Fatalf("ValidateAndTouchAt failed while active: %v", err)
		}

		// Admin deactivates account
		_, err = pool.Exec(ctx, "UPDATE giki_sports.users SET status = 'inactive' WHERE id = 'cccccccc-cccc-cccc-cccc-cccccccccccc';")
		if err != nil {
			t.Fatalf("failed to deactivate user: %v", err)
		}

		// Immediate rejection on next validation
		_, errVal := repo.ValidateAndTouchAt(ctx, tokenHash, t0.Add(10*time.Minute))
		if !errors.Is(errVal, session.ErrInvalidSession) {
			t.Errorf("expected ErrInvalidSession for deactivated user, got %v", errVal)
		}
	})

	t.Run("ConcurrentValidationsPreserveMonotonicActivity", func(t *testing.T) {
		tokenHash := strings.Repeat("8", 64)
		t0 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

		_, err := repo.CreateAt(ctx, activeUserID, tokenHash, t0)
		if err != nil {
			t.Fatalf("CreateAt failed: %v", err)
		}

		const concurrency = 10
		var wg sync.WaitGroup
		startBarrier := make(chan struct{})

		// Spawn goroutines with staggered time offsets (1s, 2s, ... 10s)
		for i := 1; i <= concurrency; i++ {
			wg.Add(1)
			offset := time.Duration(i) * time.Second
			touchTime := t0.Add(offset)

			go func(tTime time.Time) {
				defer wg.Done()
				<-startBarrier // await release

				_, err := repo.ValidateAndTouchAt(context.Background(), tokenHash, tTime)
				if err != nil {
					t.Errorf("concurrent ValidateAndTouchAt failed: %v", err)
				}
			}(touchTime)
		}

		close(startBarrier) // release all concurrent workers simultaneously
		wg.Wait()

		// The final stored last_activity_at MUST equal the maximum timestamp among all workers (t0 + 10s)
		rec, err := repo.GetSessionRecord(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetSessionRecord failed: %v", err)
		}

		maxExpectedTime := t0.Add(time.Duration(concurrency) * time.Second)
		if !rec.LastActivityAt.Equal(maxExpectedTime) {
			t.Errorf("expected monotonic last_activity_at = %v, got %v", maxExpectedTime, rec.LastActivityAt)
		}
	})

	t.Run("ValidationRevocationConcurrencyCannotReviveRevokedSession", func(t *testing.T) {
		tokenHash := strings.Repeat("9", 64)
		t0 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

		_, err := repo.CreateAt(ctx, activeUserID, tokenHash, t0)
		if err != nil {
			t.Fatalf("CreateAt failed: %v", err)
		}

		var wg sync.WaitGroup
		startBarrier := make(chan struct{})

		// Goroutine A: Revoke
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startBarrier
			_ = repo.RevokeAt(context.Background(), tokenHash, t0.Add(2*time.Minute))
		}()

		// Goroutine B: Validate and Touch
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startBarrier
			_, _ = repo.ValidateAndTouchAt(context.Background(), tokenHash, t0.Add(1*time.Minute))
		}()

		close(startBarrier)
		wg.Wait()

		// After both finish, the session must be revoked, and any subsequent validation MUST fail
		rec, err := repo.GetSessionRecord(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetSessionRecord failed: %v", err)
		}
		if rec.RevokedAt == nil {
			t.Fatal("expected session to be revoked")
		}

		// Subsequent validation must fail
		_, errLater := repo.ValidateAndTouchAt(ctx, tokenHash, t0.Add(3*time.Minute))
		if !errors.Is(errLater, session.ErrInvalidSession) {
			t.Errorf("expected ErrInvalidSession after revocation, got %v", errLater)
		}
	})

	t.Run("InvalidHashFormatAndCanceledContext", func(t *testing.T) {
		t0 := time.Now().UTC()

		// Invalid hash format (wrong length, uppercase, spaces)
		badHashes := []string{
			"",
			"short",
			strings.Repeat("A", 64),
			strings.Repeat("1", 63),
			strings.Repeat("1", 65),
			strings.Repeat("1", 63) + "z",
		}

		for _, bad := range badHashes {
			if _, err := repo.CreateAt(ctx, activeUserID, bad, t0); !errors.Is(err, session.ErrInvalidTokenHash) {
				t.Errorf("CreateAt expected ErrInvalidTokenHash for %q, got %v", bad, err)
			}
			if _, err := repo.ValidateAndTouchAt(ctx, bad, t0); !errors.Is(err, session.ErrInvalidTokenHash) {
				t.Errorf("ValidateAndTouchAt expected ErrInvalidTokenHash for %q, got %v", bad, err)
			}
			if err := repo.RevokeAt(ctx, bad, t0); !errors.Is(err, session.ErrInvalidTokenHash) {
				t.Errorf("RevokeAt expected ErrInvalidTokenHash for %q, got %v", bad, err)
			}
		}

		// Canceled context
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		validHash := strings.Repeat("0", 64)
		if _, err := repo.CreateAt(canceledCtx, activeUserID, validHash, t0); !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled on CreateAt, got %v", err)
		}
		if _, err := repo.ValidateAndTouchAt(canceledCtx, validHash, t0); !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled on ValidateAndTouchAt, got %v", err)
		}
	})

	t.Run("LockWait_IdleExpiry_SamplesClockAfterLock_AndRejects", func(t *testing.T) {
		tokenHash := strings.Repeat("a", 64)
		t0 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
		fakeClock := session.NewTestClock(t0)
		repoWithClock := session.NewRepositoryWithClock(pool, fakeClock)

		// 1. Create session at t0
		s, err := repoWithClock.Create(ctx, activeUserID, tokenHash)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		// 2. Set fake clock to 1 microsecond before 30m idle expiry (valid if evaluated right now)
		tBeforeIdle := t0.Add(session.IdleTimeout - time.Microsecond)
		fakeClock.Set(tBeforeIdle)

		// 3. Acquire exclusive row lock in separate transaction (txBlocker)
		txBlocker, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin blocker transaction: %v", err)
		}
		defer txBlocker.Rollback(ctx)

		var blockerPID uint32
		if err := txBlocker.QueryRow(ctx, "SELECT pg_backend_pid();").Scan(&blockerPID); err != nil {
			t.Fatalf("failed to get blocker PID: %v", err)
		}

		var lockedID uuid.UUID
		if err := txBlocker.QueryRow(ctx, "SELECT id FROM giki_sports.sessions WHERE token_hash = $1 FOR UPDATE;", tokenHash).Scan(&lockedID); err != nil {
			t.Fatalf("failed to lock session row in blocker: %v", err)
		}

		// 4. Begin validation in background goroutine
		type valResult struct {
			sess *session.Session
			err  error
		}
		resCh := make(chan valResult, 1)

		go func() {
			sess, err := repoWithClock.ValidateAndTouch(context.Background(), tokenHash)
			resCh <- valResult{sess: sess, err: err}
		}()

		// 5. Confirm validation goroutine is actively blocked on row lock using explicit coordination
		waitingPID, err := waitForBlockedSession(ctx, pool, blockerPID, 5*time.Second)
		if err != nil {
			t.Fatalf("failed waiting for validation to block on row lock: %v", err)
		}
		if waitingPID == 0 {
			t.Fatal("expected non-zero waiting PID")
		}

		// 6. Advance fake clock to exact idle expiry boundary while validation is waiting on the lock
		tExactIdle := t0.Add(session.IdleTimeout)
		fakeClock.Set(tExactIdle)

		// 7. Release blocker lock so validation can proceed
		if err := txBlocker.Rollback(ctx); err != nil {
			t.Fatalf("failed to rollback blocker transaction: %v", err)
		}

		// 8. Receive validation result; must be rejected with ErrInvalidSession
		select {
		case res := <-resCh:
			if !errors.Is(res.err, session.ErrInvalidSession) {
				t.Fatalf("expected ErrInvalidSession after clock advanced during lock wait, got %v", res.err)
			}
			if res.sess != nil {
				t.Fatalf("expected nil session on rejected validation, got %+v", res.sess)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for ValidateAndTouch to complete after lock release")
		}

		// 9. Verify in database that last_activity_at remains completely unchanged from creation (t0)
		rec, err := repoWithClock.GetSessionRecord(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetSessionRecord failed: %v", err)
		}
		if !rec.LastActivityAt.Equal(t0) {
			t.Fatalf("stale validation modified last_activity_at: expected %v, got %v", t0, rec.LastActivityAt)
		}
		if !rec.CreatedAt.Equal(s.CreatedAt) {
			t.Fatalf("createdAt changed: expected %v, got %v", s.CreatedAt, rec.CreatedAt)
		}
	})

	t.Run("LockWait_AbsoluteExpiry_SamplesClockAfterLock_AndRejects", func(t *testing.T) {
		tokenHash := strings.Repeat("b", 64)
		t0 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
		fakeClock := session.NewTestClock(t0)
		repoWithClock := session.NewRepositoryWithClock(pool, fakeClock)

		// 1. Create session at t0
		_, err := repoWithClock.Create(ctx, activeUserID, tokenHash)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		// 2. Keep session active via periodic touches every 25 minutes (within 30m idle window) up to 7h 45m
		curr := t0
		for curr.Before(t0.Add(7*time.Hour + 45*time.Minute)) {
			curr = curr.Add(25 * time.Minute)
			fakeClock.Set(curr)
			_, err = repoWithClock.ValidateAndTouch(ctx, tokenHash)
			if err != nil {
				t.Fatalf("keep-alive touch at %v failed: %v", curr, err)
			}
		}
		tKeepAlive := curr // 7 hours 55 minutes

		// 3. Set fake clock to 1 microsecond before 8h absolute expiry (within idle window from tKeepAlive)
		tBeforeAbs := t0.Add(session.AbsoluteLifetime - time.Microsecond)
		fakeClock.Set(tBeforeAbs)

		// 4. Acquire exclusive row lock in separate transaction
		txBlocker, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin blocker transaction: %v", err)
		}
		defer txBlocker.Rollback(ctx)

		var blockerPID uint32
		if err := txBlocker.QueryRow(ctx, "SELECT pg_backend_pid();").Scan(&blockerPID); err != nil {
			t.Fatalf("failed to get blocker PID: %v", err)
		}

		var lockedID uuid.UUID
		if err := txBlocker.QueryRow(ctx, "SELECT id FROM giki_sports.sessions WHERE token_hash = $1 FOR UPDATE;", tokenHash).Scan(&lockedID); err != nil {
			t.Fatalf("failed to lock session row: %v", err)
		}

		// 5. Begin validation in background goroutine
		type valResult struct {
			sess *session.Session
			err  error
		}
		resCh := make(chan valResult, 1)

		go func() {
			sess, err := repoWithClock.ValidateAndTouch(context.Background(), tokenHash)
			resCh <- valResult{sess: sess, err: err}
		}()

		// 6. Explicitly wait for validation to be blocked in PostgreSQL
		_, err = waitForBlockedSession(ctx, pool, blockerPID, 5*time.Second)
		if err != nil {
			t.Fatalf("failed waiting for validation to block on row lock: %v", err)
		}

		// 7. Advance fake clock to exact absolute expiry boundary (8h)
		fakeClock.Set(t0.Add(session.AbsoluteLifetime))

		// 8. Release blocker lock
		if err := txBlocker.Rollback(ctx); err != nil {
			t.Fatalf("failed to rollback blocker: %v", err)
		}

		// 9. Validation must reject with ErrInvalidSession
		select {
		case res := <-resCh:
			if !errors.Is(res.err, session.ErrInvalidSession) {
				t.Fatalf("expected ErrInvalidSession after absolute expiry during lock wait, got %v", res.err)
			}
			if res.sess != nil {
				t.Fatalf("expected nil session on rejected validation, got %+v", res.sess)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for ValidateAndTouch to complete after lock release")
		}

		// 10. Verify persisted last_activity_at remains at tKeepAlive (not modified)
		rec, err := repoWithClock.GetSessionRecord(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetSessionRecord failed: %v", err)
		}
		if !rec.LastActivityAt.Equal(tKeepAlive) {
			t.Fatalf("stale validation modified last_activity_at: expected %v, got %v", tKeepAlive, rec.LastActivityAt)
		}
	})

	t.Run("LockWait_ContextCancellation_ReleasesResources", func(t *testing.T) {
		tokenHash := strings.Repeat("c", 64)
		t0 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
		fakeClock := session.NewTestClock(t0)
		repoWithClock := session.NewRepositoryWithClock(pool, fakeClock)

		// 1. Create session at t0
		_, err := repoWithClock.Create(ctx, activeUserID, tokenHash)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		// 2. Acquire exclusive row lock in separate transaction
		txBlocker, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin blocker transaction: %v", err)
		}
		defer txBlocker.Rollback(ctx)

		var blockerPID uint32
		if err := txBlocker.QueryRow(ctx, "SELECT pg_backend_pid();").Scan(&blockerPID); err != nil {
			t.Fatalf("failed to get blocker PID: %v", err)
		}

		var lockedID uuid.UUID
		if err := txBlocker.QueryRow(ctx, "SELECT id FROM giki_sports.sessions WHERE token_hash = $1 FOR UPDATE;", tokenHash).Scan(&lockedID); err != nil {
			t.Fatalf("failed to lock session row: %v", err)
		}

		// 3. Begin validation with cancelable context
		cancelCtx, cancel := context.WithCancel(context.Background())
		type valResult struct {
			sess *session.Session
			err  error
		}
		resCh := make(chan valResult, 1)

		go func() {
			sess, err := repoWithClock.ValidateAndTouch(cancelCtx, tokenHash)
			resCh <- valResult{sess: sess, err: err}
		}()

		// 4. Explicitly wait until validation is blocked in PostgreSQL
		_, err = waitForBlockedSession(ctx, pool, blockerPID, 5*time.Second)
		if err != nil {
			t.Fatalf("failed waiting for validation to block on row lock: %v", err)
		}

		// 5. Cancel context while waiting for lock
		cancel()

		// 6. Validation must return context.Canceled error
		select {
		case res := <-resCh:
			if !errors.Is(res.err, context.Canceled) {
				t.Fatalf("expected context.Canceled on aborted lock wait, got %v", res.err)
			}
			if res.sess != nil {
				t.Fatalf("expected nil session on cancellation, got %+v", res.sess)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for ValidateAndTouch to abort after context cancellation")
		}

		// 7. Release blocker lock
		if err := txBlocker.Rollback(ctx); err != nil {
			t.Fatalf("failed to rollback blocker: %v", err)
		}

		// 8. Verify connection pool and database are completely healthy and unblocked
		rec, err := repoWithClock.GetSessionRecord(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetSessionRecord failed after cancellation: %v", err)
		}
		if !rec.LastActivityAt.Equal(t0) {
			t.Fatalf("canceled validation altered activity timestamp: expected %v, got %v", t0, rec.LastActivityAt)
		}
	})

	// Final safety verification: development database remained untouched
	finalUsers, finalSessions := countDevRecords(t)
	if finalUsers != initialUsers || finalSessions != initialSessions {
		t.Fatalf("safety violation: dev records changed! users: %d->%d, sessions: %d->%d", initialUsers, finalUsers, initialSessions, finalSessions)
	}
}
