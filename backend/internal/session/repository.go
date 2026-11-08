package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"giki-sports-backend/internal/auth"
)

// Repository defines operations for PostgreSQL session persistence and atomic validation.
type Repository interface {
	// Create persists a new session with an active token hash for an active user.
	// Sets created_at and last_activity_at to the current clock time.
	// Absolute expires_at is set to exactly now + 8 hours.
	// Returns ErrUserNotFoundOrInactive if user does not exist or status != 'active'.
	// Returns ErrInvalidTokenHash if tokenHash is malformed.
	Create(ctx context.Context, userID uuid.UUID, tokenHash string) (*Session, error)

	// CreateAt is the time-injected variant of Create for deterministic testing.
	CreateAt(ctx context.Context, userID uuid.UUID, tokenHash string, now time.Time) (*Session, error)

	// ValidateAndTouch validates the session and touches last_activity_at atomically.
	// Enforces:
	//   - now < last_activity_at + 30 minutes (idle timeout)
	//   - now < absolute_expires_at (absolute timeout)
	//   - revoked_at IS NULL
	//   - user account status == 'active'
	//
	// Locking & Timing Semantics:
	// Executes in a short transaction that acquires an exclusive row-level lock on giki_sports.sessions
	// (FOR UPDATE OF s). The joined table u (users) is NOT locked; account status is read under the snapshot.
	// Injected clock time (r.clock.Now()) is sampled strictly AFTER the row lock is granted, preventing
	// stale timestamp evaluations caused by lock contention.
	//
	// Updates last_activity_at = GREATEST(last_activity_at, now) only when valid.
	// Rejected validation leaves activity unchanged in PostgreSQL and returns ErrInvalidSession.
	ValidateAndTouch(ctx context.Context, tokenHash string) (*Session, error)

	// ValidateAndTouchAt is an explicit-time test helper that overrides r.clock.Now() with an explicit timestamp.
	// WARNING: This helper bypasses dynamic clock sampling after lock acquisition. Prefer ValidateAndTouch
	// with an injected Clock (such as TestClock) when testing lock waits and clock advancement.
	ValidateAndTouchAt(ctx context.Context, tokenHash string, now time.Time) (*Session, error)

	// Revoke invalidates a session idempotently.
	// Sets revoked_at = COALESCE(revoked_at, now), preserving the first revocation timestamp.
	// Returns ErrInvalidSession if the session does not exist.
	Revoke(ctx context.Context, tokenHash string) error

	// RevokeAt is the time-injected variant of Revoke for deterministic testing.
	RevokeAt(ctx context.Context, tokenHash string, now time.Time) error

	// GetSessionRecord retrieves full lifetime metadata for testing and audit.
	// Excludes tokenHash from return values.
	GetSessionRecord(ctx context.Context, tokenHash string) (*SessionRecord, error)
}

// PostgresRepository implements Repository using a PostgreSQL connection pool.
type PostgresRepository struct {
	pool  *pgxpool.Pool
	clock Clock
}

// NewRepository creates a new PostgresRepository instance using RealClock.
func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return NewRepositoryWithClock(pool, RealClock{})
}

// NewRepositoryWithClock creates a PostgresRepository with an injected Clock.
func NewRepositoryWithClock(pool *pgxpool.Pool, clock Clock) *PostgresRepository {
	if clock == nil {
		clock = RealClock{}
	}
	return &PostgresRepository{
		pool:  pool,
		clock: clock,
	}
}

// Create persists a new session for an active user using the repository's clock.
func (r *PostgresRepository) Create(ctx context.Context, userID uuid.UUID, tokenHash string) (*Session, error) {
	return r.CreateAt(ctx, userID, tokenHash, r.clock.Now())
}

// CreateAt creates a session at the specified timestamp.
//
// Invariants Enforced:
// 1. tokenHash format must strictly match 64 lowercase hexadecimal characters.
// 2. Target user must exist and have status = 'active'.
// 3. created_at and last_activity_at are set to the exact same instant (now).
// 4. absolute_expires_at is set to exactly now + 8 hours.
func (r *PostgresRepository) CreateAt(ctx context.Context, userID uuid.UUID, tokenHash string, now time.Time) (*Session, error) {
	if err := auth.ValidateTokenHash(tokenHash); err != nil {
		return nil, ErrInvalidTokenHash
	}
	if userID == uuid.Nil {
		return nil, ErrUserNotFoundOrInactive
	}

	// Normalize to PostgreSQL microsecond precision
	now = now.UTC().Truncate(time.Microsecond)
	absoluteExpiry := now.Add(AbsoluteLifetime)

	const createSessionSQL = `
		INSERT INTO giki_sports.sessions (
			user_id,
			token_hash,
			created_at,
			last_activity_at,
			absolute_expires_at
		)
		SELECT 
			u.id,
			$2,
			$3,
			$3,
			$4
		FROM giki_sports.users u
		WHERE u.id = $1 AND u.status = 'active'
		RETURNING 
			id,
			user_id,
			created_at,
			last_activity_at,
			absolute_expires_at;
	`

	var s Session
	err := r.pool.QueryRow(ctx, createSessionSQL, userID, tokenHash, now, absoluteExpiry).Scan(
		&s.ID,
		&s.UserID,
		&s.CreatedAt,
		&s.LastActivityAt,
		&s.AbsoluteExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFoundOrInactive
		}
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &s, nil
}

// ValidateAndTouch validates the session and updates its activity timestamp using the repository's clock.
// It acquires an exclusive row-level lock on the session row and samples r.clock.Now() strictly AFTER
// that row lock is granted, ensuring that lock wait durations never result in stale validation evaluations.
func (r *PostgresRepository) ValidateAndTouch(ctx context.Context, tokenHash string) (*Session, error) {
	return r.validateAndTouch(ctx, tokenHash, nil)
}

// ValidateAndTouchAt is an explicit-time test helper that overrides r.clock.Now() with an explicit timestamp.
//
// WARNING: This helper is provided for tests specifying a fixed point in time. It bypasses dynamic clock
// sampling after lock acquisition. For deterministic regression testing of lock waits and clock advancement,
// prefer ValidateAndTouch with an injected Clock (such as TestClock).
func (r *PostgresRepository) ValidateAndTouchAt(ctx context.Context, tokenHash string, explicitNow time.Time) (*Session, error) {
	return r.validateAndTouch(ctx, tokenHash, &explicitNow)
}

func (r *PostgresRepository) validateAndTouch(ctx context.Context, tokenHash string, explicitNow *time.Time) (*Session, error) {
	if err := auth.ValidateTokenHash(tokenHash); err != nil {
		return nil, ErrInvalidTokenHash
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to begin session validation transaction: %w", err)
	}
	defer func() {
		// Clean up any uncommitted transaction to release locks and return connections to the pool.
		// A bounded background context ensures rollback succeeds even when request context was canceled.
		rbCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(rbCtx)
	}()

	// 1. Find and lock the session row by token hash.
	// FOR UPDATE OF s locks ONLY the session row s.
	// The joined table u (users) is NOT locked; account status is read under the transaction snapshot.
	const lockSessionSQL = `
		SELECT 
			s.id,
			s.user_id,
			s.created_at,
			s.last_activity_at,
			s.absolute_expires_at,
			s.revoked_at,
			u.status
		FROM giki_sports.sessions s
		JOIN giki_sports.users u ON s.user_id = u.id
		WHERE s.token_hash = $1
		FOR UPDATE OF s;
	`

	var (
		id                uuid.UUID
		userID            uuid.UUID
		createdAt         time.Time
		lastActivityAt    time.Time
		absoluteExpiresAt time.Time
		revokedAt         *time.Time
		userStatus        string
	)

	err = tx.QueryRow(ctx, lockSessionSQL, tokenHash).Scan(
		&id,
		&userID,
		&createdAt,
		&lastActivityAt,
		&absoluteExpiresAt,
		&revokedAt,
		&userStatus,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidSession
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to acquire session lock: %w", err)
	}

	// 2. Sample injected clock AFTER acquiring the session row lock.
	var now time.Time
	if explicitNow != nil {
		now = *explicitNow
	} else {
		now = r.clock.Now()
	}
	now = now.UTC().Truncate(time.Microsecond)

	// 3. Check revocation, idle expiry, absolute expiry, and active account.
	// If any invariant fails, rollback cleanly and return ErrInvalidSession.
	// Activity remains unchanged in the database.
	if revokedAt != nil {
		return nil, ErrInvalidSession
	}
	if userStatus != "active" {
		return nil, ErrInvalidSession
	}
	if now.Before(createdAt) {
		return nil, ErrInvalidSession
	}
	if !now.Before(lastActivityAt.Add(IdleTimeout)) {
		return nil, ErrInvalidSession
	}
	if !now.Before(absoluteExpiresAt) {
		return nil, ErrInvalidSession
	}

	// 4. Update activity only when valid, preserving monotonic activity and fixed absolute expiry.
	const updateActivitySQL = `
		UPDATE giki_sports.sessions
		SET last_activity_at = GREATEST(last_activity_at, $2)
		WHERE id = $1
		RETURNING last_activity_at;
	`

	var updatedActivity time.Time
	err = tx.QueryRow(ctx, updateActivitySQL, id, now).Scan(&updatedActivity)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to update session activity: %w", err)
	}

	// 5. Commit before returning success.
	if err := tx.Commit(ctx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to commit session validation: %w", err)
	}

	return &Session{
		ID:                id,
		UserID:            userID,
		CreatedAt:         createdAt,
		LastActivityAt:    updatedActivity,
		AbsoluteExpiresAt: absoluteExpiresAt,
	}, nil
}

// Revoke invalidates a session idempotently using the repository's clock.
func (r *PostgresRepository) Revoke(ctx context.Context, tokenHash string) error {
	return r.RevokeAt(ctx, tokenHash, r.clock.Now())
}

// RevokeAt invalidates a session at the specified timestamp.
//
// Idempotence & Coordination:
// - Uses SET revoked_at = COALESCE(revoked_at, $2).
// - If already revoked, COALESCE preserves the original revocation timestamp.
// - Returns ErrInvalidSession if the session does not exist.
func (r *PostgresRepository) RevokeAt(ctx context.Context, tokenHash string, now time.Time) error {
	if err := auth.ValidateTokenHash(tokenHash); err != nil {
		return ErrInvalidTokenHash
	}

	now = now.UTC().Truncate(time.Microsecond)

	const revokeSessionSQL = `
		UPDATE giki_sports.sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE token_hash = $1
		RETURNING id;
	`

	var id uuid.UUID
	err := r.pool.QueryRow(ctx, revokeSessionSQL, tokenHash, now).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidSession
		}
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	return nil
}

// GetSessionRecord retrieves the full stored session record for audit and integration testing.
// Excludes tokenHash from return models.
func (r *PostgresRepository) GetSessionRecord(ctx context.Context, tokenHash string) (*SessionRecord, error) {
	if err := auth.ValidateTokenHash(tokenHash); err != nil {
		return nil, ErrInvalidTokenHash
	}

	const getRecordSQL = `
		SELECT 
			id,
			user_id,
			created_at,
			last_activity_at,
			absolute_expires_at,
			revoked_at
		FROM giki_sports.sessions
		WHERE token_hash = $1;
	`

	var rec SessionRecord
	err := r.pool.QueryRow(ctx, getRecordSQL, tokenHash).Scan(
		&rec.ID,
		&rec.UserID,
		&rec.CreatedAt,
		&rec.LastActivityAt,
		&rec.AbsoluteExpiresAt,
		&rec.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidSession
		}
		return nil, fmt.Errorf("failed to get session record: %w", err)
	}

	return &rec, nil
}
