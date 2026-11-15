package padel

// SlotDTO represents the public and administrative API view of a padel court slot.
// Monetary amounts (PricePaisa) are serialized as decimal strings to protect JavaScript clients
// from floating-point distortion and safe integer overflow (2^53 - 1).
type SlotDTO struct {
	ID            string  `json:"id"`
	FacilityID    string  `json:"facilityId"`
	StartTime     string  `json:"startTime"`
	EndTime       string  `json:"endTime"`
	Status        string  `json:"status"`
	PricePaisa    string  `json:"pricePaisa"`
	IsHeldByMe    bool    `json:"isHeldByMe"`
	HoldExpiresAt *string `json:"holdExpiresAt,omitempty"`
}

// FacilitySummaryDTO represents summary metadata of a facility capable of hosting Padel games.
type FacilitySummaryDTO struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// SlotsResponse represents the collection of slots for a facility on a specific business date.
type SlotsResponse struct {
	Date       string               `json:"date"`
	FacilityID string               `json:"facilityId"`
	Facility   *FacilitySummaryDTO  `json:"facility,omitempty"`
	Slots      []SlotDTO            `json:"slots"`
}

// GenerateSlotsRequest provides parameters for bulk daily slot generation by sports administrators.
type GenerateSlotsRequest struct {
	FacilityID          string `json:"facilityId"`
	Date                string `json:"date"` // Format: YYYY-MM-DD
	StartHour           int    `json:"startHour"` // Default: 8 (08:00 PKT)
	EndHour             int    `json:"endHour"`   // Default: 23 (23:00 PKT)
	SlotDurationMinutes int    `json:"slotDurationMinutes"` // Default: 60 minutes
	PricePaisa          string `json:"pricePaisa"` // Paisa decimal string, e.g. "100000" (PKR 1,000.00)
}

// GenerateSlotsResponse reports the outcome of a batch slot generation execution.
type GenerateSlotsResponse struct {
	Date         string    `json:"date"`
	FacilityID   string    `json:"facilityId"`
	SlotsCreated int       `json:"slotsCreated"`
	SlotsSkipped int       `json:"slotsSkipped"`
	Slots        []SlotDTO `json:"slots"`
}

// UpdateSlotRequest defines administrative modifications to a slot's price or status.
type UpdateSlotRequest struct {
	PricePaisa *string `json:"pricePaisa,omitempty"`
	Status     *string `json:"status,omitempty"`
}

// HoldSlotResponse details the acquired 5-minute hold on a padel slot.
type HoldSlotResponse struct {
	SlotID           string `json:"slotId"`
	FacilityID       string `json:"facilityId"`
	StartTime        string `json:"startTime"`
	EndTime          string `json:"endTime"`
	Status           string `json:"status"` // "HELD"
	PricePaisa       string `json:"pricePaisa"`
	HolderID         string `json:"holderId"`
	HoldExpiresAt    string `json:"holdExpiresAt"` // RFC3339 UTC timestamp
	RemainingSeconds int    `json:"remainingSeconds"`
}

// ReleaseHoldResponse confirms the voluntary release of an active hold.
type ReleaseHoldResponse struct {
	SlotID  string `json:"slotId"`
	Status  string `json:"status"` // "AVAILABLE"
	Message string `json:"message"`
}

// CheckoutRequest provides the slot and idempotency key for confirming a padel booking (FR-03, QR-04).
type CheckoutRequest struct {
	SlotID         string `json:"slotId"`
	IdempotencyKey string `json:"idempotencyKey"`
}

// BookingConfirmationDTO represents the confirmed booking response returned upon successful checkout.
type BookingConfirmationDTO struct {
	BookingID          string `json:"bookingId"`
	SlotID             string `json:"slotId"`
	FacilityID         string `json:"facilityId"`
	FacilityName       string `json:"facilityName,omitempty"`
	StartTime          string `json:"startTime"`
	EndTime            string `json:"endTime"`
	Status             string `json:"status"` // "CONFIRMED"
	FeePaisa           string `json:"feePaisa"`
	CheckInCode        string `json:"checkInCode"`
	DebitTransactionID string `json:"debitTransactionId"`
	CreatedAt          string `json:"createdAt"`
	AlreadyBooked      bool   `json:"alreadyBooked"`
}

// VerifyTicketRequest specifies parameters for attendant ticket check-in verification (FR-03, TC-FR-03-03).
type VerifyTicketRequest struct {
	CheckInCode string `json:"checkInCode"`
	BookingID   string `json:"bookingId,omitempty"`
}

// VerifyTicketResponse details the verified and checked-in booking contract.
type VerifyTicketResponse struct {
	BookingID    string `json:"bookingId"`
	SlotID       string `json:"slotId"`
	FacilityID   string `json:"facilityId"`
	FacilityName string `json:"facilityName,omitempty"`
	UserID       string `json:"userId"`
	UserEmail    string `json:"userEmail,omitempty"`
	UserName     string `json:"userName,omitempty"`
	StartTime    string `json:"startTime"`
	EndTime      string `json:"endTime"`
	Status       string `json:"status"` // "CHECKED_IN"
	FeePaisa     string `json:"feePaisa"`
	CheckInCode  string `json:"checkInCode"`
	CheckedInAt  string `json:"checkedInAt"`
	CheckedInBy  string `json:"checkedInBy"`
}

// CancelBookingRequest specifies optional administrative rationale for cancelling a confirmed booking (FR-06).
type CancelBookingRequest struct {
	Reason string `json:"reason"`
}

// CancelBookingResponse details the cancelled reservation contract and its 100% wallet ledger refund.
type CancelBookingResponse struct {
	BookingID           string `json:"bookingId"`
	SlotID              string `json:"slotId"`
	UserID              string `json:"userId"`
	Status              string `json:"status"` // "CANCELLED"
	FeePaisa            string `json:"feePaisa"`
	RefundTransactionID string `json:"refundTransactionId"`
	CancelledAt         string `json:"cancelledAt"`
	CancelledBy         string `json:"cancelledBy"`
	Reason              string `json:"reason,omitempty"`
	Message             string `json:"message"`
}

// ErrorResponse represents a standardized JSON error message.
type ErrorResponse struct {
	Error string `json:"error"`
}
