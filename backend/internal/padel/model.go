package padel

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SlotStatus represents the current state of a physical padel court time window.
type SlotStatus string

const (
	SlotStatusAvailable SlotStatus = "AVAILABLE"
	SlotStatusHeld      SlotStatus = "HELD"
	SlotStatusBooked    SlotStatus = "BOOKED"
	SlotStatusBlocked   SlotStatus = "BLOCKED"
)

// BookingStatus represents the contractual reservation state of a padel court booking.
type BookingStatus string

const (
	BookingStatusConfirmed BookingStatus = "CONFIRMED"
	BookingStatusCheckedIn BookingStatus = "CHECKED_IN"
	BookingStatusCancelled BookingStatus = "CANCELLED"
)

var (
	ErrInvalidStatus         = errors.New("invalid slot status")
	ErrInvalidTimeRange      = errors.New("start time must be strictly before end time")
	ErrNegativePrice         = errors.New("slot price cannot be negative")
	ErrInconsistentHold      = errors.New("held slots require both holder ID and hold expiration timestamp")
	ErrSlotNotFound          = errors.New("slot not found")
	ErrSlotBooked            = errors.New("cannot modify a slot that is already booked")
	ErrSlotConflict          = errors.New("slot is currently held by another user")
	ErrSlotUnavailable       = errors.New("slot is not available for booking")
	ErrHoldNotOwned          = errors.New("cannot release a hold owned by another user")
	ErrHoldExpired           = errors.New("hold on slot has expired")
	ErrNotSlotHolder         = errors.New("user does not have an active hold on this slot")
	ErrIdempotencyConflict   = errors.New("idempotency key reused with mismatched parameters")
	ErrBlankIdempotencyKey   = errors.New("idempotency key cannot be blank")
	ErrFacilityNotFound      = errors.New("facility not found or inactive")
	ErrInvalidDate           = errors.New("invalid date format, expected YYYY-MM-DD")
	ErrInvalidSlotDuration   = errors.New("slot duration must be positive and evenly divide the operating window")
	ErrInvalidOperatingHours = errors.New("start hour must be before end hour")
	ErrBookingNotFound        = errors.New("booking not found")
	ErrInvalidCheckInCode     = errors.New("invalid or empty check-in code")
	ErrTicketAlreadyCheckedIn       = errors.New("ticket has already been checked in")
	ErrBookingCancelled             = errors.New("cannot check in a cancelled booking")
	ErrBookingAlreadyCancelled      = errors.New("booking is already cancelled")
	ErrCannotCancelCheckedInBooking = errors.New("cannot cancel a booking that has already been checked in")
	ErrUserCancellationForbidden    = errors.New("user self-cancellation is not permitted per sports complex policy")
)

// DefaultHoldDuration defines the strict 5-minute temporary reservation window (FR-03).
const DefaultHoldDuration = 5 * time.Minute

// IsValidSlotStatus checks if a given string corresponds to a valid SlotStatus.
func IsValidSlotStatus(s string) bool {
	switch SlotStatus(strings.ToUpper(strings.TrimSpace(s))) {
	case SlotStatusAvailable, SlotStatusHeld, SlotStatusBooked, SlotStatusBlocked:
		return true
	default:
		return false
	}
}

// Slot represents a discrete time inventory window for a Padel court.
type Slot struct {
	ID            uuid.UUID  `json:"id"`
	FacilityID    uuid.UUID  `json:"facilityId"`
	StartTime     time.Time  `json:"startTime"`
	EndTime       time.Time  `json:"endTime"`
	Status        SlotStatus `json:"status"`
	HolderID      *uuid.UUID `json:"holderId,omitempty"`
	HoldExpiresAt *time.Time `json:"holdExpiresAt,omitempty"`
	PricePaisa    int64      `json:"pricePaisa"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// IsHoldExpired evaluates whether an existing hold on this slot has expired relative to the given time.
func (s *Slot) IsHoldExpired(now time.Time) bool {
	if s.Status != SlotStatusHeld {
		return false
	}
	if s.HoldExpiresAt == nil {
		return true
	}
	return !now.Before(*s.HoldExpiresAt)
}

// EffectiveStatus returns the real-time status of the slot.
// If the slot is currently marked HELD but its hold expiration timestamp is in the past,
// it is dynamically evaluated as AVAILABLE (Lazy Hold Expiration).
func (s *Slot) EffectiveStatus(now time.Time) SlotStatus {
	if s.Status == SlotStatusHeld && s.IsHoldExpired(now) {
		return SlotStatusAvailable
	}
	return s.Status
}

// Validate ensures all structural and domain invariants of the Slot entity are satisfied.
func (s *Slot) Validate() error {
	if s.FacilityID == uuid.Nil {
		return errors.New("facility ID is required")
	}
	if !s.StartTime.Before(s.EndTime) {
		return ErrInvalidTimeRange
	}
	if s.PricePaisa < 0 {
		return ErrNegativePrice
	}
	if !IsValidSlotStatus(string(s.Status)) {
		return ErrInvalidStatus
	}
	if s.Status == SlotStatusHeld {
		if s.HolderID == nil || s.HoldExpiresAt == nil {
			return ErrInconsistentHold
		}
	} else {
		if s.HolderID != nil || s.HoldExpiresAt != nil {
			return ErrInconsistentHold
		}
	}
	return nil
}

// Booking represents a confirmed, funded court reservation contract (FR-03, QR-01, QR-03, QR-04).
type Booking struct {
	ID                 uuid.UUID     `json:"id"`
	SlotID             uuid.UUID     `json:"slotId"`
	UserID             uuid.UUID     `json:"userId"`
	Status             BookingStatus `json:"status"`
	FeePaisa           int64         `json:"feePaisa"`
	IdempotencyKey     string        `json:"idempotencyKey"`
	CheckInCode        string        `json:"checkInCode"`
	DebitTransactionID *uuid.UUID    `json:"debitTransactionId,omitempty"`
	RefundTransactionID *uuid.UUID   `json:"refundTransactionId,omitempty"`
	CancelledAt        *time.Time    `json:"cancelledAt,omitempty"`
	CancelledBy        *uuid.UUID    `json:"cancelledBy,omitempty"`
	CancellationReason *string       `json:"cancellationReason,omitempty"`
	CheckedInAt        *time.Time    `json:"checkedInAt,omitempty"`
	CheckedInBy        *uuid.UUID    `json:"checkedInBy,omitempty"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}
