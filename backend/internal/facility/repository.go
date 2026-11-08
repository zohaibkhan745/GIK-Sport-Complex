package facility

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines operations for sports facilities and their weekly schedules.
type Repository interface {
	// List returns all facilities with their associated recurring weekly schedules.
	// Returns an empty slice (not nil) if no facilities exist.
	List(ctx context.Context) ([]Facility, error)

	// GetBySlug returns a single facility with its recurring weekly schedules by URL slug.
	// Returns ErrNotFound if no facility matches the given slug.
	GetBySlug(ctx context.Context, slug string) (*Facility, error)

	// UpdateSchedule updates one existing weekly schedule window scoped by facility slug and schedule ID.
	UpdateSchedule(ctx context.Context, facilitySlug string, scheduleID uuid.UUID, input UpdateScheduleInput) (*Schedule, error)

	// UpdateScheduleWithAudit updates a schedule window and writes an administrative audit log
	// within a single atomic PostgreSQL transaction. If either step fails, the entire transaction rolls back.
	UpdateScheduleWithAudit(ctx context.Context, facilitySlug string, scheduleID uuid.UUID, input UpdateScheduleInput, audit AuditParams) (*Schedule, error)

	// DeleteScheduleWithAudit deletes one existing weekly schedule window and writes an administrative audit log
	// within a single atomic PostgreSQL transaction. If either step fails, the entire transaction rolls back.
	DeleteScheduleWithAudit(ctx context.Context, facilitySlug string, scheduleID uuid.UUID, audit AuditParams) error

	// CreateScheduleWithAudit creates one new weekly schedule window scoped by facility slug,
	// recording a schedule.create audit log entry within a single atomic PostgreSQL transaction.
	// If either step fails, the entire transaction rolls back.
	CreateScheduleWithAudit(ctx context.Context, facilitySlug string, input CreateScheduleInput, audit AuditParams) (*Schedule, error)
}

// PostgresRepository implements Repository using a PostgreSQL connection pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new PostgresRepository instance backed by the provided pgxpool.Pool.
func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// List retrieves all facilities and their associated recurring weekly schedules.
//
// Bounded-Query Strategy & N+1 Prevention:
// Rather than executing one query for facilities followed by N queries for schedules (the N+1 anti-pattern),
// List executes exactly two bounded queries:
// 1. Fetches all facilities ordered deterministically by name and slug.
// 2. Fetches all schedules for all facilities in a single bulk query, ordered by facility, day, and time.
//
// Read-Consistency Behavior:
// Each query executes as an independent READ COMMITTED statement against the connection pool.
// Therefore, the two queries do NOT automatically share the same database snapshot. If concurrent transactions
// commit modifications between query 1 and query 2, query 2 reflects those committed changes. For routine read
// operations, this provides optimal throughput without holding prolonged database snapshot locks.
func (r *PostgresRepository) List(ctx context.Context) ([]Facility, error) {
	const listFacilitiesSQL = `
		SELECT 
			id, 
			slug, 
			name, 
			description, 
			location, 
			status, 
			created_at, 
			updated_at
		FROM giki_sports.facilities
		ORDER BY name ASC, slug ASC;
	`

	rows, err := r.pool.Query(ctx, listFacilitiesSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to list facilities: %w", err)
	}
	defer rows.Close()

	var facilities []Facility
	facilityMap := make(map[uuid.UUID]int)

	for rows.Next() {
		var f Facility
		var desc, loc *string
		var statusStr string

		err := rows.Scan(
			&f.ID,
			&f.Slug,
			&f.Name,
			&desc,
			&loc,
			&statusStr,
			&f.CreatedAt,
			&f.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan facility row: %w", err)
		}

		f.Description = desc
		f.Location = loc
		f.Status = OperationalStatus(statusStr)
		f.Schedules = make([]Schedule, 0)

		facilityMap[f.ID] = len(facilities)
		facilities = append(facilities, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("facility rows iteration error: %w", err)
	}

	// For an empty database, return non-nil empty collection
	if len(facilities) == 0 {
		return make([]Facility, 0), nil
	}

	// Bulk query 2: Retrieve all schedules for all facilities in a single bounded query
	const listSchedulesSQL = `
		SELECT 
			id, 
			facility_id, 
			day_of_week, 
			to_char(start_time, 'HH24:MI:SS') AS start_time, 
			to_char(end_time, 'HH24:MI:SS') AS end_time, 
			audience, 
			created_at, 
			updated_at
		FROM giki_sports.schedules
		ORDER BY facility_id ASC, day_of_week ASC, start_time ASC, end_time ASC, audience ASC, id ASC;
	`

	sRows, err := r.pool.Query(ctx, listSchedulesSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to list schedules: %w", err)
	}
	defer sRows.Close()

	for sRows.Next() {
		var s Schedule
		err := sRows.Scan(
			&s.ID,
			&s.FacilityID,
			&s.DayOfWeek,
			&s.StartTime,
			&s.EndTime,
			&s.Audience,
			&s.CreatedAt,
			&s.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan schedule row: %w", err)
		}

		if idx, ok := facilityMap[s.FacilityID]; ok {
			facilities[idx].Schedules = append(facilities[idx].Schedules, s)
		}
	}
	if err := sRows.Err(); err != nil {
		return nil, fmt.Errorf("schedule rows iteration error: %w", err)
	}

	return facilities, nil
}

// GetBySlug retrieves a single facility and its recurring weekly schedules by URL slug.
// If the slug does not exist, returns ErrNotFound.
func (r *PostgresRepository) GetBySlug(ctx context.Context, slug string) (*Facility, error) {
	const getFacilityBySlugSQL = `
		SELECT 
			id, 
			slug, 
			name, 
			description, 
			location, 
			status, 
			created_at, 
			updated_at
		FROM giki_sports.facilities
		WHERE slug = $1;
	`

	var f Facility
	var desc, loc *string
	var statusStr string

	err := r.pool.QueryRow(ctx, getFacilityBySlugSQL, slug).Scan(
		&f.ID,
		&f.Slug,
		&f.Name,
		&desc,
		&loc,
		&statusStr,
		&f.CreatedAt,
		&f.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &NotFoundError{Slug: slug}
		}
		return nil, fmt.Errorf("failed to get facility by slug %q: %w", slug, err)
	}

	f.Description = desc
	f.Location = loc
	f.Status = OperationalStatus(statusStr)
	f.Schedules = make([]Schedule, 0)

	const getSchedulesForFacilitySQL = `
		SELECT 
			id, 
			facility_id, 
			day_of_week, 
			to_char(start_time, 'HH24:MI:SS') AS start_time, 
			to_char(end_time, 'HH24:MI:SS') AS end_time, 
			audience, 
			created_at, 
			updated_at
		FROM giki_sports.schedules
		WHERE facility_id = $1
		ORDER BY day_of_week ASC, start_time ASC, end_time ASC, audience ASC, id ASC;
	`

	sRows, err := r.pool.Query(ctx, getSchedulesForFacilitySQL, f.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedules for facility %q: %w", slug, err)
	}
	defer sRows.Close()

	for sRows.Next() {
		var s Schedule
		err := sRows.Scan(
			&s.ID,
			&s.FacilityID,
			&s.DayOfWeek,
			&s.StartTime,
			&s.EndTime,
			&s.Audience,
			&s.CreatedAt,
			&s.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan schedule row for facility %q: %w", slug, err)
		}

		f.Schedules = append(f.Schedules, s)
	}
	if err := sRows.Err(); err != nil {
		return nil, fmt.Errorf("schedule rows iteration error: %w", err)
	}

	return &f, nil
}

// UpdateSchedule updates one existing weekly schedule window scoped by facility slug and schedule ID.
//
// Scoping & Safety:
// The query strictly conditions on BOTH s.id = $1 AND s.facility_id = f.id AND f.slug = $2.
// If the schedule exists under a different facility, exactly zero rows are updated, and
// ErrScheduleNotFound is returned without modifying either facility or schedule.
//
// Concurrency & Idempotency:
// Executes parameterized SQL with PostgreSQL RETURNING clause to scan the committed row directly.
// If the proposed window collides with another existing window on (facility_id, day_of_week, audience, start_time, end_time),
// the unique constraint uq_schedules_window is caught and mapped to ErrDuplicateSchedule.
func (r *PostgresRepository) UpdateSchedule(ctx context.Context, facilitySlug string, scheduleID uuid.UUID, input UpdateScheduleInput) (*Schedule, error) {
	cleanSlug := strings.TrimSpace(facilitySlug)
	if cleanSlug == "" || scheduleID == uuid.Nil {
		return nil, &ScheduleNotFoundError{FacilitySlug: facilitySlug, ScheduleID: scheduleID}
	}

	if err := input.Validate(); err != nil {
		return nil, err
	}

	const updateScheduleSQL = `
		UPDATE giki_sports.schedules s
		SET 
			day_of_week = $3,
			start_time = $4::time,
			end_time = $5::time,
			audience = $6,
			updated_at = now()
		FROM giki_sports.facilities f
		WHERE s.id = $1
		  AND s.facility_id = f.id
		  AND f.slug = $2
		RETURNING 
			s.id, 
			s.facility_id, 
			s.day_of_week, 
			to_char(s.start_time, 'HH24:MI:SS') AS start_time, 
			to_char(s.end_time, 'HH24:MI:SS') AS end_time, 
			s.audience, 
			s.created_at, 
			s.updated_at;
	`

	var s Schedule
	err := r.pool.QueryRow(ctx, updateScheduleSQL,
		scheduleID,
		cleanSlug,
		input.DayOfWeek,
		input.StartTime,
		input.EndTime,
		input.Audience,
	).Scan(
		&s.ID,
		&s.FacilityID,
		&s.DayOfWeek,
		&s.StartTime,
		&s.EndTime,
		&s.Audience,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ScheduleNotFoundError{FacilitySlug: facilitySlug, ScheduleID: scheduleID}
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" && pgErr.ConstraintName == "uq_schedules_window" {
				return nil, &DuplicateScheduleError{
					FacilitySlug: facilitySlug,
					ScheduleID:   scheduleID,
					DayOfWeek:    input.DayOfWeek,
					StartTime:    input.StartTime,
					EndTime:      input.EndTime,
					Audience:     input.Audience,
				}
			}
			if pgErr.Code == "23514" {
				return nil, &ScheduleValidationError{
					Message: fmt.Sprintf("database check constraint failed: %s", pgErr.ConstraintName),
				}
			}
		}

		return nil, fmt.Errorf("failed to update schedule %s for facility %q: %w", scheduleID, facilitySlug, err)
	}

	return &s, nil
}

// UpdateScheduleWithAudit updates a schedule window and writes an administrative audit log
// within a single atomic PostgreSQL transaction. If either step fails, the entire transaction rolls back.
//
// Transaction Mechanics:
// 1. Begins a transaction on the PostgreSQL pool.
// 2. Selects the existing schedule row conditioned on BOTH schedule ID and facility slug,
//    locking the schedule row with FOR UPDATE OF s to prevent concurrent conflicting writes and to capture
//    the previous window parameters (dayOfWeek, startTime, endTime, audience) for the audit trail.
// 3. Executes the schedule UPDATE, capturing the returned modified schedule.
// 4. Marshals the audit details payload containing facilitySlug, facilityID, old_window, and new_window.
// 5. Inserts an administrative audit log row into giki_sports.audit_logs capturing actorID,
//    action 'schedule.update', target_type 'facility_schedule', target_id scheduleID, details, ip, userAgent,
//    and status 'SUCCESS'.
// 6. If the audit insert (or any preceding step) fails (e.g., actor foreign key violation), the deferred
//    tx.Rollback() ensures the schedule update is completely rolled back, leaving the schedule unchanged.
// 7. If both the update and audit insert succeed, tx.Commit() atomically commits both operations.
func (r *PostgresRepository) UpdateScheduleWithAudit(
	ctx context.Context,
	facilitySlug string,
	scheduleID uuid.UUID,
	input UpdateScheduleInput,
	audit AuditParams,
) (*Schedule, error) {
	cleanSlug := strings.TrimSpace(facilitySlug)
	if cleanSlug == "" || scheduleID == uuid.Nil {
		return nil, &ScheduleNotFoundError{FacilitySlug: facilitySlug, ScheduleID: scheduleID}
	}

	if err := input.Validate(); err != nil {
		return nil, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin schedule update transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Step 1: Select existing schedule with FOR UPDATE OF s to capture previous window and verify facility ownership
	const selectExistingSQL = `
		SELECT 
			s.facility_id,
			s.day_of_week,
			to_char(s.start_time, 'HH24:MI:SS') AS start_time,
			to_char(s.end_time, 'HH24:MI:SS') AS end_time,
			s.audience
		FROM giki_sports.schedules s
		JOIN giki_sports.facilities f ON s.facility_id = f.id
		WHERE s.id = $1 AND f.slug = $2
		FOR UPDATE OF s;
	`

	var facID uuid.UUID
	var oldDay int
	var oldStart, oldEnd, oldAudience string
	err = tx.QueryRow(ctx, selectExistingSQL, scheduleID, cleanSlug).Scan(
		&facID,
		&oldDay,
		&oldStart,
		&oldEnd,
		&oldAudience,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ScheduleNotFoundError{FacilitySlug: facilitySlug, ScheduleID: scheduleID}
		}
		return nil, fmt.Errorf("failed to lock schedule %s for facility %q: %w", scheduleID, facilitySlug, err)
	}

	// Step 2: Execute the schedule UPDATE
	const updateSQL = `
		UPDATE giki_sports.schedules
		SET 
			day_of_week = $2,
			start_time = $3::time,
			end_time = $4::time,
			audience = $5,
			updated_at = now()
		WHERE id = $1
		RETURNING 
			id, facility_id, day_of_week,
			to_char(start_time, 'HH24:MI:SS') AS start_time,
			to_char(end_time, 'HH24:MI:SS') AS end_time,
			audience, created_at, updated_at;
	`

	var s Schedule
	err = tx.QueryRow(ctx, updateSQL,
		scheduleID,
		input.DayOfWeek,
		input.StartTime,
		input.EndTime,
		input.Audience,
	).Scan(
		&s.ID,
		&s.FacilityID,
		&s.DayOfWeek,
		&s.StartTime,
		&s.EndTime,
		&s.Audience,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" && pgErr.ConstraintName == "uq_schedules_window" {
				return nil, &DuplicateScheduleError{
					FacilitySlug: facilitySlug,
					ScheduleID:   scheduleID,
					DayOfWeek:    input.DayOfWeek,
					StartTime:    input.StartTime,
					EndTime:      input.EndTime,
					Audience:     input.Audience,
				}
			}
			if pgErr.Code == "23514" {
				return nil, &ScheduleValidationError{
					Message: fmt.Sprintf("database check constraint failed: %s", pgErr.ConstraintName),
				}
			}
		}
		return nil, fmt.Errorf("failed to update schedule %s: %w", scheduleID, err)
	}

	// Step 3: Marshal audit details capturing previous and new window
	detailsMap := map[string]interface{}{
		"facility_slug": cleanSlug,
		"facility_id":   facID.String(),
		"old_window": map[string]interface{}{
			"day_of_week": oldDay,
			"start_time":  oldStart,
			"end_time":    oldEnd,
			"audience":    oldAudience,
		},
		"new_window": map[string]interface{}{
			"day_of_week": s.DayOfWeek,
			"start_time":  s.StartTime,
			"end_time":    s.EndTime,
			"audience":    s.Audience,
		},
	}
	detailsJSON, err := json.Marshal(detailsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal audit details: %w", err)
	}

	// Step 4: Insert audit log entry within the same transaction
	var ip *string
	if trimmedIP := strings.TrimSpace(audit.IPAddress); trimmedIP != "" {
		ip = &trimmedIP
	}
	var ua *string
	if trimmedUA := strings.TrimSpace(audit.UserAgent); trimmedUA != "" {
		ua = &trimmedUA
	}

	const insertAuditSQL = `
		INSERT INTO giki_sports.audit_logs (
			actor_id,
			action,
			target_type,
			target_id,
			details,
			ip_address,
			user_agent,
			status,
			created_at
		) VALUES (
			$1, 'schedule.update', 'facility_schedule', $2, $3, $4, $5, 'SUCCESS', now()
		);
	`

	_, err = tx.Exec(ctx, insertAuditSQL,
		audit.ActorID,
		scheduleID,
		detailsJSON,
		ip,
		ua,
	)
	if err != nil {
		// Audit insert failed! The deferred tx.Rollback() ensures the schedule update is not committed.
		return nil, fmt.Errorf("failed to write audit log for schedule update: %w", err)
	}

	// Step 5: Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit schedule update and audit log transaction: %w", err)
	}

	return &s, nil
}

// DeleteScheduleWithAudit deletes one existing weekly schedule window scoped by facility slug and schedule ID,
// recording a schedule.delete audit log entry within a single atomic PostgreSQL transaction (FR-09).
//
// Atomicity & Audit Guarantees:
// 1. Begins a transaction on the PostgreSQL pool.
// 2. Selects the existing schedule row conditioned on BOTH schedule ID and facility slug,
//    locking the schedule row with FOR UPDATE OF s to verify facility ownership and to capture
//    its previous window parameters (dayOfWeek, startTime, endTime, audience) before deletion.
// 3. Executes DELETE FROM giki_sports.schedules WHERE id = $1.
// 4. Marshals the audit details payload containing facility_slug, facility_id, and old_window.
// 5. Inserts an administrative audit log row into giki_sports.audit_logs capturing actorID,
//    action 'schedule.delete', target_type 'facility_schedule', target_id scheduleID, details, ip, userAgent,
//    and status 'SUCCESS'.
// 6. Because audit_logs.target_id has no foreign key constraint to schedules, the audit record remains
//    permanently intact and readable even after the schedule row is removed.
// 7. If the audit insert (or any preceding step) fails (e.g. actor foreign key violation), the deferred
//    tx.Rollback() ensures the schedule row is NOT deleted, leaving the database state completely intact.
// 8. Success is reported only after tx.Commit() atomically commits both the deletion and the audit log.
func (r *PostgresRepository) DeleteScheduleWithAudit(
	ctx context.Context,
	facilitySlug string,
	scheduleID uuid.UUID,
	audit AuditParams,
) error {
	cleanSlug := strings.TrimSpace(facilitySlug)
	if cleanSlug == "" || scheduleID == uuid.Nil {
		return &ScheduleNotFoundError{FacilitySlug: facilitySlug, ScheduleID: scheduleID}
	}

	if audit.ActorID == uuid.Nil {
		return errors.New("audit actor ID is required")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin schedule delete transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Step 1: Select existing schedule with FOR UPDATE OF s to capture previous window and verify facility ownership
	const selectExistingSQL = `
		SELECT 
			s.facility_id,
			s.day_of_week,
			to_char(s.start_time, 'HH24:MI:SS') AS start_time,
			to_char(s.end_time, 'HH24:MI:SS') AS end_time,
			s.audience
		FROM giki_sports.schedules s
		JOIN giki_sports.facilities f ON s.facility_id = f.id
		WHERE s.id = $1 AND f.slug = $2
		FOR UPDATE OF s;
	`

	var facID uuid.UUID
	var oldDay int
	var oldStart, oldEnd, oldAudience string
	err = tx.QueryRow(ctx, selectExistingSQL, scheduleID, cleanSlug).Scan(
		&facID,
		&oldDay,
		&oldStart,
		&oldEnd,
		&oldAudience,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ScheduleNotFoundError{FacilitySlug: facilitySlug, ScheduleID: scheduleID}
		}
		return fmt.Errorf("failed to lock schedule %s for facility %q: %w", scheduleID, facilitySlug, err)
	}

	// Step 2: Delete the schedule row
	const deleteSQL = `
		DELETE FROM giki_sports.schedules
		WHERE id = $1;
	`

	cmdTag, err := tx.Exec(ctx, deleteSQL, scheduleID)
	if err != nil {
		return fmt.Errorf("failed to delete schedule %s: %w", scheduleID, err)
	}
	if cmdTag.RowsAffected() == 0 {
		return &ScheduleNotFoundError{FacilitySlug: facilitySlug, ScheduleID: scheduleID}
	}

	// Step 3: Marshal audit details capturing previous window and facility context
	detailsMap := map[string]interface{}{
		"facility_slug": cleanSlug,
		"facility_id":   facID.String(),
		"old_window": map[string]interface{}{
			"day_of_week": oldDay,
			"start_time":  oldStart,
			"end_time":    oldEnd,
			"audience":    oldAudience,
		},
	}
	detailsJSON, err := json.Marshal(detailsMap)
	if err != nil {
		return fmt.Errorf("failed to marshal audit details: %w", err)
	}

	// Step 4: Insert audit log entry within the same transaction
	var ip *string
	if trimmedIP := strings.TrimSpace(audit.IPAddress); trimmedIP != "" {
		ip = &trimmedIP
	}
	var ua *string
	if trimmedUA := strings.TrimSpace(audit.UserAgent); trimmedUA != "" {
		ua = &trimmedUA
	}

	const insertAuditSQL = `
		INSERT INTO giki_sports.audit_logs (
			actor_id,
			action,
			target_type,
			target_id,
			details,
			ip_address,
			user_agent,
			status,
			created_at
		) VALUES (
			$1, 'schedule.delete', 'facility_schedule', $2, $3, $4, $5, 'SUCCESS', now()
		);
	`

	_, err = tx.Exec(ctx, insertAuditSQL,
		audit.ActorID,
		scheduleID,
		detailsJSON,
		ip,
		ua,
	)
	if err != nil {
		// Audit insert failed! The deferred tx.Rollback() ensures the schedule deletion is not committed.
		return fmt.Errorf("failed to write audit log for schedule delete: %w", err)
	}

	// Step 5: Commit transaction atomically
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit schedule delete and audit log transaction: %w", err)
	}

	return nil
}

// CreateScheduleWithAudit creates one new weekly schedule window scoped by facility slug,
// recording a schedule.create audit log entry within a single atomic PostgreSQL transaction (FR-09).
//
// Atomicity & Audit Guarantees:
// 1. Validates input parameters (dayOfWeek 1-7, supported audience, start < end, 24:00:00 allowed only as end_time).
// 2. Begins a transaction on the PostgreSQL pool.
// 3. Resolves the facility ID by facility slug. If unknown or blank, returns &NotFoundError{Slug: facilitySlug}.
// 4. Inserts the new schedule row, letting PostgreSQL generate a new UUID for id (gen_random_uuid()).
//    If an exact duplicate window exists for the facility, returns &DuplicateScheduleError.
// 5. Marshals the audit details payload containing facility_slug, facility_id, schedule_id, and saved_window.
// 6. Inserts an administrative audit log row into giki_sports.audit_logs capturing actorID,
//    action 'schedule.create', target_type 'facility_schedule', target_id scheduleID, details, ip, userAgent,
//    and status 'SUCCESS'.
// 7. If the audit insert (or any preceding step) fails (e.g. actor foreign key violation), the deferred
//    tx.Rollback() ensures the schedule creation is completely rolled back, leaving the schedules table unchanged.
// 8. The saved Schedule model is returned ONLY after tx.Commit() successfully commits both the schedule
//    and the audit log.
func (r *PostgresRepository) CreateScheduleWithAudit(
	ctx context.Context,
	facilitySlug string,
	input CreateScheduleInput,
	audit AuditParams,
) (*Schedule, error) {
	cleanSlug := strings.TrimSpace(facilitySlug)
	if cleanSlug == "" {
		return nil, &NotFoundError{Slug: facilitySlug}
	}

	if audit.ActorID == uuid.Nil {
		return nil, errors.New("audit actor ID is required")
	}

	if err := input.Validate(); err != nil {
		return nil, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin schedule create transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Step 1: Resolve the facility by slug
	const selectFacilitySQL = `
		SELECT id
		FROM giki_sports.facilities
		WHERE slug = $1;
	`

	var facilityID uuid.UUID
	err = tx.QueryRow(ctx, selectFacilitySQL, cleanSlug).Scan(&facilityID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &NotFoundError{Slug: facilitySlug}
		}
		return nil, fmt.Errorf("failed to resolve facility %q: %w", facilitySlug, err)
	}

	// Step 2: Insert the new schedule window
	const insertScheduleSQL = `
		INSERT INTO giki_sports.schedules (
			facility_id,
			day_of_week,
			start_time,
			end_time,
			audience,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3::time, $4::time, $5, now(), now()
		)
		RETURNING 
			id, 
			facility_id, 
			day_of_week,
			to_char(start_time, 'HH24:MI:SS') AS start_time,
			to_char(end_time, 'HH24:MI:SS') AS end_time,
			audience, 
			created_at, 
			updated_at;
	`

	var s Schedule
	err = tx.QueryRow(ctx, insertScheduleSQL,
		facilityID,
		input.DayOfWeek,
		input.StartTime,
		input.EndTime,
		input.Audience,
	).Scan(
		&s.ID,
		&s.FacilityID,
		&s.DayOfWeek,
		&s.StartTime,
		&s.EndTime,
		&s.Audience,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" && pgErr.ConstraintName == "uq_schedules_window" {
				return nil, &DuplicateScheduleError{
					FacilitySlug: facilitySlug,
					ScheduleID:   uuid.Nil,
					DayOfWeek:    input.DayOfWeek,
					StartTime:    input.StartTime,
					EndTime:      input.EndTime,
					Audience:     input.Audience,
				}
			}
			if pgErr.Code == "23514" {
				return nil, &ScheduleValidationError{
					Message: fmt.Sprintf("database check constraint failed: %s", pgErr.ConstraintName),
				}
			}
		}
		return nil, fmt.Errorf("failed to insert schedule for facility %q: %w", facilitySlug, err)
	}

	// Step 3: Marshal audit details capturing facility and saved window
	windowValues := map[string]interface{}{
		"day_of_week": s.DayOfWeek,
		"start_time":  s.StartTime,
		"end_time":    s.EndTime,
		"audience":    s.Audience,
	}
	detailsMap := map[string]interface{}{
		"facility_slug": cleanSlug,
		"facility_id":   facilityID.String(),
		"schedule_id":   s.ID.String(),
		"saved_window":  windowValues,
		"new_window":    windowValues,
	}
	detailsJSON, err := json.Marshal(detailsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal audit details: %w", err)
	}

	// Step 4: Insert audit log entry within the same transaction
	var ip *string
	if trimmedIP := strings.TrimSpace(audit.IPAddress); trimmedIP != "" {
		ip = &trimmedIP
	}
	var ua *string
	if trimmedUA := strings.TrimSpace(audit.UserAgent); trimmedUA != "" {
		ua = &trimmedUA
	}

	const insertAuditSQL = `
		INSERT INTO giki_sports.audit_logs (
			actor_id,
			action,
			target_type,
			target_id,
			details,
			ip_address,
			user_agent,
			status,
			created_at
		) VALUES (
			$1, 'schedule.create', 'facility_schedule', $2, $3, $4, $5, 'SUCCESS', now()
		);
	`

	_, err = tx.Exec(ctx, insertAuditSQL,
		audit.ActorID,
		s.ID,
		detailsJSON,
		ip,
		ua,
	)
	if err != nil {
		// Audit insert failed! The deferred tx.Rollback() ensures the schedule creation is not committed.
		return nil, fmt.Errorf("failed to write audit log for schedule create: %w", err)
	}

	// Step 5: Commit transaction atomically
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit schedule create and audit log transaction: %w", err)
	}

	return &s, nil
}
