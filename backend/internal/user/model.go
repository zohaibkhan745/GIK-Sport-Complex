package user

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is a sentinel error returned when a requested user does not exist.
var ErrNotFound = errors.New("user not found")

// NotFoundError provides structured error context when a user lookup by email or ID fails.
type NotFoundError struct {
	Identifier string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("user %q not found", e.Identifier)
}

func (e *NotFoundError) Is(target error) bool {
	return target == ErrNotFound
}

// AccountStatus represents the operational state of a user account.
type AccountStatus string

const (
	StatusActive   AccountStatus = "active"
	StatusInactive AccountStatus = "inactive"
)

// Role represents an authorization role in the sports complex management system.
type Role string

const (
	RoleStudent        Role = "student"
	RoleFaculty        Role = "faculty"
	RoleSportsDirector Role = "sports_director"
	RoleSuperAdmin     Role = "super_admin"
)

// User represents the standard domain read model for an account.
// It includes user identity, normalized email, display name, account status, and assigned roles.
//
// CREDENTIAL SEPARATION:
// User strictly excludes sensitive credential fields (such as PasswordHash) to prevent
// accidental exposure across application layers, logs, or future HTTP profile serialization.
type User struct {
	ID          uuid.UUID     `json:"id"`
	Email       string        `json:"email"`
	DisplayName string        `json:"displayName"`
	Status      AccountStatus `json:"status"`
	Roles       []Role        `json:"roles"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}

// UserCredentials represents a narrowly scoped internal credential read model
// designed exclusively for authentication and credential verification workflows.
//
// CREDENTIAL SAFETY GUARANTEES:
// 1. PasswordHash is tagged with `json:"-"` so it is omitted if serialized to JSON.
// 2. String() and GoString() mask the password hash with "[REDACTED]" to prevent
//    credential leakage in standard fmt logging, stack traces, or debug statements.
type UserCredentials struct {
	User         User
	PasswordHash string `json:"-"`
}

// String implements fmt.Stringer to prevent leaking the password hash during routine formatting.
func (u UserCredentials) String() string {
	return fmt.Sprintf("UserCredentials{ID: %s, Email: %q, PasswordHash: [REDACTED]}", u.User.ID, u.User.Email)
}

// GoString implements fmt.GoStringer to prevent leaking the password hash in %#v printouts.
func (u UserCredentials) GoString() string {
	return fmt.Sprintf("user.UserCredentials{ID: %s, Email: %q, PasswordHash: \"[REDACTED]\"}", u.User.ID, u.User.Email)
}

// NormalizeEmail normalizes an email address by trimming whitespace and lowercasing.
// This matches the database functional unique index: LOWER(TRIM(email)).
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
