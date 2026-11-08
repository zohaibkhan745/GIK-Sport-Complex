package devuser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"giki-sports-backend/internal/auth"
	"giki-sports-backend/internal/user"
)

// DefaultAllowedDatabase defines the standard sports complex development database name.
const DefaultAllowedDatabase = "giki_sports_complex"

// Preset represents a supported synthetic local development account preset.
type Preset string

const (
	PresetDirector Preset = "director"
	PresetStudent  Preset = "student"
)

// PresetDefinition encapsulates the predefined configuration for a development account.
// All presets use synthetic email addresses on the RFC 2606 reserved '.test' TLD.
type PresetDefinition struct {
	Preset      Preset
	Email       string
	DisplayName string
	Role        user.Role
}

// Presets maps each supported preset name to its immutable definition.
// Arbitrary roles or super_admin are intentionally excluded.
var Presets = map[Preset]PresetDefinition{
	PresetDirector: {
		Preset:      PresetDirector,
		Email:       "director@example.test",
		DisplayName: "Development Sports Director",
		Role:        user.RoleSportsDirector,
	},
	PresetStudent: {
		Preset:      PresetStudent,
		Email:       "student@example.test",
		DisplayName: "Development Student",
		Role:        user.RoleStudent,
	},
}

// GetPreset resolves a preset name into its definition, failing closed on unknown values.
func GetPreset(name string) (PresetDefinition, error) {
	p := Preset(strings.ToLower(strings.TrimSpace(name)))
	def, ok := Presets[p]
	if !ok {
		return PresetDefinition{}, fmt.Errorf("unsupported preset %q: allowed presets are 'director' or 'student'", name)
	}
	return def, nil
}

// ValidateTarget verifies that the database host is a loopback address and
// the database name matches an allowed development database name (default: "giki_sports_complex").
//
// These checks are defensive safeguards against accidental execution against
// remote or production databases, not cryptographic proof of the database's identity.
func ValidateTarget(host, dbName string, allowedDBs ...string) error {
	trimmedHost := strings.TrimSpace(host)
	if trimmedHost == "" {
		return errors.New("database host cannot be empty")
	}

	if !isLoopback(trimmedHost) {
		return fmt.Errorf("target host %q is not a loopback address; dev provisioning is restricted to 127.0.0.1, localhost, or ::1", trimmedHost)
	}

	trimmedDB := strings.TrimSpace(dbName)
	if trimmedDB == "" {
		return errors.New("database name cannot be empty")
	}

	allowed := allowedDBs
	if len(allowed) == 0 {
		allowed = []string{DefaultAllowedDatabase}
	}

	matched := false
	for _, a := range allowed {
		if trimmedDB == a {
			matched = true
			break
		}
	}
	if !matched {
		return fmt.Errorf("target database %q is not permitted; dev provisioning is restricted to: %s", trimmedDB, strings.Join(allowed, ", "))
	}

	return nil
}

func isLoopback(host string) bool {
	lower := strings.ToLower(host)
	if lower == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// ReadPasswordInteractive reads and confirms a password without echo.
// If isTerminal is false, noninteractive input is refused.
func ReadPasswordInteractive(isTerminal bool, readPasswordFn func() ([]byte, error), out io.Writer, email string) (string, error) {
	if !isTerminal {
		return "", errors.New("refusing noninteractive input: provision-dev-user requires an interactive terminal for secure password entry")
	}

	fmt.Fprintf(out, "Enter password for %s: ", email)
	passBytes, err := readPasswordFn()
	fmt.Fprintln(out)
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	fmt.Fprint(out, "Confirm password: ")
	confirmBytes, err := readPasswordFn()
	fmt.Fprintln(out)
	if err != nil {
		return "", fmt.Errorf("failed to read password confirmation: %w", err)
	}

	if string(passBytes) != string(confirmBytes) {
		return "", errors.New("passwords do not match")
	}

	pass := string(passBytes)
	if len(pass) == 0 {
		return "", errors.New("password cannot be empty")
	}
	if len(pass) > auth.MaxPasswordLength {
		return "", fmt.Errorf("password exceeds maximum allowed length of %d bytes", auth.MaxPasswordLength)
	}

	return pass, nil
}

// ProvisionResult summarizes the outcome of a provisioning transaction.
// Passwords and hashes are strictly excluded from this model.
type ProvisionResult struct {
	UserID        uuid.UUID
	Email         string
	DisplayName   string
	Role          user.Role
	AlreadyExists bool
	ExistingRoles []user.Role
	CreatedAt     time.Time
}

// Provision provisions a user account based on a preset in an atomic PostgreSQL transaction.
func Provision(ctx context.Context, pool *pgxpool.Pool, preset Preset, password string) (*ProvisionResult, error) {
	def, err := GetPreset(string(preset))
	if err != nil {
		return nil, err
	}
	return ProvisionUser(ctx, pool, def.Email, def.DisplayName, def.Role, password)
}

// ProvisionUser creates a user and assigns a role within a single atomic PostgreSQL transaction.
//
// Invariants:
// 1. If the email already exists, no changes are made: password, status, and roles are preserved.
// 2. Concurrent attempts are handled safely via the database uniqueness constraint (uq_users_email_lower).
// 3. A failure during role insertion triggers an automatic rollback of the user insert.
// 4. Success is reported only after tx.Commit succeeds.
// 5. Does not create a session.
func ProvisionUser(ctx context.Context, pool *pgxpool.Pool, email, displayName string, role user.Role, password string) (*ProvisionResult, error) {
	if len(password) == 0 {
		return nil, errors.New("password cannot be empty")
	}
	if len(password) > auth.MaxPasswordLength {
		return nil, fmt.Errorf("password exceeds maximum allowed length of %d bytes", auth.MaxPasswordLength)
	}

	normalizedEmail := user.NormalizeEmail(email)
	if normalizedEmail == "" {
		return nil, errors.New("email cannot be empty")
	}

	// Compute Argon2id hash using production default parameters
	passwordHash, err := auth.Hash(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password with Argon2id: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Step 1: Check if user already exists
	var (
		existingID          uuid.UUID
		existingDisplayName string
		existingStatus      string
		existingCreatedAt   time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT id, display_name, status, created_at
		FROM giki_sports.users
		WHERE LOWER(TRIM(email)) = $1
		FOR SHARE;
	`, normalizedEmail).Scan(&existingID, &existingDisplayName, &existingStatus, &existingCreatedAt)

	if err == nil {
		// Existing account found: retrieve existing roles and report without changing anything!
		rows, err := tx.Query(ctx, `
			SELECT role FROM giki_sports.user_roles WHERE user_id = $1 ORDER BY role ASC;
		`, existingID)
		if err != nil {
			return nil, fmt.Errorf("failed to query existing user roles: %w", err)
		}
		defer rows.Close()

		var existingRoles []user.Role
		for rows.Next() {
			var r string
			if err := rows.Scan(&r); err != nil {
				return nil, fmt.Errorf("failed to scan existing user role: %w", err)
			}
			existingRoles = append(existingRoles, user.Role(r))
		}
		if rows.Err() != nil {
			return nil, fmt.Errorf("error reading existing user roles: %w", rows.Err())
		}

		// Rollback explicitly since no mutations were made
		_ = tx.Rollback(ctx)

		return &ProvisionResult{
			UserID:        existingID,
			Email:         normalizedEmail,
			DisplayName:   existingDisplayName,
			Role:          role,
			AlreadyExists: true,
			ExistingRoles: existingRoles,
			CreatedAt:     existingCreatedAt,
		}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to check existing user: %w", err)
	}

	// Step 2: Insert user
	userID := uuid.New()
	var createdAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, $2, $3, $4, 'active')
		RETURNING created_at;
	`, userID, normalizedEmail, displayName, passwordHash).Scan(&createdAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Concurrent duplicate insert attempt caught by uq_users_email_lower
			return &ProvisionResult{
				Email:         normalizedEmail,
				DisplayName:   displayName,
				Role:          role,
				AlreadyExists: true,
			}, nil
		}
		return nil, fmt.Errorf("failed to insert user: %w", err)
	}

	// Step 3: Insert role
	_, err = tx.Exec(ctx, `
		INSERT INTO giki_sports.user_roles (user_id, role)
		VALUES ($1, $2);
	`, userID, string(role))
	if err != nil {
		// If role insert fails, transaction rolls back, undoing the user insert
		return nil, fmt.Errorf("failed to insert user role: %w", err)
	}

	// Step 4: Commit transaction - report success only after commit succeeds
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &ProvisionResult{
		UserID:        userID,
		Email:         normalizedEmail,
		DisplayName:   displayName,
		Role:          role,
		AlreadyExists: false,
		CreatedAt:     createdAt,
	}, nil
}

