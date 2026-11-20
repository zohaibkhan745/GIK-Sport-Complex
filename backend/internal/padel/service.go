package padel

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"giki-sports-backend/internal/wallet"
)

// DefaultPakistanLocation returns Asia/Karachi timezone, with a robust UTC+5 fallback for environments without tzdata.
func DefaultPakistanLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Karachi")
	if err == nil && loc != nil {
		return loc
	}
	return time.FixedZone("PKT", 5*60*60)
}

// TimeProvider defines an interface for obtaining the current time, enabling deterministic testing.
type TimeProvider interface {
	Now() time.Time
}

type realTimeProvider struct{}

func (realTimeProvider) Now() time.Time {
	return time.Now().UTC()
}

// Service provides high-level business operations for Padel court inventory, scheduling, and querying.
type Service struct {
	repo      Repository
	walletSvc *wallet.Service
	uow       UnitOfWork
	clock     TimeProvider
	loc       *time.Location
}

// NewService constructs a Service with the provided repository, wallet service, unit of work, and default timezone.
func NewService(repo Repository, walletSvc *wallet.Service, uow UnitOfWork) *Service {
	return &Service{
		repo:      repo,
		walletSvc: walletSvc,
		uow:       uow,
		clock:     realTimeProvider{},
		loc:       DefaultPakistanLocation(),
	}
}

// NewServiceWithClock constructs a Service with a custom TimeProvider for deterministic testing.
func NewServiceWithClock(repo Repository, walletSvc *wallet.Service, uow UnitOfWork, clock TimeProvider, loc *time.Location) *Service {
	if loc == nil {
		loc = DefaultPakistanLocation()
	}
	return &Service{
		repo:      repo,
		walletSvc: walletSvc,
		uow:       uow,
		clock:     clock,
		loc:       loc,
	}
}

// GetPadelFacilities returns all active facilities eligible for Padel reservations.
func (s *Service) GetPadelFacilities(ctx context.Context) ([]FacilitySummaryDTO, error) {
	return s.repo.GetPadelFacilities(ctx)
}

// GetDefaultFacilityID returns the UUID of the preferred Padel facility, or ErrFacilityNotFound if none exist.
func (s *Service) GetDefaultFacilityID(ctx context.Context) (uuid.UUID, error) {
	facilities, err := s.repo.GetPadelFacilities(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	if len(facilities) == 0 {
		return uuid.Nil, ErrFacilityNotFound
	}
	parsedID, err := uuid.Parse(facilities[0].ID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid facility UUID in repository: %w", err)
	}
	return parsedID, nil
}

// GetSlotsForDate retrieves all slots for a facility on a specific calendar day in Pakistan Standard Time.
// Evaluates lazy hold expiration so that expired holds immediately show as AVAILABLE to clients.
func (s *Service) GetSlotsForDate(ctx context.Context, facilityID uuid.UUID, dateStr string, viewerUserID *uuid.UUID) (*SlotsResponse, error) {
	parsedDate, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(dateStr), s.loc)
	if err != nil {
		return nil, ErrInvalidDate
	}

	facility, err := s.repo.GetFacilityByID(ctx, facilityID)
	if err != nil {
		return nil, err
	}

	// Calculate bounds for the full 24-hour day in PKT: [00:00:00 PKT, next day 00:00:00 PKT)
	startOfDayPKT := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, s.loc)
	endOfDayPKT := startOfDayPKT.Add(24 * time.Hour)

	startOfDayUTC := startOfDayPKT.UTC()
	endOfDayUTC := endOfDayPKT.UTC()

	slots, err := s.repo.FindSlots(ctx, facilityID, startOfDayUTC, endOfDayUTC)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	slotDTOs := make([]SlotDTO, 0, len(slots))

	for _, slot := range slots {
		effectiveStatus := slot.EffectiveStatus(now)

		isHeldByMe := false
		var holdExpiresAtStr *string

		if slot.Status == SlotStatusHeld && !slot.IsHoldExpired(now) {
			if viewerUserID != nil && slot.HolderID != nil && *slot.HolderID == *viewerUserID {
				isHeldByMe = true
			}
			if slot.HoldExpiresAt != nil {
				formatted := slot.HoldExpiresAt.UTC().Format(time.RFC3339)
				holdExpiresAtStr = &formatted
			}
		}

		slotDTOs = append(slotDTOs, SlotDTO{
			ID:            slot.ID.String(),
			FacilityID:    slot.FacilityID.String(),
			StartTime:     slot.StartTime.UTC().Format(time.RFC3339),
			EndTime:       slot.EndTime.UTC().Format(time.RFC3339),
			Status:        string(effectiveStatus),
			PricePaisa:    strconv.FormatInt(slot.PricePaisa, 10),
			IsHeldByMe:    isHeldByMe,
			HoldExpiresAt: holdExpiresAtStr,
		})
	}

	return &SlotsResponse{
		Date:       dateStr,
		FacilityID: facilityID.String(),
		Facility:   facility,
		Slots:      slotDTOs,
	}, nil
}

// GenerateDailySlots creates discrete time inventory windows for a specific date and operating window.
// Default configuration: 08:00 to 23:00 PKT, 60-minute duration, 100,000 paisa (PKR 1,000.00).
func (s *Service) GenerateDailySlots(ctx context.Context, req GenerateSlotsRequest) (*GenerateSlotsResponse, error) {
	facUUID, err := uuid.Parse(strings.TrimSpace(req.FacilityID))
	if err != nil || facUUID == uuid.Nil {
		return nil, errors.New("invalid or missing facility ID")
	}

	fac, err := s.repo.GetFacilityByID(ctx, facUUID)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(strings.ToLower(fac.Slug), "padel") && !strings.Contains(strings.ToLower(fac.Name), "padel") {
		return nil, errors.New("court booking is strictly limited to padel court facilities")
	}

	dateStr := strings.TrimSpace(req.Date)
	parsedDate, err := time.ParseInLocation("2006-01-02", dateStr, s.loc)
	if err != nil {
		return nil, ErrInvalidDate
	}

	startHour := req.StartHour
	if startHour <= 0 {
		startHour = 8 // Default 08:00 PKT
	}
	endHour := req.EndHour
	if endHour <= 0 {
		endHour = 23 // Default 23:00 PKT
	}

	if startHour < 0 || startHour >= 24 || endHour <= 0 || endHour > 24 || startHour >= endHour {
		return nil, ErrInvalidOperatingHours
	}

	durationMin := req.SlotDurationMinutes
	if durationMin <= 0 {
		durationMin = 60 // Default 60 minutes
	}

	totalOperatingMinutes := (endHour - startHour) * 60
	if totalOperatingMinutes%durationMin != 0 {
		return nil, ErrInvalidSlotDuration
	}

	pricePaisa := int64(100000) // Default 100,000 paisa = PKR 1,000.00
	if strings.TrimSpace(req.PricePaisa) != "" {
		parsedPrice, err := strconv.ParseInt(strings.TrimSpace(req.PricePaisa), 10, 64)
		if err != nil || parsedPrice < 0 {
			return nil, ErrNegativePrice
		}
		pricePaisa = parsedPrice
	}

	// Generate non-overlapping contiguous slots across the operating window
	startTimePKT := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), startHour, 0, 0, 0, s.loc)
	slotDuration := time.Duration(durationMin) * time.Minute

	numSlots := totalOperatingMinutes / durationMin
	slots := make([]Slot, 0, numSlots)

	for i := 0; i < numSlots; i++ {
		slotStart := startTimePKT.Add(time.Duration(i) * slotDuration)
		slotEnd := slotStart.Add(slotDuration)

		slots = append(slots, Slot{
			ID:         uuid.New(),
			FacilityID: facUUID,
			StartTime:  slotStart.UTC(),
			EndTime:    slotEnd.UTC(),
			Status:     SlotStatusAvailable,
			PricePaisa: pricePaisa,
		})
	}

	created, skipped, err := s.repo.BatchCreateSlots(ctx, slots)
	if err != nil {
		return nil, fmt.Errorf("failed to generate slots in repository: %w", err)
	}

	// Query created/current slots for the day to return full response
	slotsResp, err := s.GetSlotsForDate(ctx, facUUID, dateStr, nil)
	if err != nil {
		return nil, err
	}

	return &GenerateSlotsResponse{
		Date:         dateStr,
		FacilityID:   facUUID.String(),
		SlotsCreated: created,
		SlotsSkipped: skipped,
		Slots:        slotsResp.Slots,
	}, nil
}

// UpdateSlot allows administrators to modify the price and/or status of an unbooked slot.
func (s *Service) UpdateSlot(ctx context.Context, id uuid.UUID, req UpdateSlotRequest) (*SlotDTO, error) {
	var pricePaisaPtr *int64
	if req.PricePaisa != nil && strings.TrimSpace(*req.PricePaisa) != "" {
		parsed, err := strconv.ParseInt(strings.TrimSpace(*req.PricePaisa), 10, 64)
		if err != nil || parsed < 0 {
			return nil, ErrNegativePrice
		}
		pricePaisaPtr = &parsed
	}

	var statusPtr *SlotStatus
	if req.Status != nil && strings.TrimSpace(*req.Status) != "" {
		st := SlotStatus(strings.ToUpper(strings.TrimSpace(*req.Status)))
		if !IsValidSlotStatus(string(st)) {
			return nil, ErrInvalidStatus
		}
		statusPtr = &st
	}

	updatedSlot, err := s.repo.UpdateSlot(ctx, id, pricePaisaPtr, statusPtr)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	effectiveStatus := updatedSlot.EffectiveStatus(now)

	return &SlotDTO{
		ID:         updatedSlot.ID.String(),
		FacilityID: updatedSlot.FacilityID.String(),
		StartTime:  updatedSlot.StartTime.UTC().Format(time.RFC3339),
		EndTime:    updatedSlot.EndTime.UTC().Format(time.RFC3339),
		Status:     string(effectiveStatus),
		PricePaisa: strconv.FormatInt(updatedSlot.PricePaisa, 10),
	}, nil
}

// HoldSlot acquires a temporary 5-minute hold on a slot for the authenticated user (FR-03, QR-01).
func (s *Service) HoldSlot(ctx context.Context, slotID uuid.UUID, userID uuid.UUID) (*HoldSlotResponse, error) {
	if slotID == uuid.Nil {
		return nil, errors.New("slot ID is required")
	}
	if userID == uuid.Nil {
		return nil, errors.New("user ID is required")
	}

	now := s.clock.Now()
	heldSlot, err := s.repo.HoldSlot(ctx, slotID, userID, now, DefaultHoldDuration)
	if err != nil {
		return nil, err
	}

	remainingSeconds := 0
	holdExpiresAtStr := ""
	if heldSlot.HoldExpiresAt != nil {
		holdExpiresAtStr = heldSlot.HoldExpiresAt.UTC().Format(time.RFC3339)
		rem := int(heldSlot.HoldExpiresAt.Sub(now).Seconds())
		if rem > 0 {
			remainingSeconds = rem
		}
	}

	return &HoldSlotResponse{
		SlotID:           heldSlot.ID.String(),
		FacilityID:       heldSlot.FacilityID.String(),
		StartTime:        heldSlot.StartTime.UTC().Format(time.RFC3339),
		EndTime:          heldSlot.EndTime.UTC().Format(time.RFC3339),
		Status:           string(heldSlot.Status),
		PricePaisa:       strconv.FormatInt(heldSlot.PricePaisa, 10),
		HolderID:         userID.String(),
		HoldExpiresAt:    holdExpiresAtStr,
		RemainingSeconds: remainingSeconds,
	}, nil
}

// ReleaseHold cancels an active hold owned by the authenticated user.
func (s *Service) ReleaseHold(ctx context.Context, slotID uuid.UUID, userID uuid.UUID) (*ReleaseHoldResponse, error) {
	if slotID == uuid.Nil {
		return nil, errors.New("slot ID is required")
	}
	if userID == uuid.Nil {
		return nil, errors.New("user ID is required")
	}

	releasedSlot, err := s.repo.ReleaseHold(ctx, slotID, userID)
	if err != nil {
		return nil, err
	}

	return &ReleaseHoldResponse{
		SlotID:  releasedSlot.ID.String(),
		Status:  string(releasedSlot.Status),
		Message: "Hold released successfully",
	}, nil
}

// Checkout coordinates the atomic booking confirmation and wallet debit using the Abstract Factory pattern (FR-03, QR-01, QR-03, QR-04).
func (s *Service) Checkout(ctx context.Context, req CheckoutRequest, userID uuid.UUID) (*BookingConfirmationDTO, error) {
	if s.uow == nil {
		return nil, errors.New("unit of work is uninitialized")
	}
	if s.walletSvc == nil {
		return nil, errors.New("wallet service is uninitialized")
	}
	if userID == uuid.Nil {
		return nil, errors.New("user ID is required")
	}

	slotID, err := uuid.Parse(strings.TrimSpace(req.SlotID))
	if err != nil {
		return nil, ErrSlotNotFound
	}

	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey == "" {
		return nil, ErrBlankIdempotencyKey
	}

	var confirmed *BookingConfirmationDTO

	err = s.uow.Execute(ctx, func(factory TransactionRepositoryFactory) error {
		// 1. Fast pre-lock idempotency check: allows sequential replays to return immediately
		existing, err := factory.Bookings().FindBookingByIdempotency(ctx, userID, idempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.SlotID != slotID {
				return ErrIdempotencyConflict
			}
			// Idempotent replay: populate DTO from existing confirmed booking and return (QR-04)
			slot, err := factory.Bookings().LockSlotForBooking(ctx, existing.SlotID)
			if err != nil {
				return err
			}
			var debitTxID string
			if existing.DebitTransactionID != nil {
				debitTxID = existing.DebitTransactionID.String()
			}
			confirmed = &BookingConfirmationDTO{
				BookingID:          existing.ID.String(),
				SlotID:             existing.SlotID.String(),
				FacilityID:         slot.FacilityID.String(),
				StartTime:          slot.StartTime.UTC().Format(time.RFC3339),
				EndTime:            slot.EndTime.UTC().Format(time.RFC3339),
				Status:             string(existing.Status),
				FeePaisa:           strconv.FormatInt(existing.FeePaisa, 10),
				CheckInCode:        existing.CheckInCode,
				DebitTransactionID: debitTxID,
				CreatedAt:          existing.CreatedAt.UTC().Format(time.RFC3339),
				AlreadyBooked:      true,
			}
			return nil
		}

		// 2. Lock the target slot row with SELECT ... FOR UPDATE (QR-01 double-booking prevention)
		slot, err := factory.Bookings().LockSlotForBooking(ctx, slotID)
		if err != nil {
			return err
		}

		// 3. Post-lock idempotency check in case a concurrent request with identical key committed while waiting
		existing, err = factory.Bookings().FindBookingByIdempotency(ctx, userID, idempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.SlotID != slotID {
				return ErrIdempotencyConflict
			}
			var debitTxID string
			if existing.DebitTransactionID != nil {
				debitTxID = existing.DebitTransactionID.String()
			}
			confirmed = &BookingConfirmationDTO{
				BookingID:          existing.ID.String(),
				SlotID:             existing.SlotID.String(),
				FacilityID:         slot.FacilityID.String(),
				StartTime:          slot.StartTime.UTC().Format(time.RFC3339),
				EndTime:            slot.EndTime.UTC().Format(time.RFC3339),
				Status:             string(existing.Status),
				FeePaisa:           strconv.FormatInt(existing.FeePaisa, 10),
				CheckInCode:        existing.CheckInCode,
				DebitTransactionID: debitTxID,
				CreatedAt:          existing.CreatedAt.UTC().Format(time.RFC3339),
				AlreadyBooked:      true,
			}
			return nil
		}

		// 4. Validate slot status and 5-minute hold invariants (FR-03)
		now := s.clock.Now()
		if slot.Status == SlotStatusBooked || slot.Status == SlotStatusBlocked {
			return ErrSlotUnavailable
		}
		if slot.Status != SlotStatusHeld {
			return ErrNotSlotHolder
		}
		if slot.HolderID == nil || *slot.HolderID != userID {
			return ErrNotSlotHolder
		}
		if slot.IsHoldExpired(now) {
			return ErrHoldExpired
		}

		// 5. Resolve user personal wallet and system revenue wallet within transaction via Abstract Factory
		userWalletID, err := factory.Ledger().FindPersonalWallet(ctx, userID)
		if err != nil {
			return err
		}
		sysWalletID, err := factory.Ledger().FindOrCreateSystemWallet(ctx, "Sports Revenue", wallet.WalletTypeRevenue, "PKR")
		if err != nil {
			return err
		}

		// 6. Execute atomic wallet debit within transaction via Abstract Factory Ledger repository
		bookingID := uuid.New()
		transferParams := wallet.TransferParams{
			SenderWalletID:   userWalletID,
			ReceiverWalletID: sysWalletID,
			Amount:           slot.PricePaisa,
			Type:             "PADEL_BOOKING",
			ReferenceID:      fmt.Sprintf("padel_booking:%s", bookingID.String()),
			Description:      fmt.Sprintf("Padel court reservation for slot %s", slot.ID.String()),
		}

		postResult, err := factory.Ledger().TransferFunds(ctx, transferParams)
		if err != nil {
			return err
		}

		// 7. Generate cryptographically random 4-digit check-in verification code
		code, err := GenerateCheckInCode()
		if err != nil {
			return err
		}

		// 8. Insert confirmed booking record in padel_bookings
		booking := &Booking{
			ID:                 bookingID,
			SlotID:             slot.ID,
			UserID:             userID,
			Status:             BookingStatusConfirmed,
			FeePaisa:           slot.PricePaisa,
			IdempotencyKey:     idempotencyKey,
			CheckInCode:        code,
			DebitTransactionID: &postResult.TransactionID,
		}

		if err := factory.Bookings().CreateBooking(ctx, booking); err != nil {
			return err
		}

		// 9. Transition slot status to BOOKED
		if err := factory.Bookings().MarkSlotBooked(ctx, slot.ID); err != nil {
			return err
		}

		// 10. Record immutable audit log entry in giki_sports.audit_logs
		auditDetails := map[string]interface{}{
			"slot_id":         slot.ID.String(),
			"facility_id":     slot.FacilityID.String(),
			"fee_paisa":       slot.PricePaisa,
			"debit_tx_id":     postResult.TransactionID.String(),
			"idempotency_key": idempotencyKey,
		}
		_ = factory.Bookings().RecordAuditLog(ctx, userID, "booking.checkout", "padel_booking", booking.ID, auditDetails)

		confirmed = &BookingConfirmationDTO{
			BookingID:          booking.ID.String(),
			SlotID:             slot.ID.String(),
			FacilityID:         slot.FacilityID.String(),
			StartTime:          slot.StartTime.UTC().Format(time.RFC3339),
			EndTime:            slot.EndTime.UTC().Format(time.RFC3339),
			Status:             string(BookingStatusConfirmed),
			FeePaisa:           strconv.FormatInt(slot.PricePaisa, 10),
			CheckInCode:        code,
			DebitTransactionID: postResult.TransactionID.String(),
			CreatedAt:          booking.CreatedAt.UTC().Format(time.RFC3339),
			AlreadyBooked:      false,
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Populate facility name if available
	if confirmed != nil {
		if facUUID, parseErr := uuid.Parse(confirmed.FacilityID); parseErr == nil {
			if fac, facErr := s.repo.GetFacilityByID(ctx, facUUID); facErr == nil && fac != nil {
				confirmed.FacilityName = fac.Name
			}
		}
	}

	return confirmed, nil
}

// VerifyTicket validates a court check-in ticket code, transitions the booking from CONFIRMED to CHECKED_IN,
// and records the attendant's audit metadata (FR-03, TC-FR-03-03).
func (s *Service) VerifyTicket(ctx context.Context, req VerifyTicketRequest, attendantID uuid.UUID) (*VerifyTicketResponse, error) {
	if attendantID == uuid.Nil {
		return nil, errors.New("attendant ID is required")
	}

	checkInCode := strings.TrimSpace(req.CheckInCode)
	if checkInCode == "" {
		return nil, ErrInvalidCheckInCode
	}

	var bookingUUID *uuid.UUID
	if strings.TrimSpace(req.BookingID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(req.BookingID))
		if err != nil {
			return nil, ErrBookingNotFound
		}
		bookingUUID = &parsed
	}

	now := s.clock.Now()
	return s.repo.VerifyTicket(ctx, checkInCode, bookingUUID, attendantID, now)
}

// CancelBooking processes an administrative booking cancellation and full 100% wallet ledger refund (FR-06, TC-FR-06-02, QR-03).
func (s *Service) CancelBooking(ctx context.Context, bookingID uuid.UUID, adminID uuid.UUID, req CancelBookingRequest) (*CancelBookingResponse, error) {
	if s.uow == nil {
		return nil, errors.New("unit of work is uninitialized")
	}
	if bookingID == uuid.Nil {
		return nil, ErrBookingNotFound
	}
	if adminID == uuid.Nil {
		return nil, errors.New("admin ID is required")
	}

	var resp *CancelBookingResponse

	err := s.uow.Execute(ctx, func(factory TransactionRepositoryFactory) error {
		// 1. Lock the booking row with SELECT ... FOR UPDATE (concurrency & race prevention)
		booking, err := factory.Bookings().LockBookingForUpdate(ctx, bookingID)
		if err != nil {
			return err
		}

		// 2. Validate current booking state
		if booking.Status == BookingStatusCancelled {
			return ErrBookingAlreadyCancelled
		}
		if booking.Status == BookingStatusCheckedIn {
			return ErrCannotCancelCheckedInBooking
		}
		if booking.Status != BookingStatusConfirmed {
			return errors.New("only confirmed bookings can be cancelled")
		}

		// 3. Lock the associated slot row
		_, err = factory.Bookings().LockSlotForBooking(ctx, booking.SlotID)
		if err != nil && !errors.Is(err, ErrSlotNotFound) {
			return err
		}

		// 4. Resolve wallets within the transaction via Abstract Factory
		sysWalletID, err := factory.Ledger().FindOrCreateSystemWallet(ctx, "Sports Revenue", wallet.WalletTypeRevenue, "PKR")
		if err != nil {
			return fmt.Errorf("failed to resolve system revenue wallet: %w", err)
		}
		userWalletID, err := factory.Ledger().FindPersonalWallet(ctx, booking.UserID)
		if err != nil {
			return fmt.Errorf("failed to resolve user personal wallet: %w", err)
		}

		// 5. Post 100% full refund transfer from SYS_REVENUE to User Personal Wallet (FR-06, single-refund invariant)
		refundParams := wallet.TransferParams{
			SenderWalletID:   sysWalletID,
			ReceiverWalletID: userWalletID,
			Amount:           booking.FeePaisa, // 100% full refund without deduction
			Type:             "PADEL_REFUND",
			ReferenceID:      fmt.Sprintf("padel_refund:%s", booking.ID.String()),
			Description:      fmt.Sprintf("100%% refund for cancelled padel court reservation %s", booking.ID.String()),
		}

		postResult, err := factory.Ledger().TransferFunds(ctx, refundParams)
		if err != nil {
			return fmt.Errorf("failed to process refund transfer: %w", err)
		}

		// 6. Release slot back to AVAILABLE (clear hold metadata and free for re-booking)
		if err := factory.Bookings().ReleaseSlotToAvailable(ctx, booking.SlotID); err != nil {
			return fmt.Errorf("failed to release slot: %w", err)
		}

		// 7. Transition booking status to CANCELLED and record refund transaction ID
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			reason = "Administrative cancellation"
		}
		now := s.clock.Now().UTC()

		cancelledBooking, err := factory.Bookings().CancelBooking(ctx, booking.ID, adminID, reason, postResult.TransactionID, now)
		if err != nil {
			return fmt.Errorf("failed to cancel booking: %w", err)
		}

		// 8. Record audit log entry in giki_sports.audit_logs
		auditDetails := map[string]interface{}{
			"booking_id":   booking.ID.String(),
			"slot_id":      booking.SlotID.String(),
			"user_id":      booking.UserID.String(),
			"fee_paisa":    booking.FeePaisa,
			"refund_tx_id": postResult.TransactionID.String(),
			"reason":       reason,
		}
		_ = factory.Bookings().RecordAuditLog(ctx, adminID, "booking.admin_cancel", "padel_booking", booking.ID, auditDetails)

		resp = &CancelBookingResponse{
			BookingID:           cancelledBooking.ID.String(),
			SlotID:              cancelledBooking.SlotID.String(),
			UserID:              cancelledBooking.UserID.String(),
			Status:              string(cancelledBooking.Status),
			FeePaisa:            strconv.FormatInt(cancelledBooking.FeePaisa, 10),
			RefundTransactionID: postResult.TransactionID.String(),
			CancelledAt:         now.Format(time.RFC3339),
			CancelledBy:         adminID.String(),
			Reason:              reason,
			Message:             "Booking successfully cancelled and 100% refund credited to user wallet",
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}
