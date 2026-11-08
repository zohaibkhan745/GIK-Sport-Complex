package authentication

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"giki-sports-backend/internal/auth"
	"giki-sports-backend/internal/session"
	"giki-sports-backend/internal/user"
)

// Sentinel domain errors for authentication orchestration.
var (
	// ErrInvalidCredentials indicates that the supplied email/password pair was not accepted.
	// Returned uniformly for unknown users, incorrect passwords, and inactive user accounts
	// to prevent observable account status enumeration.
	ErrInvalidCredentials = errors.New("authentication: invalid email or password")

	// ErrInvalidSession indicates that the supplied session token is malformed, unrecognized,
	// expired, revoked, or belongs to a missing or inactive user.
	ErrInvalidSession = errors.New("authentication: invalid, expired, or revoked session")

	// ErrInternal indicates an unexpected internal failure, such as data corruption or malformed stored hashes.
	ErrInternal = errors.New("authentication: internal error")
)

// UserCredentialRepository defines the consumer-owned user lookup operations needed by the authentication service.
type UserCredentialRepository interface {
	// GetCredentialsByEmail retrieves the credential-bearing model (including PasswordHash) by email.
	// Returns user.ErrNotFound if the user does not exist.
	GetCredentialsByEmail(ctx context.Context, email string) (*user.UserCredentials, error)

	// GetByID retrieves a user domain model and their assigned roles by user ID.
	// Returns user.ErrNotFound if the user does not exist.
	GetByID(ctx context.Context, id uuid.UUID) (*user.User, error)
}

// SessionStore defines the consumer-owned session repository operations needed by the authentication service.
type SessionStore interface {
	// Create persists a new session row with a token hash for an active user.
	// Returns session.ErrUserNotFoundOrInactive if the user does not exist or status != 'active'.
	Create(ctx context.Context, userID uuid.UUID, tokenHash string) (*session.Session, error)

	// ValidateAndTouch performs atomic production validation and activity update under a session row lock.
	// Returns session.ErrInvalidSession if expired, revoked, inactive, or not found.
	ValidateAndTouch(ctx context.Context, tokenHash string) (*session.Session, error)

	// Revoke sets revoked_at idempotently.
	// Returns session.ErrInvalidSession if the session does not exist.
	Revoke(ctx context.Context, tokenHash string) error
}

// LoginResult represents the sensitive return value of a successful Login call.
// It contains the plaintext raw session token strictly intended for Set-Cookie delivery.
//
// CREDENTIAL ISOLATION:
// RawToken is tagged with `json:"-"` and masked as [REDACTED] in String() and GoString()
// to prevent credential leakage in serialization, log formatters, and stack dumps.
type LoginResult struct {
	RawToken string           `json:"-"`
	User     *user.User       `json:"user"`
	Session  *session.Session `json:"session"`
}

// String implements fmt.Stringer to ensure RawToken is never printed in logs or debug dumps.
func (r LoginResult) String() string {
	userID := "nil"
	if r.User != nil {
		userID = r.User.ID.String()
	}
	return fmt.Sprintf("LoginResult{UserID: %s, RawToken: [REDACTED]}", userID)
}

// GoString implements fmt.GoStringer for %#v formatting.
func (r LoginResult) GoString() string {
	return r.String()
}

// AuthenticatedPrincipal represents a validated active user session.
// Strictly excludes credentials, password hashes, and session tokens.
type AuthenticatedPrincipal struct {
	User    *user.User       `json:"user"`
	Session *session.Session `json:"session"`
}

// Service orchestrates authentication workflows: login, session validation, and logout.
type Service struct {
	userRepo    UserCredentialRepository
	sessionRepo SessionStore
	dummyHash   string

	tokenGen func() (string, string, error)
	verifyFn func(password, encodedHash string) (bool, error)
}

// NewService constructs an authentication Service backed by user and session repositories.
// Precomputes a production-parameter dummy Argon2id hash once during initialization to
// mitigate fast-path account enumeration on unknown and inactive user paths.
func NewService(userRepo UserCredentialRepository, sessionRepo SessionStore) (*Service, error) {
	dummyHash, err := auth.Hash("sports-complex-dummy-timing-mitigation-salt-hash")
	if err != nil {
		return nil, fmt.Errorf("authentication: failed to precompute dummy password hash: %w", err)
	}

	return &Service{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		dummyHash:   dummyHash,
		tokenGen:    auth.GenerateSessionToken,
		verifyFn:    auth.Verify,
	}, nil
}

// NewServiceWithParams constructs a Service with custom token generation and verification functions for testing.
func NewServiceWithParams(
	userRepo UserCredentialRepository,
	sessionRepo SessionStore,
	dummyHash string,
	tokenGen func() (string, string, error),
	verifyFn func(password, encodedHash string) (bool, error),
) *Service {
	if dummyHash == "" {
		dummyHash, _ = auth.Hash("sports-complex-dummy-timing-mitigation-salt-hash")
	}
	if tokenGen == nil {
		tokenGen = auth.GenerateSessionToken
	}
	if verifyFn == nil {
		verifyFn = auth.Verify
	}
	return &Service{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		dummyHash:   dummyHash,
		tokenGen:    tokenGen,
		verifyFn:    verifyFn,
	}
}

// Login authenticates a user by email and password, creates an active session, and returns
// a LoginResult containing the raw session token for cookie issuance.
//
// Timing & Enumeration Defense:
// Unknown users, wrong passwords, and inactive accounts are rejected with the identical
// ErrInvalidCredentials error. When an email is not found or is inactive, the service
// executes a dummy Argon2id verification using a precomputed hash to mitigate obvious
// timing discrepancies.
func (s *Service) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	if strings.TrimSpace(email) == "" || password == "" {
		_, _ = s.verifyFn(password, s.dummyHash)
		return nil, ErrInvalidCredentials
	}

	normalizedEmail := user.NormalizeEmail(email)
	creds, err := s.userRepo.GetCredentialsByEmail(ctx, normalizedEmail)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			// Unknown user: execute dummy verification to simulate password verification work
			_, _ = s.verifyFn(password, s.dummyHash)
			return nil, ErrInvalidCredentials
		}
		// Genuine database or context failure
		return nil, fmt.Errorf("authentication: credential lookup failed: %w", err)
	}

	// Inactive user: execute dummy verification to simulate work and prevent fast-path enumeration
	if creds.User.Status != user.StatusActive {
		_, _ = s.verifyFn(password, s.dummyHash)
		return nil, ErrInvalidCredentials
	}

	// Verify password against stored Argon2id hash
	matched, err := s.verifyFn(password, creds.PasswordHash)
	if err != nil {
		// Malformed stored hash format: treat as internal integrity error
		return nil, fmt.Errorf("%w: stored password hash is invalid: %v", ErrInternal, err)
	}
	if !matched {
		return nil, ErrInvalidCredentials
	}

	// Generate high-entropy opaque session token and SHA-256 hash ONLY after credentials pass
	rawToken, tokenHash, err := s.tokenGen()
	if err != nil {
		return nil, fmt.Errorf("authentication: token generation failed: %w", err)
	}

	// Persist session in repository
	sess, err := s.sessionRepo.Create(ctx, creds.User.ID, tokenHash)
	if err != nil {
		// If the account transitioned to inactive concurrently before session creation, reject login
		if errors.Is(err, session.ErrUserNotFoundOrInactive) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("authentication: session creation failed: %w", err)
	}

	return &LoginResult{
		RawToken: rawToken,
		User:     &creds.User,
		Session:  sess,
	}, nil
}

// ValidateSession validates an opaque session token, touches its activity timestamp,
// and returns the authenticated user principal.
//
// Concurrency & Authorization Boundaries:
// Uses production session.ValidateAndTouch to atomically lock the session row, verify dual
// timeouts (30-minute idle, 8-hour absolute ceiling), verify revocation, and update activity.
// Loads current user roles via GetByID. Note that separate user reads do not promise that
// later role or account modifications will retroactively cancel an already authorized request.
func (s *Service) ValidateSession(ctx context.Context, rawToken string) (*AuthenticatedPrincipal, error) {
	if err := auth.ValidateSessionToken(rawToken); err != nil {
		return nil, ErrInvalidSession
	}

	tokenHash, err := auth.HashSessionToken(rawToken)
	if err != nil {
		return nil, ErrInvalidSession
	}

	// Call the production ValidateAndTouch path
	sess, err := s.sessionRepo.ValidateAndTouch(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, session.ErrInvalidSession) {
			return nil, ErrInvalidSession
		}
		return nil, fmt.Errorf("authentication: session validation failed: %w", err)
	}

	// Load current user profile and assigned roles
	u, err := s.userRepo.GetByID(ctx, sess.UserID)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrInvalidSession
		}
		return nil, fmt.Errorf("authentication: user profile lookup failed: %w", err)
	}

	if u.Status != user.StatusActive {
		return nil, ErrInvalidSession
	}

	return &AuthenticatedPrincipal{
		User:    u,
		Session: sess,
	}, nil
}

// Logout revokes a session by its raw token.
//
// Idempotence & Coordination:
// Hashes the token and revokes it via the session repository without calling ValidateAndTouch,
// ensuring that expired or revoked sessions can still be revoked idempotently.
// Malformed, unrecognized, or already revoked tokens are treated as harmless repeated logouts.
// Genuine database or context errors are propagated.
func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if err := auth.ValidateSessionToken(rawToken); err != nil {
		// Malformed token format is treated as harmless repeated logout
		return nil
	}

	tokenHash, err := auth.HashSessionToken(rawToken)
	if err != nil {
		return nil
	}

	err = s.sessionRepo.Revoke(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, session.ErrInvalidSession) {
			// Unknown, already revoked, or missing session is harmless idempotent logout
			return nil
		}
		return fmt.Errorf("authentication: failed to revoke session: %w", err)
	}

	return nil
}
