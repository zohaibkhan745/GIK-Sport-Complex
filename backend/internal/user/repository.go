package user

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines read-only operations for users and their assigned roles.
type Repository interface {
	// GetByEmail retrieves an active or inactive user and their assigned roles by email.
	// Email is normalized (trimmed and lowercased) matching uq_users_email_lower.
	// Returns ErrNotFound if the user does not exist.
	GetByEmail(ctx context.Context, email string) (*User, error)

	// GetByID retrieves an active or inactive user and their assigned roles by UUID.
	// Returns ErrNotFound if the user does not exist.
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)

	// GetCredentialsByEmail retrieves the credential-bearing model (including PasswordHash)
	// strictly for authentication and login verification.
	// Returns ErrNotFound if the user does not exist.
	GetCredentialsByEmail(ctx context.Context, email string) (*UserCredentials, error)
}

// PostgresRepository implements Repository using a PostgreSQL connection pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new PostgresRepository instance backed by the provided pgxpool.Pool.
func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// GetByEmail retrieves a user and their assigned roles by email.
//
// Read-Consistency & Query Strategy:
// Uses a single parameterized query with LEFT JOIN under a statement-level snapshot.
// This guarantees read consistency between the user profile and their assigned roles,
// avoiding race conditions or multiple database round-trips.
//
// Roles are returned in deterministic alphabetical order (ORDER BY r.role ASC).
// Users with zero roles return an empty, non-nil slice ([]Role{}).
func (r *PostgresRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	normalized := NormalizeEmail(email)
	if normalized == "" {
		return nil, &NotFoundError{Identifier: email}
	}

	const getUserByEmailSQL = `
		SELECT 
			u.id, 
			u.email, 
			u.display_name, 
			u.status, 
			u.created_at, 
			u.updated_at, 
			r.role
		FROM giki_sports.users u
		LEFT JOIN giki_sports.user_roles r ON u.id = r.user_id
		WHERE LOWER(TRIM(u.email)) = $1
		ORDER BY r.role ASC;
	`

	rows, err := r.pool.Query(ctx, getUserByEmailSQL, normalized)
	if err != nil {
		return nil, fmt.Errorf("failed to query user by email: %w", err)
	}
	defer rows.Close()

	var u *User
	for rows.Next() {
		var (
			id          uuid.UUID
			rowEmail    string
			displayName string
			statusStr   string
			createdAt   time.Time
			updatedAt   time.Time
			roleStr     *string
		)

		if err := rows.Scan(&id, &rowEmail, &displayName, &statusStr, &createdAt, &updatedAt, &roleStr); err != nil {
			return nil, fmt.Errorf("failed to scan user row: %w", err)
		}

		if u == nil {
			u = &User{
				ID:          id,
				Email:       rowEmail,
				DisplayName: displayName,
				Status:      AccountStatus(statusStr),
				Roles:       make([]Role, 0),
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
			}
		}

		if roleStr != nil && *roleStr != "" {
			u.Roles = append(u.Roles, Role(*roleStr))
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("user rows iteration error: %w", err)
	}

	if u == nil {
		return nil, &NotFoundError{Identifier: email}
	}

	return u, nil
}

// GetByID retrieves a user and their assigned roles by user UUID.
//
// Read-Consistency & Query Strategy:
// Uses a single parameterized query with LEFT JOIN under a statement-level snapshot.
// Roles are returned in deterministic alphabetical order (ORDER BY r.role ASC).
func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	if id == uuid.Nil {
		return nil, &NotFoundError{Identifier: id.String()}
	}

	const getUserByIDSQL = `
		SELECT 
			u.id, 
			u.email, 
			u.display_name, 
			u.status, 
			u.created_at, 
			u.updated_at, 
			r.role
		FROM giki_sports.users u
		LEFT JOIN giki_sports.user_roles r ON u.id = r.user_id
		WHERE u.id = $1
		ORDER BY r.role ASC;
	`

	rows, err := r.pool.Query(ctx, getUserByIDSQL, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query user by id: %w", err)
	}
	defer rows.Close()

	var u *User
	for rows.Next() {
		var (
			rowID       uuid.UUID
			rowEmail    string
			displayName string
			statusStr   string
			createdAt   time.Time
			updatedAt   time.Time
			roleStr     *string
		)

		if err := rows.Scan(&rowID, &rowEmail, &displayName, &statusStr, &createdAt, &updatedAt, &roleStr); err != nil {
			return nil, fmt.Errorf("failed to scan user row: %w", err)
		}

		if u == nil {
			u = &User{
				ID:          rowID,
				Email:       rowEmail,
				DisplayName: displayName,
				Status:      AccountStatus(statusStr),
				Roles:       make([]Role, 0),
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
			}
		}

		if roleStr != nil && *roleStr != "" {
			u.Roles = append(u.Roles, Role(*roleStr))
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("user rows iteration error: %w", err)
	}

	if u == nil {
		return nil, &NotFoundError{Identifier: id.String()}
	}

	return u, nil
}

// GetCredentialsByEmail retrieves the internal credential-bearing model strictly for
// login and password verification. Ordinary profile requests must use GetByEmail instead.
//
// Read-Consistency & Query Strategy:
// Single parameterized query with LEFT JOIN. PasswordHash is loaded strictly inside this method.
func (r *PostgresRepository) GetCredentialsByEmail(ctx context.Context, email string) (*UserCredentials, error) {
	normalized := NormalizeEmail(email)
	if normalized == "" {
		return nil, &NotFoundError{Identifier: email}
	}

	const getCredentialsByEmailSQL = `
		SELECT 
			u.id, 
			u.email, 
			u.display_name, 
			u.password_hash, 
			u.status, 
			u.created_at, 
			u.updated_at, 
			r.role
		FROM giki_sports.users u
		LEFT JOIN giki_sports.user_roles r ON u.id = r.user_id
		WHERE LOWER(TRIM(u.email)) = $1
		ORDER BY r.role ASC;
	`

	rows, err := r.pool.Query(ctx, getCredentialsByEmailSQL, normalized)
	if err != nil {
		return nil, fmt.Errorf("failed to query user credentials: %w", err)
	}
	defer rows.Close()

	var (
		u            *User
		passwordHash string
	)

	for rows.Next() {
		var (
			id          uuid.UUID
			rowEmail    string
			displayName string
			hashStr     string
			statusStr   string
			createdAt   time.Time
			updatedAt   time.Time
			roleStr     *string
		)

		if err := rows.Scan(&id, &rowEmail, &displayName, &hashStr, &statusStr, &createdAt, &updatedAt, &roleStr); err != nil {
			return nil, fmt.Errorf("failed to scan user credentials row: %w", err)
		}

		if u == nil {
			passwordHash = hashStr
			u = &User{
				ID:          id,
				Email:       rowEmail,
				DisplayName: displayName,
				Status:      AccountStatus(statusStr),
				Roles:       make([]Role, 0),
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
			}
		}

		if roleStr != nil && *roleStr != "" {
			u.Roles = append(u.Roles, Role(*roleStr))
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("user credentials rows iteration error: %w", err)
	}

	if u == nil {
		return nil, &NotFoundError{Identifier: email}
	}

	return &UserCredentials{
		User:         *u,
		PasswordHash: passwordHash,
	}, nil
}
