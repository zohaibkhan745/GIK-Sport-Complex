package session

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Session timeout and lifetime invariants according to QR-13 and auth-data-model.md.
const (
	// IdleTimeout specifies the maximum duration of inactivity before a session expires (30 minutes).
	IdleTimeout = 30 * time.Minute

	// AbsoluteLifetime specifies the maximum total duration of a session from creation (8 hours).
	AbsoluteLifetime = 8 * time.Hour
)

// Sentinel domain errors returned by session operations.
// Unknown, expired, revoked, and inactive sessions return one recognizable ErrInvalidSession.
// Infrastructure and connection failures remain distinct as standard Go wrapped errors.
var (
	ErrInvalidSession         = errors.New("session: invalid, expired, revoked, or inactive session")
	ErrInvalidTokenHash       = errors.New("session: invalid token hash format")
	ErrUserNotFoundOrInactive = errors.New("session: user not found or account is inactive")
)

// Session represents the public domain read model for an active authenticated session.
//
// CREDENTIAL ISOLATION:
// Session strictly excludes token hashes from its struct fields, JSON tags, and formatters.
// Token hashes are never exposed through this model.
// Roles are NOT cached in Session; roles remain dynamically managed by the user domain.
type Session struct {
	ID                uuid.UUID `json:"id"`
	UserID            uuid.UUID `json:"userId"`
	CreatedAt         time.Time `json:"createdAt"`
	LastActivityAt    time.Time `json:"lastActivityAt"`
	AbsoluteExpiresAt time.Time `json:"absoluteExpiresAt"`
}

// SessionRecord provides full lifetime inspection including nullable RevokedAt.
// Used for audit, lifecycle state verification, and integration testing.
// Excludes token hashes to prevent credential leakage.
type SessionRecord struct {
	ID                uuid.UUID  `json:"id"`
	UserID            uuid.UUID  `json:"userId"`
	CreatedAt         time.Time  `json:"createdAt"`
	LastActivityAt    time.Time  `json:"lastActivityAt"`
	AbsoluteExpiresAt time.Time  `json:"absoluteExpiresAt"`
	RevokedAt         *time.Time `json:"revokedAt,omitempty"`
}

// Clock provides an interface for capturing the current timestamp.
// Enables deterministic, sleep-free testing of temporal boundary conditions.
type Clock interface {
	Now() time.Time
}

// RealClock implements Clock using system wall-clock time in UTC.
type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now().UTC()
}

// TestClock implements Clock with a thread-safe manually controllable timestamp for deterministic testing.
type TestClock struct {
	mu  sync.RWMutex
	now time.Time
}

// NewTestClock initializes a TestClock with the specified initial timestamp in UTC.
func NewTestClock(initial time.Time) *TestClock {
	return &TestClock{now: initial.UTC()}
}

// Now returns the current simulated timestamp.
func (c *TestClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

// Set sets the simulated timestamp to t in UTC.
func (c *TestClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t.UTC()
}

// Advance advances the simulated timestamp by d.
func (c *TestClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

