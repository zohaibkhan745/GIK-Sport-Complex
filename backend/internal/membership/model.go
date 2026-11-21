package membership

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// MembershipStatus defines the lifecycle status of a gym membership.
type MembershipStatus string

const (
	MembershipStatusActive    MembershipStatus = "ACTIVE"
	MembershipStatusExpired   MembershipStatus = "EXPIRED"
	MembershipStatusCancelled MembershipStatus = "CANCELLED"
)

// PlanType defines the subscription tier of a gym membership.
type PlanType string

const (
	PlanTypeStandard30Day PlanType = "STANDARD_30_DAY"
)

// Default gym membership fee in integer paisa (e.g. 300,000 paisa = PKR 3,000.00).
const DefaultGymMembershipFeePaisa = int64(300_000)

// Standard duration for a gym membership pass is exactly 30 days (720 hours).
const PassDuration = 30 * 24 * time.Hour

// GymMembershipPlan represents a configurable gym pass tier with editable fee, duration, and details.
type GymMembershipPlan struct {
	ID           uuid.UUID  `json:"id"`
	PlanCode     string     `json:"planCode"`
	Name         string     `json:"name"`
	DurationDays int        `json:"durationDays"`
	PricePaisa   int64      `json:"pricePaisa"`
	IsActive     bool       `json:"isActive"`
	Description  string     `json:"description"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	UpdatedBy    *uuid.UUID `json:"updatedBy,omitempty"`
}

var (
	ErrMembershipNotFound        = errors.New("membership not found")
	ErrInvalidMembershipDates    = errors.New("invalid membership start or end time: start must precede end")
	ErrInvalidPlanType           = errors.New("invalid membership plan type")
	ErrInsufficientFunds         = errors.New("insufficient wallet balance for membership purchase")
	ErrIdempotencyKeyRequired    = errors.New("idempotency key is required")
	ErrDuplicatePurchase         = errors.New("duplicate membership purchase request")
	ErrDatabase                  = errors.New("membership database error")
	ErrInvalidPlanDuration       = errors.New("plan duration must be greater than zero days")
	ErrNegativePlanFee           = errors.New("plan fee cannot be negative")
	ErrPlanNotFound              = errors.New("gym membership plan not found")
)

// Membership represents a 30-day gym pass belonging to a user.
type Membership struct {
	ID                 uuid.UUID        `json:"id"`
	UserID             uuid.UUID        `json:"userId"`
	PlanType           PlanType         `json:"planType"`
	Status             MembershipStatus `json:"status"`
	StartTime          time.Time        `json:"startTime"`
	EndTime            time.Time        `json:"endTime"`
	PricePaisa         int64            `json:"pricePaisa"`
	DebitTransactionID uuid.UUID        `json:"debitTransactionId"`
	IdempotencyKey     string           `json:"idempotencyKey"`
	CreatedAt          time.Time        `json:"createdAt"`
	UpdatedAt          time.Time        `json:"updatedAt"`
}

// UserMembershipSummary aggregates a user's active membership state with history.
type UserMembershipSummary struct {
	IsActive          bool          `json:"isActive"`
	CurrentMembership *Membership   `json:"currentMembership,omitempty"`
	EffectiveExpiry   *time.Time    `json:"effectiveExpiry,omitempty"`
	DaysRemaining     int           `json:"daysRemaining"`
	SecondsRemaining  int64         `json:"secondsRemaining"`
	History           []*Membership `json:"history"`
}

// Clock abstracts system time to enable deterministic testing of exact-second validity.
type Clock interface {
	Now() time.Time
}

// SystemClock provides real-world UTC time.
type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

// ControlledClock provides a manually manipulated clock for rigorous validity tests.
type ControlledClock struct {
	currentTime time.Time
}

// NewControlledClock instantiates a clock fixed at initialTime.
func NewControlledClock(initialTime time.Time) *ControlledClock {
	return &ControlledClock{currentTime: initialTime.UTC()}
}

func (c *ControlledClock) Now() time.Time {
	return c.currentTime
}

func (c *ControlledClock) Set(t time.Time) {
	c.currentTime = t.UTC()
}

func (c *ControlledClock) Advance(d time.Duration) {
	c.currentTime = c.currentTime.Add(d)
}
