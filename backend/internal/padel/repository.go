package padel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines database operations for padel facilities and slots inventory.
type Repository interface {
	GetFacilityByID(ctx context.Context, id uuid.UUID) (*FacilitySummaryDTO, error)
	GetPadelFacilities(ctx context.Context) ([]FacilitySummaryDTO, error)
	FindSlots(ctx context.Context, facilityID uuid.UUID, startFrom, startBefore time.Time) ([]Slot, error)
	GetSlotByID(ctx context.Context, id uuid.UUID) (*Slot, error)
	CreateSlot(ctx context.Context, slot *Slot) (*Slot, error)
	BatchCreateSlots(ctx context.Context, slots []Slot) (created int, skipped int, err error)
	UpdateSlot(ctx context.Context, id uuid.UUID, pricePaisa *int64, status *SlotStatus) (*Slot, error)
	HoldSlot(ctx context.Context, slotID uuid.UUID, userID uuid.UUID, now time.Time, holdDuration time.Duration) (*Slot, error)
	ReleaseHold(ctx context.Context, slotID uuid.UUID, userID uuid.UUID) (*Slot, error)
	CleanExpiredHolds(ctx context.Context, now time.Time) (int64, error)
	VerifyTicket(ctx context.Context, checkInCode string, bookingID *uuid.UUID, attendantID uuid.UUID, now time.Time) (*VerifyTicketResponse, error)
}

// PostgresRepository implements Repository using a pgxpool.Pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new PostgresRepository instance.
func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// GetFacilityByID looks up a facility by its UUID.
func (r *PostgresRepository) GetFacilityByID(ctx context.Context, id uuid.UUID) (*FacilitySummaryDTO, error) {
	const query = `
		SELECT id, slug, name, status
		FROM giki_sports.facilities
		WHERE id = $1;
	`
	var fac FacilitySummaryDTO
	var facID uuid.UUID
	err := r.pool.QueryRow(ctx, query, id).Scan(&facID, &fac.Slug, &fac.Name, &fac.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFacilityNotFound
		}
		return nil, fmt.Errorf("failed to query facility %s: %w", id, err)
	}
	fac.ID = facID.String()
	return &fac, nil
}

// GetPadelFacilities returns active facilities specifically for Padel.
func (r *PostgresRepository) GetPadelFacilities(ctx context.Context) ([]FacilitySummaryDTO, error) {
	const query = `
		SELECT id, slug, name, status
		FROM giki_sports.facilities
		WHERE status = 'active'
		  AND (slug ILIKE '%padel%' OR name ILIKE '%padel%')
		ORDER BY name ASC;
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query padel facilities: %w", err)
	}
	defer rows.Close()

	var facilities []FacilitySummaryDTO
	for rows.Next() {
		var fac FacilitySummaryDTO
		var facID uuid.UUID
		if err := rows.Scan(&facID, &fac.Slug, &fac.Name, &fac.Status); err != nil {
			return nil, fmt.Errorf("failed to scan facility: %w", err)
		}
		fac.ID = facID.String()
		facilities = append(facilities, fac)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("facilities row iteration error: %w", err)
	}
	return facilities, nil
}

// FindSlots queries all slots for a given facility within a start_time window [startFrom, startBefore).
func (r *PostgresRepository) FindSlots(ctx context.Context, facilityID uuid.UUID, startFrom, startBefore time.Time) ([]Slot, error) {
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
		WHERE facility_id = $1 
		  AND start_time >= $2 
		  AND start_time < $3
		ORDER BY start_time ASC;
	`
	rows, err := r.pool.Query(ctx, query, facilityID, startFrom, startBefore)
	if err != nil {
		return nil, fmt.Errorf("failed to query slots: %w", err)
	}
	defer rows.Close()

	var slots []Slot
	for rows.Next() {
		var s Slot
		var statusStr string
		err := rows.Scan(
			&s.ID,
			&s.FacilityID,
			&s.StartTime,
			&s.EndTime,
			&statusStr,
			&s.HolderID,
			&s.HoldExpiresAt,
			&s.PricePaisa,
			&s.CreatedAt,
			&s.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan slot row: %w", err)
		}
		s.Status = SlotStatus(statusStr)
		slots = append(slots, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("slots row iteration error: %w", err)
	}
	return slots, nil
}

// GetSlotByID retrieves a single slot by its ID.
func (r *PostgresRepository) GetSlotByID(ctx context.Context, id uuid.UUID) (*Slot, error) {
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
		WHERE id = $1;
	`
	var s Slot
	var statusStr string
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&s.ID,
		&s.FacilityID,
		&s.StartTime,
		&s.EndTime,
		&statusStr,
		&s.HolderID,
		&s.HoldExpiresAt,
		&s.PricePaisa,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSlotNotFound
		}
		return nil, fmt.Errorf("failed to query slot %s: %w", id, err)
	}
	s.Status = SlotStatus(statusStr)
	return &s, nil
}

// CreateSlot inserts a new slot into the database.
func (r *PostgresRepository) CreateSlot(ctx context.Context, slot *Slot) (*Slot, error) {
	if err := slot.Validate(); err != nil {
		return nil, err
	}
	const query = `
		INSERT INTO giki_sports.padel_slots (
			facility_id, 
			start_time, 
			end_time, 
			status, 
			holder_id, 
			hold_expires_at, 
			price_paisa
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at;
	`
	err := r.pool.QueryRow(
		ctx,
		query,
		slot.FacilityID,
		slot.StartTime,
		slot.EndTime,
		string(slot.Status),
		slot.HolderID,
		slot.HoldExpiresAt,
		slot.PricePaisa,
	).Scan(&slot.ID, &slot.CreatedAt, &slot.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert slot: %w", err)
	}
	return slot, nil
}

// BatchCreateSlots creates multiple slots within a single transaction.
// Uses ON CONFLICT (facility_id, start_time) DO NOTHING to safely skip existing slots.
func (r *PostgresRepository) BatchCreateSlots(ctx context.Context, slots []Slot) (created int, skipped int, err error) {
	if len(slots) == 0 {
		return 0, 0, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to begin batch slot insert transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	const insertSQL = `
		INSERT INTO giki_sports.padel_slots (
			id,
			facility_id, 
			start_time, 
			end_time, 
			status, 
			price_paisa
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (facility_id, start_time) DO NOTHING
		RETURNING id;
	`

	for i := range slots {
		slot := &slots[i]
		if err := slot.Validate(); err != nil {
			return 0, 0, fmt.Errorf("slot at index %d failed validation: %w", i, err)
		}
		if slot.ID == uuid.Nil {
			slot.ID = uuid.New()
		}

		var insertedID uuid.UUID
		err := tx.QueryRow(
			ctx,
			insertSQL,
			slot.ID,
			slot.FacilityID,
			slot.StartTime,
			slot.EndTime,
			string(slot.Status),
			slot.PricePaisa,
		).Scan(&insertedID)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// ON CONFLICT DO NOTHING returned no row -> skipped
				skipped++
				continue
			}
			return 0, 0, fmt.Errorf("failed to insert batch slot %s: %w", slot.StartTime, err)
		}
		created++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("failed to commit batch slot insert: %w", err)
	}

	return created, skipped, nil
}

// UpdateSlot updates an existing slot's price and/or status.
// Enforces that an actively BOOKED slot cannot have its price or status changed.
func (r *PostgresRepository) UpdateSlot(ctx context.Context, id uuid.UUID, pricePaisa *int64, status *SlotStatus) (*Slot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin update transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Fetch current slot state with row lock
	const selectSQL = `
		SELECT status
		FROM giki_sports.padel_slots
		WHERE id = $1
		FOR UPDATE;
	`
	var currentStatus string
	err = tx.QueryRow(ctx, selectSQL, id).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSlotNotFound
		}
		return nil, fmt.Errorf("failed to lock slot %s: %w", id, err)
	}

	if SlotStatus(currentStatus) == SlotStatusBooked {
		return nil, ErrSlotBooked
	}

	// Build update query dynamically
	updateSQL := `
		UPDATE giki_sports.padel_slots
		SET updated_at = now()
	`
	var args []any
	args = append(args, id) // $1

	argIdx := 2
	if pricePaisa != nil {
		if *pricePaisa < 0 {
			return nil, ErrNegativePrice
		}
		updateSQL += fmt.Sprintf(", price_paisa = $%d", argIdx)
		args = append(args, *pricePaisa)
		argIdx++
	}

	if status != nil {
		if !IsValidSlotStatus(string(*status)) {
			return nil, ErrInvalidStatus
		}
		if *status == SlotStatusHeld {
			return nil, errors.New("cannot manually update slot status to HELD via admin update")
		}
		updateSQL += fmt.Sprintf(", status = $%d", argIdx)
		args = append(args, string(*status))
		argIdx++
	}

	updateSQL += `
		WHERE id = $1
		RETURNING 
			id, 
			facility_id, 
			start_time, 
			end_time, 
			status, 
			holder_id, 
			hold_expires_at, 
			price_paisa, 
			created_at, 
			updated_at;
	`

	var s Slot
	var statusStr string
	err = tx.QueryRow(ctx, updateSQL, args...).Scan(
		&s.ID,
		&s.FacilityID,
		&s.StartTime,
		&s.EndTime,
		&statusStr,
		&s.HolderID,
		&s.HoldExpiresAt,
		&s.PricePaisa,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to execute slot update: %w", err)
	}
	s.Status = SlotStatus(statusStr)

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit slot update: %w", err)
	}

	return &s, nil
}

// HoldSlot acquires an exclusive 5-minute hold on a slot using row-level locking (SELECT ... FOR UPDATE).
// Concurrency & Invariants (QR-01, FR-03):
// - Row-level lock guarantees serialized hold evaluation.
// - If the slot is AVAILABLE: Hold is granted.
// - If the slot is HELD and hold has expired (lazy expiration): Hold is granted to the new contender.
// - If the slot is HELD by the same user: Hold window is refreshed/extended.
// - If the slot is HELD by another user with an active hold: ErrSlotConflict (HTTP 409) is returned.
// - If the slot is BOOKED or BLOCKED: ErrSlotUnavailable (HTTP 409) is returned.
func (r *PostgresRepository) HoldSlot(ctx context.Context, slotID uuid.UUID, userID uuid.UUID, now time.Time, holdDuration time.Duration) (*Slot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin hold transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	const selectSQL = `
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
	var statusStr string
	err = tx.QueryRow(ctx, selectSQL, slotID).Scan(
		&s.ID,
		&s.FacilityID,
		&s.StartTime,
		&s.EndTime,
		&statusStr,
		&s.HolderID,
		&s.HoldExpiresAt,
		&s.PricePaisa,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSlotNotFound
		}
		return nil, fmt.Errorf("failed to query slot for hold: %w", err)
	}
	s.Status = SlotStatus(statusStr)

	// Invariant evaluation
	switch s.Status {
	case SlotStatusBooked, SlotStatusBlocked:
		return nil, ErrSlotUnavailable
	case SlotStatusHeld:
		if s.HolderID != nil && *s.HolderID != userID && !s.IsHoldExpired(now) {
			return nil, ErrSlotConflict
		}
	case SlotStatusAvailable:
		// Available -> proceed with hold
	default:
		return nil, ErrSlotUnavailable
	}

	holdExpiresAt := now.Add(holdDuration)
	const updateSQL = `
		UPDATE giki_sports.padel_slots
		SET 
			status = 'HELD',
			holder_id = $1,
			hold_expires_at = $2,
			updated_at = now()
		WHERE id = $3
		RETURNING 
			id, 
			facility_id, 
			start_time, 
			end_time, 
			status, 
			holder_id, 
			hold_expires_at, 
			price_paisa, 
			created_at, 
			updated_at;
	`
	var updated Slot
	var updatedStatus string
	err = tx.QueryRow(ctx, updateSQL, userID, holdExpiresAt, slotID).Scan(
		&updated.ID,
		&updated.FacilityID,
		&updated.StartTime,
		&updated.EndTime,
		&updatedStatus,
		&updated.HolderID,
		&updated.HoldExpiresAt,
		&updated.PricePaisa,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update slot hold: %w", err)
	}
	updated.Status = SlotStatus(updatedStatus)

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit hold transaction: %w", err)
	}

	return &updated, nil
}

// ReleaseHold voluntarily releases an active hold owned by the specified user.
func (r *PostgresRepository) ReleaseHold(ctx context.Context, slotID uuid.UUID, userID uuid.UUID) (*Slot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin release hold transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	const selectSQL = `
		SELECT status, holder_id
		FROM giki_sports.padel_slots
		WHERE id = $1
		FOR UPDATE;
	`
	var statusStr string
	var holderID *uuid.UUID
	err = tx.QueryRow(ctx, selectSQL, slotID).Scan(&statusStr, &holderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSlotNotFound
		}
		return nil, fmt.Errorf("failed to query slot for release: %w", err)
	}

	if SlotStatus(statusStr) != SlotStatusHeld {
		return nil, errors.New("slot is not currently held")
	}

	if holderID != nil && *holderID != userID {
		return nil, ErrHoldNotOwned
	}

	const updateSQL = `
		UPDATE giki_sports.padel_slots
		SET 
			status = 'AVAILABLE',
			holder_id = NULL,
			hold_expires_at = NULL,
			updated_at = now()
		WHERE id = $1
		RETURNING 
			id, 
			facility_id, 
			start_time, 
			end_time, 
			status, 
			holder_id, 
			hold_expires_at, 
			price_paisa, 
			created_at, 
			updated_at;
	`
	var updated Slot
	var updatedStatus string
	err = tx.QueryRow(ctx, updateSQL, slotID).Scan(
		&updated.ID,
		&updated.FacilityID,
		&updated.StartTime,
		&updated.EndTime,
		&updatedStatus,
		&updated.HolderID,
		&updated.HoldExpiresAt,
		&updated.PricePaisa,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to release hold: %w", err)
	}
	updated.Status = SlotStatus(updatedStatus)

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit release hold: %w", err)
	}

	return &updated, nil
}

// CleanExpiredHolds resets any held slots whose hold_expires_at <= now to AVAILABLE.
func (r *PostgresRepository) CleanExpiredHolds(ctx context.Context, now time.Time) (int64, error) {
	const query = `
		UPDATE giki_sports.padel_slots
		SET status = 'AVAILABLE', holder_id = NULL, hold_expires_at = NULL, updated_at = now()
		WHERE status = 'HELD' AND hold_expires_at <= $1;
	`
	cmdTag, err := r.pool.Exec(ctx, query, now)
	if err != nil {
		return 0, fmt.Errorf("failed to clean expired holds: %w", err)
	}
	return cmdTag.RowsAffected(), nil
}

// VerifyTicket validates a check-in code, transitions the booking from CONFIRMED to CHECKED_IN,
// records attendant metadata, and writes an administrative audit log entry (FR-03, TC-FR-03-03).
func (r *PostgresRepository) VerifyTicket(ctx context.Context, checkInCode string, bookingID *uuid.UUID, attendantID uuid.UUID, now time.Time) (*VerifyTicketResponse, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Lock the booking row with SELECT ... FOR UPDATE
	var selectQuery string
	var args []interface{}
	if bookingID != nil && *bookingID != uuid.Nil {
		selectQuery = `
			SELECT 
				b.id,
				b.slot_id,
				b.user_id,
				b.status,
				b.fee_paisa,
				b.check_in_code,
				b.checked_in_at,
				b.checked_in_by,
				s.facility_id,
				s.start_time,
				s.end_time,
				f.name AS facility_name,
				u.email AS user_email,
				u.display_name AS user_name
			FROM giki_sports.padel_bookings b
			JOIN giki_sports.padel_slots s ON b.slot_id = s.id
			JOIN giki_sports.facilities f ON s.facility_id = f.id
			JOIN giki_sports.users u ON b.user_id = u.id
			WHERE b.check_in_code = $1 AND b.id = $2
			FOR UPDATE OF b;
		`
		args = []interface{}{checkInCode, *bookingID}
	} else {
		selectQuery = `
			SELECT 
				b.id,
				b.slot_id,
				b.user_id,
				b.status,
				b.fee_paisa,
				b.check_in_code,
				b.checked_in_at,
				b.checked_in_by,
				s.facility_id,
				s.start_time,
				s.end_time,
				f.name AS facility_name,
				u.email AS user_email,
				u.display_name AS user_name
			FROM giki_sports.padel_bookings b
			JOIN giki_sports.padel_slots s ON b.slot_id = s.id
			JOIN giki_sports.facilities f ON s.facility_id = f.id
			JOIN giki_sports.users u ON b.user_id = u.id
			WHERE b.check_in_code = $1
			FOR UPDATE OF b;
		`
		args = []interface{}{checkInCode}
	}

	var bID, slotID, userID, facilityID uuid.UUID
	var status, code, facilityName, userEmail, userName string
	var feePaisa int64
	var checkedInAt *time.Time
	var checkedInBy *uuid.UUID
	var startTime, endTime time.Time

	err = tx.QueryRow(ctx, selectQuery, args...).Scan(
		&bID,
		&slotID,
		&userID,
		&status,
		&feePaisa,
		&code,
		&checkedInAt,
		&checkedInBy,
		&facilityID,
		&startTime,
		&endTime,
		&facilityName,
		&userEmail,
		&userName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("failed to query booking: %w", err)
	}

	// 2. Validate current state transitions
	switch BookingStatus(status) {
	case BookingStatusCheckedIn:
		return nil, ErrTicketAlreadyCheckedIn
	case BookingStatusCancelled:
		return nil, ErrBookingCancelled
	case BookingStatusConfirmed:
		// Valid state for check-in
	default:
		return nil, fmt.Errorf("booking in unexpected status %s cannot be checked in", status)
	}

	// 3. Transition booking status to CHECKED_IN
	const updateQuery = `
		UPDATE giki_sports.padel_bookings
		SET 
			status = 'CHECKED_IN',
			checked_in_at = $1,
			checked_in_by = $2,
			updated_at = $1
		WHERE id = $3;
	`
	_, err = tx.Exec(ctx, updateQuery, now, attendantID, bID)
	if err != nil {
		return nil, fmt.Errorf("failed to update booking check-in status: %w", err)
	}

	// 4. Record audit log entry (FR-09, §6.2)
	auditDetails := map[string]interface{}{
		"slot_id":       slotID.String(),
		"facility_id":   facilityID.String(),
		"check_in_code": code,
		"checked_in_at": now.UTC().Format(time.RFC3339),
	}
	detailsJSON, err := json.Marshal(auditDetails)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal audit details: %w", err)
	}

	const insertAuditQuery = `
		INSERT INTO giki_sports.audit_logs (
			actor_id,
			action,
			target_type,
			target_id,
			details,
			status,
			created_at
		) VALUES (
			$1, 'booking.verify_ticket', 'padel_booking', $2, $3, 'SUCCESS', $4
		);
	`
	_, err = tx.Exec(ctx, insertAuditQuery, attendantID, bID, detailsJSON, now)
	if err != nil {
		return nil, fmt.Errorf("failed to record check-in audit log: %w", err)
	}

	// 5. Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit check-in transaction: %w", err)
	}

	return &VerifyTicketResponse{
		BookingID:    bID.String(),
		SlotID:       slotID.String(),
		FacilityID:   facilityID.String(),
		FacilityName: facilityName,
		UserID:       userID.String(),
		UserEmail:    userEmail,
		UserName:     userName,
		StartTime:    startTime.UTC().Format(time.RFC3339),
		EndTime:      endTime.UTC().Format(time.RFC3339),
		Status:       string(BookingStatusCheckedIn),
		FeePaisa:     strconv.FormatInt(feePaisa, 10),
		CheckInCode:  code,
		CheckedInAt:  now.UTC().Format(time.RFC3339),
		CheckedInBy:  attendantID.String(),
	}, nil
}
