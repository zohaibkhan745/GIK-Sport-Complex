package membership

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines storage access for gym memberships and audit logs.
type Repository interface {
	CreateMembershipTx(ctx context.Context, tx pgx.Tx, m *Membership) error
	GetMembershipByIdempotencyTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, idempotencyKey string) (*Membership, error)
	GetLatestActiveOrUpcomingMembershipTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, now time.Time) (*Membership, error)
	RecordAuditLogTx(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, action, targetType string, targetID uuid.UUID, details map[string]interface{}) error
	ListMembershipsByUserID(ctx context.Context, userID uuid.UUID) ([]*Membership, error)
	ListAllMemberships(ctx context.Context, limit, offset int) ([]*Membership, error)
	GetMembershipByID(ctx context.Context, id uuid.UUID) (*Membership, error)
}

// PostgresRepository implements Repository using PostgreSQL.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new PostgresRepository instance.
func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// CreateMembershipTx inserts a new membership record within a transaction.
func (r *PostgresRepository) CreateMembershipTx(ctx context.Context, tx pgx.Tx, m *Membership) error {
	const query = `
		INSERT INTO giki_sports.gym_memberships (
			id, user_id, plan_type, status, start_time, end_time, price_paisa,
			debit_transaction_id, idempotency_key, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);
	`
	now := time.Now().UTC()
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	m.CreatedAt = now
	m.UpdatedAt = now

	_, err := tx.Exec(ctx, query,
		m.ID,
		m.UserID,
		m.PlanType,
		m.Status,
		m.StartTime,
		m.EndTime,
		m.PricePaisa,
		m.DebitTransactionID,
		m.IdempotencyKey,
		m.CreatedAt,
		m.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicatePurchase
		}
		return fmt.Errorf("failed to insert gym membership: %w", err)
	}
	return nil
}

// GetMembershipByIdempotencyTx queries a membership previously inserted with the same idempotency key for this user.
func (r *PostgresRepository) GetMembershipByIdempotencyTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, idempotencyKey string) (*Membership, error) {
	const query = `
		SELECT id, user_id, plan_type, status, start_time, end_time, price_paisa,
		       debit_transaction_id, idempotency_key, created_at, updated_at
		FROM giki_sports.gym_memberships
		WHERE user_id = $1 AND idempotency_key = $2;
	`
	var m Membership
	err := tx.QueryRow(ctx, query, userID, idempotencyKey).Scan(
		&m.ID,
		&m.UserID,
		&m.PlanType,
		&m.Status,
		&m.StartTime,
		&m.EndTime,
		&m.PricePaisa,
		&m.DebitTransactionID,
		&m.IdempotencyKey,
		&m.CreatedAt,
		&m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query membership by idempotency key: %w", err)
	}
	return &m, nil
}

// GetLatestActiveOrUpcomingMembershipTx finds the user's latest active pass that expires in the future,
// with a row-level lock (FOR UPDATE) to serialize concurrent renewal calculations.
func (r *PostgresRepository) GetLatestActiveOrUpcomingMembershipTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, now time.Time) (*Membership, error) {
	const query = `
		SELECT id, user_id, plan_type, status, start_time, end_time, price_paisa,
		       debit_transaction_id, idempotency_key, created_at, updated_at
		FROM giki_sports.gym_memberships
		WHERE user_id = $1 AND status = 'ACTIVE' AND end_time > $2
		ORDER BY end_time DESC
		LIMIT 1
		FOR UPDATE;
	`
	var m Membership
	err := tx.QueryRow(ctx, query, userID, now).Scan(
		&m.ID,
		&m.UserID,
		&m.PlanType,
		&m.Status,
		&m.StartTime,
		&m.EndTime,
		&m.PricePaisa,
		&m.DebitTransactionID,
		&m.IdempotencyKey,
		&m.CreatedAt,
		&m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query latest active membership: %w", err)
	}
	return &m, nil
}

// RecordAuditLogTx writes an administrative/transactional audit event inside the ongoing transaction.
func (r *PostgresRepository) RecordAuditLogTx(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, action, targetType string, targetID uuid.UUID, details map[string]interface{}) error {
	const query = `
		INSERT INTO giki_sports.audit_logs (
			id, actor_id, action, target_type, target_id, details, created_at
		) VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, now());
	`
	detailsBytes, err := json.Marshal(details)
	if err != nil {
		detailsBytes = []byte("{}")
	}
	_, err = tx.Exec(ctx, query, actorID, action, targetType, targetID, detailsBytes)
	if err != nil {
		return fmt.Errorf("failed to write audit log: %w", err)
	}
	return nil
}

// ListMembershipsByUserID retrieves all memberships for a user ordered by creation descending.
func (r *PostgresRepository) ListMembershipsByUserID(ctx context.Context, userID uuid.UUID) ([]*Membership, error) {
	const query = `
		SELECT id, user_id, plan_type, status, start_time, end_time, price_paisa,
		       debit_transaction_id, idempotency_key, created_at, updated_at
		FROM giki_sports.gym_memberships
		WHERE user_id = $1
		ORDER BY end_time DESC, created_at DESC;
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list memberships by user: %w", err)
	}
	defer rows.Close()

	var result []*Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(
			&m.ID,
			&m.UserID,
			&m.PlanType,
			&m.Status,
			&m.StartTime,
			&m.EndTime,
			&m.PricePaisa,
			&m.DebitTransactionID,
			&m.IdempotencyKey,
			&m.CreatedAt,
			&m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan membership: %w", err)
		}
		result = append(result, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}
	return result, nil
}

// ListAllMemberships returns a paginated list of memberships for admin queries.
func (r *PostgresRepository) ListAllMemberships(ctx context.Context, limit, offset int) ([]*Membership, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	const query = `
		SELECT id, user_id, plan_type, status, start_time, end_time, price_paisa,
		       debit_transaction_id, idempotency_key, created_at, updated_at
		FROM giki_sports.gym_memberships
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2;
	`
	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list all memberships: %w", err)
	}
	defer rows.Close()

	var result []*Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(
			&m.ID,
			&m.UserID,
			&m.PlanType,
			&m.Status,
			&m.StartTime,
			&m.EndTime,
			&m.PricePaisa,
			&m.DebitTransactionID,
			&m.IdempotencyKey,
			&m.CreatedAt,
			&m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan membership: %w", err)
		}
		result = append(result, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}
	return result, nil
}

// GetMembershipByID retrieves a single membership by ID.
func (r *PostgresRepository) GetMembershipByID(ctx context.Context, id uuid.UUID) (*Membership, error) {
	const query = `
		SELECT id, user_id, plan_type, status, start_time, end_time, price_paisa,
		       debit_transaction_id, idempotency_key, created_at, updated_at
		FROM giki_sports.gym_memberships
		WHERE id = $1;
	`
	var m Membership
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&m.ID,
		&m.UserID,
		&m.PlanType,
		&m.Status,
		&m.StartTime,
		&m.EndTime,
		&m.PricePaisa,
		&m.DebitTransactionID,
		&m.IdempotencyKey,
		&m.CreatedAt,
		&m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMembershipNotFound
		}
		return nil, fmt.Errorf("failed to get membership %s: %w", id, err)
	}
	return &m, nil
}
