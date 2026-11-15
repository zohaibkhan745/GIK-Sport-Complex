package padel

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"giki-sports-backend/internal/wallet"
)

// GenerateCheckInCode generates a cryptographically secure 4-digit numeric verification code (1000-9999).
func GenerateCheckInCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(9000))
	if err != nil {
		return "", fmt.Errorf("failed to generate secure check-in code: %w", err)
	}
	return fmt.Sprintf("%04d", n.Int64()+1000), nil
}

// BookingRepository defines transactional database operations for court slots and confirmed bookings.
type BookingRepository interface {
	LockSlotForBooking(ctx context.Context, slotID uuid.UUID) (*Slot, error)
	CreateBooking(ctx context.Context, booking *Booking) error
	MarkSlotBooked(ctx context.Context, slotID uuid.UUID) error
	FindBookingByIdempotency(ctx context.Context, userID uuid.UUID, idempotencyKey string) (*Booking, error)
	FindBookingByID(ctx context.Context, bookingID uuid.UUID) (*Booking, error)
	LockBookingForUpdate(ctx context.Context, bookingID uuid.UUID) (*Booking, error)
	ReleaseSlotToAvailable(ctx context.Context, slotID uuid.UUID) error
	CancelBooking(ctx context.Context, bookingID uuid.UUID, adminID uuid.UUID, reason string, refundTxID uuid.UUID, now time.Time) (*Booking, error)
	RecordAuditLog(ctx context.Context, actorID uuid.UUID, action string, targetType string, targetID uuid.UUID, details map[string]interface{}) error
}

// LedgerRepository defines transactional double-entry financial transfer operations.
type LedgerRepository interface {
	FindPersonalWallet(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	FindOrCreateSystemWallet(ctx context.Context, name string, wType wallet.WalletType, currency string) (uuid.UUID, error)
	TransferFunds(ctx context.Context, params wallet.TransferParams) (*wallet.PostResult, error)
}

// TransactionRepositoryFactory is the Abstract Factory interface supplying a compatible family
// of transaction-bound repositories for atomic booking and financial settlement.
// (SE-423 §6.1 Requirement: Creational Abstract Factory Pattern).
type TransactionRepositoryFactory interface {
	Bookings() BookingRepository
	Ledger() LedgerRepository
}

// UnitOfWork encapsulates the database transaction lifecycle for coordinated multi-repository operations.
type UnitOfWork interface {
	Execute(ctx context.Context, fn func(factory TransactionRepositoryFactory) error) error
}

// postgresBookingRepository is the concrete transactional implementation of BookingRepository bound to a pgx.Tx.
type postgresBookingRepository struct {
	tx pgx.Tx
}

func (r *postgresBookingRepository) LockSlotForBooking(ctx context.Context, slotID uuid.UUID) (*Slot, error) {
	const query = `
		SELECT 
			id, 
			facility_id, 
			start_time, 
			end_time, 
			status, 
			holder_id, 
			hold_expires_at, 
			price_paisa, 
			created_at, 
			updated_at
		FROM giki_sports.padel_slots
		WHERE id = $1
		FOR UPDATE;
	`
	var s Slot
	var holderID *uuid.UUID
	var holdExpiresAt *time.Time
	var rawStatus string

	err := r.tx.QueryRow(ctx, query, slotID).Scan(
		&s.ID,
		&s.FacilityID,
		&s.StartTime,
		&s.EndTime,
		&rawStatus,
		&holderID,
		&holdExpiresAt,
		&s.PricePaisa,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSlotNotFound
		}
		return nil, fmt.Errorf("failed to lock slot %s: %w", slotID, err)
	}

	s.Status = SlotStatus(rawStatus)
	s.HolderID = holderID
	s.HoldExpiresAt = holdExpiresAt
	return &s, nil
}

func (r *postgresBookingRepository) CreateBooking(ctx context.Context, booking *Booking) error {
	const query = `
		INSERT INTO giki_sports.padel_bookings (
			id,
			slot_id,
			user_id,
			status,
			fee_paisa,
			idempotency_key,
			check_in_code,
			debit_transaction_id,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, now(), now()
		)
		RETURNING created_at, updated_at;
	`
	err := r.tx.QueryRow(ctx, query,
		booking.ID,
		booking.SlotID,
		booking.UserID,
		string(booking.Status),
		booking.FeePaisa,
		booking.IdempotencyKey,
		booking.CheckInCode,
		booking.DebitTransactionID,
	).Scan(&booking.CreatedAt, &booking.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create booking record: %w", err)
	}
	return nil
}

func (r *postgresBookingRepository) MarkSlotBooked(ctx context.Context, slotID uuid.UUID) error {
	const query = `
		UPDATE giki_sports.padel_slots
		SET 
			status = 'BOOKED',
			holder_id = NULL,
			hold_expires_at = NULL,
			updated_at = now()
		WHERE id = $1;
	`
	tag, err := r.tx.Exec(ctx, query, slotID)
	if err != nil {
		return fmt.Errorf("failed to mark slot %s booked: %w", slotID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSlotNotFound
	}
	return nil
}

func (r *postgresBookingRepository) FindBookingByIdempotency(ctx context.Context, userID uuid.UUID, idempotencyKey string) (*Booking, error) {
	const query = `
		SELECT 
			id,
			slot_id,
			user_id,
			status,
			fee_paisa,
			idempotency_key,
			check_in_code,
			checked_in_at,
			checked_in_by,
			cancelled_at,
			cancelled_by,
			cancellation_reason,
			debit_transaction_id,
			refund_transaction_id,
			created_at,
			updated_at
		FROM giki_sports.padel_bookings
		WHERE user_id = $1 AND idempotency_key = $2;
	`
	var b Booking
	var rawStatus string
	err := r.tx.QueryRow(ctx, query, userID, idempotencyKey).Scan(
		&b.ID,
		&b.SlotID,
		&b.UserID,
		&rawStatus,
		&b.FeePaisa,
		&b.IdempotencyKey,
		&b.CheckInCode,
		&b.CheckedInAt,
		&b.CheckedInBy,
		&b.CancelledAt,
		&b.CancelledBy,
		&b.CancellationReason,
		&b.DebitTransactionID,
		&b.RefundTransactionID,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No existing booking found
		}
		return nil, fmt.Errorf("failed to query booking by idempotency: %w", err)
	}
	b.Status = BookingStatus(rawStatus)
	return &b, nil
}

func (r *postgresBookingRepository) FindBookingByID(ctx context.Context, bookingID uuid.UUID) (*Booking, error) {
	const query = `
		SELECT 
			id,
			slot_id,
			user_id,
			status,
			fee_paisa,
			idempotency_key,
			check_in_code,
			checked_in_at,
			checked_in_by,
			cancelled_at,
			cancelled_by,
			cancellation_reason,
			debit_transaction_id,
			refund_transaction_id,
			created_at,
			updated_at
		FROM giki_sports.padel_bookings
		WHERE id = $1;
	`
	var b Booking
	var rawStatus string
	err := r.tx.QueryRow(ctx, query, bookingID).Scan(
		&b.ID,
		&b.SlotID,
		&b.UserID,
		&rawStatus,
		&b.FeePaisa,
		&b.IdempotencyKey,
		&b.CheckInCode,
		&b.CheckedInAt,
		&b.CheckedInBy,
		&b.CancelledAt,
		&b.CancelledBy,
		&b.CancellationReason,
		&b.DebitTransactionID,
		&b.RefundTransactionID,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query booking %s: %w", bookingID, err)
	}
	b.Status = BookingStatus(rawStatus)
	return &b, nil
}

func (r *postgresBookingRepository) LockBookingForUpdate(ctx context.Context, bookingID uuid.UUID) (*Booking, error) {
	const query = `
		SELECT 
			id,
			slot_id,
			user_id,
			status,
			fee_paisa,
			idempotency_key,
			check_in_code,
			checked_in_at,
			checked_in_by,
			cancelled_at,
			cancelled_by,
			cancellation_reason,
			debit_transaction_id,
			refund_transaction_id,
			created_at,
			updated_at
		FROM giki_sports.padel_bookings
		WHERE id = $1
		FOR UPDATE;
	`
	var b Booking
	var rawStatus string
	err := r.tx.QueryRow(ctx, query, bookingID).Scan(
		&b.ID,
		&b.SlotID,
		&b.UserID,
		&rawStatus,
		&b.FeePaisa,
		&b.IdempotencyKey,
		&b.CheckInCode,
		&b.CheckedInAt,
		&b.CheckedInBy,
		&b.CancelledAt,
		&b.CancelledBy,
		&b.CancellationReason,
		&b.DebitTransactionID,
		&b.RefundTransactionID,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("failed to lock booking %s: %w", bookingID, err)
	}
	b.Status = BookingStatus(rawStatus)
	return &b, nil
}

func (r *postgresBookingRepository) ReleaseSlotToAvailable(ctx context.Context, slotID uuid.UUID) error {
	const query = `
		UPDATE giki_sports.padel_slots
		SET 
			status = 'AVAILABLE',
			holder_id = NULL,
			hold_expires_at = NULL,
			updated_at = now()
		WHERE id = $1;
	`
	tag, err := r.tx.Exec(ctx, query, slotID)
	if err != nil {
		return fmt.Errorf("failed to release slot %s to available: %w", slotID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSlotNotFound
	}
	return nil
}

func (r *postgresBookingRepository) CancelBooking(ctx context.Context, bookingID uuid.UUID, adminID uuid.UUID, reason string, refundTxID uuid.UUID, now time.Time) (*Booking, error) {
	const query = `
		UPDATE giki_sports.padel_bookings
		SET 
			status = 'CANCELLED',
			cancelled_at = $2,
			cancelled_by = $3,
			cancellation_reason = $4,
			refund_transaction_id = $5,
			updated_at = $2
		WHERE id = $1
		RETURNING 
			id, slot_id, user_id, status, fee_paisa, idempotency_key, check_in_code,
			checked_in_at, checked_in_by, cancelled_at, cancelled_by, cancellation_reason,
			debit_transaction_id, refund_transaction_id, created_at, updated_at;
	`
	var b Booking
	var rawStatus string
	err := r.tx.QueryRow(ctx, query, bookingID, now, adminID, reason, refundTxID).Scan(
		&b.ID,
		&b.SlotID,
		&b.UserID,
		&rawStatus,
		&b.FeePaisa,
		&b.IdempotencyKey,
		&b.CheckInCode,
		&b.CheckedInAt,
		&b.CheckedInBy,
		&b.CancelledAt,
		&b.CancelledBy,
		&b.CancellationReason,
		&b.DebitTransactionID,
		&b.RefundTransactionID,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("failed to cancel booking %s: %w", bookingID, err)
	}
	b.Status = BookingStatus(rawStatus)
	return &b, nil
}

func (r *postgresBookingRepository) RecordAuditLog(ctx context.Context, actorID uuid.UUID, action string, targetType string, targetID uuid.UUID, details map[string]interface{}) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("failed to marshal audit log details: %w", err)
	}

	const query = `
		INSERT INTO giki_sports.audit_logs (
			actor_id,
			action,
			target_type,
			target_id,
			details,
			status,
			created_at
		) VALUES (
			$1, $2, $3, $4, $5, 'SUCCESS', now()
		);
	`
	_, err = r.tx.Exec(ctx, query, actorID, action, targetType, targetID, detailsJSON)
	if err != nil {
		return fmt.Errorf("failed to record audit log: %w", err)
	}
	return nil
}

// postgresLedgerRepository is the concrete transactional implementation of LedgerRepository bound to a pgx.Tx.
type postgresLedgerRepository struct {
	tx        pgx.Tx
	walletSvc *wallet.Service
}

func (r *postgresLedgerRepository) FindPersonalWallet(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	if r.walletSvc == nil {
		return uuid.Nil, errors.New("wallet service is uninitialized")
	}
	return r.walletSvc.FindPersonalWalletTx(ctx, r.tx, userID)
}

func (r *postgresLedgerRepository) FindOrCreateSystemWallet(ctx context.Context, name string, wType wallet.WalletType, currency string) (uuid.UUID, error) {
	if r.walletSvc == nil {
		return uuid.Nil, errors.New("wallet service is uninitialized")
	}
	return r.walletSvc.FindOrCreateSystemWalletTx(ctx, r.tx, name, wType, currency)
}

func (r *postgresLedgerRepository) TransferFunds(ctx context.Context, params wallet.TransferParams) (*wallet.PostResult, error) {
	if r.walletSvc == nil {
		return nil, errors.New("wallet service is uninitialized")
	}
	return r.walletSvc.PostTransactionTx(ctx, r.tx, params)
}

// postgresTransactionRepositoryFactory is the concrete factory implementation binding repositories to a live transaction.
type postgresTransactionRepositoryFactory struct {
	tx        pgx.Tx
	walletSvc *wallet.Service
}

func (f *postgresTransactionRepositoryFactory) Bookings() BookingRepository {
	return &postgresBookingRepository{tx: f.tx}
}

func (f *postgresTransactionRepositoryFactory) Ledger() LedgerRepository {
	return &postgresLedgerRepository{tx: f.tx, walletSvc: f.walletSvc}
}

// PostgresUnitOfWork coordinates transaction lifecycle and Abstract Factory instantiations.
type PostgresUnitOfWork struct {
	pool      *pgxpool.Pool
	walletSvc *wallet.Service
}

// NewUnitOfWork constructs a PostgresUnitOfWork instance.
func NewUnitOfWork(pool *pgxpool.Pool, walletSvc *wallet.Service) *PostgresUnitOfWork {
	return &PostgresUnitOfWork{
		pool:      pool,
		walletSvc: walletSvc,
	}
}

// Execute begins a PostgreSQL transaction, executes the callback with the concrete factory,
// and commits or rolls back atomically.
func (u *PostgresUnitOfWork) Execute(ctx context.Context, fn func(factory TransactionRepositoryFactory) error) error {
	if u.pool == nil {
		return errors.New("database pool is uninitialized")
	}

	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	factory := &postgresTransactionRepositoryFactory{
		tx:        tx,
		walletSvc: u.walletSvc,
	}

	if err := fn(factory); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}
