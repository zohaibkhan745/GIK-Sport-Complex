package content

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

// Repository defines read and write operations for content items and drafts.
type Repository interface {
	// ListPublishedByKind returns at most the latest 50 published items of the requested kind (announcement or achievement)
	// that have a publication timestamp at or before the current wall-clock instant (published_at <= now()).
	// Drafts and future-dated items are excluded at the SQL level.
	// Results are ordered newest publication first (published_at DESC), with a deterministic id ASC tie-breaker.
	// Returns an empty slice (not nil) when no items match.
	ListPublishedByKind(ctx context.Context, kind Kind) ([]PublishedItem, error)

	// CreateDraft creates a new incomplete or complete draft content item and writes a
	// content.draft.create audit log entry within a single atomic PostgreSQL transaction.
	// Sets creator_id from the trusted actor, status to 'draft', and published_at to NULL.
	// If either the draft insert or audit insert fails, the entire transaction rolls back.
	CreateDraft(ctx context.Context, input CreateDraftInput, audit AuditParams) (*Draft, error)

	// GetDraftByID retrieves a single draft by ID. If the item does not exist, ErrDraftNotFound is returned.
	// If the item exists but is not in draft status (e.g. published), ErrNotADraft is returned.
	GetDraftByID(ctx context.Context, id uuid.UUID) (*Draft, error)

	// UpdateDraft updates title and content of an existing draft and writes a content.draft.update
	// audit log entry within a single atomic PostgreSQL transaction.
	// Preserves kind, creator_id, and created_at. Enforces the draft condition in the write itself.
	// Returns ErrDraftNotFound if the item does not exist, or ErrNotADraft if it is not a draft.
	UpdateDraft(ctx context.Context, id uuid.UUID, input UpdateDraftInput, audit AuditParams) (*Draft, error)

	// PublishDraft transitions one existing draft to 'published' status and records a content.publish
	// audit log entry within a single atomic PostgreSQL transaction.
	// Locks the target row with FOR UPDATE, validates that title and body are non-blank, sets published_at and updated_at
	// to the current server timestamp, and returns the PublishedItem.
	// If the item does not exist, ErrDraftNotFound is returned.
	// If the item is already published, ErrNotADraft is returned without mutating timestamps or logging an audit entry.
	// If title or content are blank or whitespace-only, ErrIncompleteDraft is returned.
	// If the audit insert fails, the transaction rolls back, leaving the item as an unpublished draft.
	PublishDraft(ctx context.Context, id uuid.UUID, audit AuditParams) (*PublishedItem, error)
}

// PostgresRepository implements Repository using a PostgreSQL connection pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new PostgresRepository instance backed by the provided pgxpool.Pool.
func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// ListPublishedByKind queries giki_sports.content_items for published items matching the requested kind,
// returning at most the latest 50 items ordered by published_at DESC, id ASC.
func (r *PostgresRepository) ListPublishedByKind(ctx context.Context, kind Kind) ([]PublishedItem, error) {
	if err := ValidateKind(kind); err != nil {
		return nil, err
	}

	const listPublishedSQL = `
		SELECT 
			id, 
			kind, 
			title, 
			content, 
			published_at
		FROM giki_sports.content_items
		WHERE kind = $1
		  AND status = 'published'
		  AND published_at <= now()
		ORDER BY published_at DESC, id ASC
		LIMIT 50;
	`

	rows, err := r.pool.Query(ctx, listPublishedSQL, string(kind))
	if err != nil {
		return nil, fmt.Errorf("failed to list published %s items: %w", kind, err)
	}
	defer rows.Close()

	items := make([]PublishedItem, 0)
	for rows.Next() {
		var item PublishedItem
		var kindStr string
		if err := rows.Scan(
			&item.ID,
			&kindStr,
			&item.Title,
			&item.Content,
			&item.PublishedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan published %s row: %w", kind, err)
		}
		item.Kind = Kind(kindStr)
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during published %s iteration: %w", kind, err)
	}

	return items, nil
}

// CreateDraft creates a new draft content item and records a content.draft.create audit log
// within a single atomic PostgreSQL transaction.
func (r *PostgresRepository) CreateDraft(
	ctx context.Context,
	input CreateDraftInput,
	audit AuditParams,
) (*Draft, error) {
	if audit.ActorID == uuid.Nil {
		return nil, &DraftValidationError{Message: "audit actor ID is required"}
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin draft create transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	const insertDraftSQL = `
		INSERT INTO giki_sports.content_items (
			kind,
			title,
			content,
			status,
			creator_id,
			published_at,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, 'draft', $4, NULL, now(), now()
		)
		RETURNING 
			id,
			kind,
			title,
			content,
			status,
			creator_id,
			created_at,
			updated_at,
			published_at;
	`

	var d Draft
	var kindStr string
	err = tx.QueryRow(ctx, insertDraftSQL,
		string(input.Kind),
		input.Title,
		input.Content,
		audit.ActorID,
	).Scan(
		&d.ID,
		&kindStr,
		&d.Title,
		&d.Content,
		&d.Status,
		&d.CreatorID,
		&d.CreatedAt,
		&d.UpdatedAt,
		&d.PublishedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23503" && pgErr.ConstraintName == "fk_content_items_creator" {
				return nil, fmt.Errorf("creator with ID %s not found: %w", audit.ActorID, err)
			}
			if pgErr.Code == "23514" {
				return nil, &DraftValidationError{Message: fmt.Sprintf("database check constraint failed: %s", pgErr.ConstraintName)}
			}
		}
		return nil, fmt.Errorf("failed to insert draft: %w", err)
	}
	d.Kind = Kind(kindStr)

	// Insert audit record: record actor, target ID, kind, and action.
	// Intentionally omit unpublished title/body from audit log.
	if err := insertDraftAuditLog(ctx, tx, audit.ActorID, "content.draft.create", d.ID, d.Kind, audit); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit draft create transaction: %w", err)
	}

	return &d, nil
}

// GetDraftByID retrieves a single draft by ID. If the item does not exist, ErrDraftNotFound is returned.
// If the item exists but is not in draft status, ErrNotADraft is returned.
func (r *PostgresRepository) GetDraftByID(ctx context.Context, id uuid.UUID) (*Draft, error) {
	if id == uuid.Nil {
		return nil, &DraftValidationError{Message: "draft ID is required"}
	}

	const getDraftSQL = `
		SELECT 
			id,
			kind,
			title,
			content,
			status,
			creator_id,
			created_at,
			updated_at,
			published_at
		FROM giki_sports.content_items
		WHERE id = $1;
	`

	var d Draft
	var kindStr string
	err := r.pool.QueryRow(ctx, getDraftSQL, id).Scan(
		&d.ID,
		&kindStr,
		&d.Title,
		&d.Content,
		&d.Status,
		&d.CreatorID,
		&d.CreatedAt,
		&d.UpdatedAt,
		&d.PublishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &DraftNotFoundError{ID: id}
		}
		return nil, fmt.Errorf("failed to get draft %s: %w", id, err)
	}

	if d.Status != "draft" {
		return nil, &NotADraftError{ID: id, Status: d.Status}
	}

	d.Kind = Kind(kindStr)
	return &d, nil
}

// UpdateDraft updates title and content of an existing draft and writes a content.draft.update audit log
// within a single atomic PostgreSQL transaction.
// Enforces the draft condition in the write itself.
func (r *PostgresRepository) UpdateDraft(
	ctx context.Context,
	id uuid.UUID,
	input UpdateDraftInput,
	audit AuditParams,
) (*Draft, error) {
	if id == uuid.Nil {
		return nil, &DraftValidationError{Message: "draft ID is required"}
	}
	if audit.ActorID == uuid.Nil {
		return nil, &DraftValidationError{Message: "audit actor ID is required"}
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin draft update transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Execute UPDATE enforcing status = 'draft' in the write itself
	const updateDraftSQL = `
		UPDATE giki_sports.content_items
		SET 
			title = $1,
			content = $2,
			updated_at = now()
		WHERE id = $3
		  AND status = 'draft'
		RETURNING 
			id,
			kind,
			title,
			content,
			status,
			creator_id,
			created_at,
			updated_at,
			published_at;
	`

	var d Draft
	var kindStr string
	err = tx.QueryRow(ctx, updateDraftSQL,
		input.Title,
		input.Content,
		id,
	).Scan(
		&d.ID,
		&kindStr,
		&d.Title,
		&d.Content,
		&d.Status,
		&d.CreatorID,
		&d.CreatedAt,
		&d.UpdatedAt,
		&d.PublishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Row was not updated. Distinguish missing vs. non-draft item
			var currentStatus string
			statusErr := tx.QueryRow(ctx, "SELECT status FROM giki_sports.content_items WHERE id = $1;", id).Scan(&currentStatus)
			if statusErr != nil {
				if errors.Is(statusErr, pgx.ErrNoRows) {
					return nil, &DraftNotFoundError{ID: id}
				}
				return nil, fmt.Errorf("failed to inspect status of content item %s: %w", id, statusErr)
			}
			return nil, &NotADraftError{ID: id, Status: currentStatus}
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23514" {
				return nil, &DraftValidationError{Message: fmt.Sprintf("database check constraint failed: %s", pgErr.ConstraintName)}
			}
		}
		return nil, fmt.Errorf("failed to update draft %s: %w", id, err)
	}
	d.Kind = Kind(kindStr)

	// Insert audit record: record actor, target ID, kind, and action.
	// Intentionally omit unpublished title/body from audit log.
	if err := insertDraftAuditLog(ctx, tx, audit.ActorID, "content.draft.update", d.ID, d.Kind, audit); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit draft update transaction: %w", err)
	}

	return &d, nil
}

// PublishDraft transitions an existing draft to published status and records a content.publish
// audit log entry within a single atomic PostgreSQL transaction.
func (r *PostgresRepository) PublishDraft(
	ctx context.Context,
	id uuid.UUID,
	audit AuditParams,
) (*PublishedItem, error) {
	if id == uuid.Nil {
		return nil, &DraftValidationError{Message: "content ID is required"}
	}
	if audit.ActorID == uuid.Nil {
		return nil, &DraftValidationError{Message: "audit actor ID is required"}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin publish transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Step 1: Lock the target row and inspect the current version
	const selectForUpdateSQL = `
		SELECT 
			id,
			kind,
			title,
			content,
			status
		FROM giki_sports.content_items
		WHERE id = $1
		FOR UPDATE;
	`

	var currentID uuid.UUID
	var kindStr string
	var titlePtr, contentPtr *string
	var currentStatus string

	err = tx.QueryRow(ctx, selectForUpdateSQL, id).Scan(
		&currentID,
		&kindStr,
		&titlePtr,
		&contentPtr,
		&currentStatus,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &DraftNotFoundError{ID: id}
		}
		return nil, fmt.Errorf("failed to lock content item %s for publish: %w", id, err)
	}

	// Step 2: Ensure item is a draft (already published items are rejected without new timestamp or audit entry)
	if currentStatus != "draft" {
		return nil, &NotADraftError{ID: id, Status: currentStatus}
	}

	// Step 3: Validate completeness (title and content must be non-nil and non-blank)
	if titlePtr == nil || strings.TrimSpace(*titlePtr) == "" {
		return nil, &IncompleteDraftError{
			ID:     id,
			Reason: "title must be provided and cannot be blank or whitespace-only",
		}
	}
	if contentPtr == nil || strings.TrimSpace(*contentPtr) == "" {
		return nil, &IncompleteDraftError{
			ID:     id,
			Reason: "content must be provided and cannot be blank or whitespace-only",
		}
	}

	// Step 4: Update status to published, assigning published_at and updated_at server-side.
	// Preserves ID, kind, creator, title, and body.
	const updatePublishSQL = `
		UPDATE giki_sports.content_items
		SET 
			status = 'published',
			published_at = now(),
			updated_at = now()
		WHERE id = $1
		  AND status = 'draft'
		RETURNING 
			id,
			kind,
			title,
			content,
			published_at;
	`

	var pubItem PublishedItem
	var pubKindStr string
	err = tx.QueryRow(ctx, updatePublishSQL, id).Scan(
		&pubItem.ID,
		&pubKindStr,
		&pubItem.Title,
		&pubItem.Content,
		&pubItem.PublishedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23514" {
				return nil, &DraftValidationError{
					Message: fmt.Sprintf("database check constraint failed: %s", pgErr.ConstraintName),
				}
			}
		}
		return nil, fmt.Errorf("failed to transition draft %s to published: %w", id, err)
	}
	pubItem.Kind = Kind(pubKindStr)

	// Step 5: Insert one content.publish audit entry in the same transaction
	if err := insertDraftAuditLog(ctx, tx, audit.ActorID, "content.publish", pubItem.ID, pubItem.Kind, audit); err != nil {
		return nil, err
	}

	// Step 6: Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit publish transaction: %w", err)
	}

	return &pubItem, nil
}

func insertDraftAuditLog(
	ctx context.Context,
	tx pgx.Tx,
	actorID uuid.UUID,
	action string,
	targetID uuid.UUID,
	kind Kind,
	audit AuditParams,
) error {
	detailsMap := map[string]interface{}{
		"actor_id":  actorID.String(),
		"target_id": targetID.String(),
		"kind":      string(kind),
		"action":    action,
	}
	detailsJSON, err := json.Marshal(detailsMap)
	if err != nil {
		return fmt.Errorf("failed to marshal audit details: %w", err)
	}

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
			$1, $2, 'content_item', $3, $4, $5, $6, 'SUCCESS', now()
		);
	`

	_, err = tx.Exec(ctx, insertAuditSQL,
		actorID,
		action,
		targetID,
		detailsJSON,
		ip,
		ua,
	)
	if err != nil {
		return fmt.Errorf("failed to insert audit log for action %q: %w", action, err)
	}

	return nil
}
