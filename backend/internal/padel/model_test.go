package padel

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSlot_EffectiveStatusAndHoldExpiration(t *testing.T) {
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	holderID := uuid.New()

	t.Run("available slot remains available", func(t *testing.T) {
		s := Slot{
			Status: SlotStatusAvailable,
		}
		if s.IsHoldExpired(now) {
			t.Errorf("expected IsHoldExpired to be false for available slot")
		}
		if s.EffectiveStatus(now) != SlotStatusAvailable {
			t.Errorf("expected effective status AVAILABLE, got %s", s.EffectiveStatus(now))
		}
	})

	t.Run("held slot with future expiry remains held", func(t *testing.T) {
		futureExpiry := now.Add(5 * time.Minute)
		s := Slot{
			Status:        SlotStatusHeld,
			HolderID:      &holderID,
			HoldExpiresAt: &futureExpiry,
		}
		if s.IsHoldExpired(now) {
			t.Errorf("expected IsHoldExpired to be false for unexpired hold")
		}
		if s.EffectiveStatus(now) != SlotStatusHeld {
			t.Errorf("expected effective status HELD, got %s", s.EffectiveStatus(now))
		}
	})

	t.Run("held slot with exact current time is expired", func(t *testing.T) {
		exactExpiry := now
		s := Slot{
			Status:        SlotStatusHeld,
			HolderID:      &holderID,
			HoldExpiresAt: &exactExpiry,
		}
		if !s.IsHoldExpired(now) {
			t.Errorf("expected IsHoldExpired to be true when now == holdExpiresAt")
		}
		if s.EffectiveStatus(now) != SlotStatusAvailable {
			t.Errorf("expected lazy status to resolve to AVAILABLE, got %s", s.EffectiveStatus(now))
		}
	})

	t.Run("held slot with past expiry lazily resolves to available", func(t *testing.T) {
		pastExpiry := now.Add(-10 * time.Second)
		s := Slot{
			Status:        SlotStatusHeld,
			HolderID:      &holderID,
			HoldExpiresAt: &pastExpiry,
		}
		if !s.IsHoldExpired(now) {
			t.Errorf("expected IsHoldExpired to be true for past expiry")
		}
		if s.EffectiveStatus(now) != SlotStatusAvailable {
			t.Errorf("expected lazy status to resolve to AVAILABLE, got %s", s.EffectiveStatus(now))
		}
	})

	t.Run("booked slot remains booked", func(t *testing.T) {
		s := Slot{
			Status: SlotStatusBooked,
		}
		if s.IsHoldExpired(now) {
			t.Errorf("expected IsHoldExpired to be false for booked slot")
		}
		if s.EffectiveStatus(now) != SlotStatusBooked {
			t.Errorf("expected effective status BOOKED, got %s", s.EffectiveStatus(now))
		}
	})
}

func TestSlot_Validate(t *testing.T) {
	validFacID := uuid.New()
	holderID := uuid.New()
	startTime := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	endTime := startTime.Add(60 * time.Minute)
	futureExpiry := startTime.Add(5 * time.Minute)

	t.Run("valid available slot passes validation", func(t *testing.T) {
		s := Slot{
			FacilityID: validFacID,
			StartTime:  startTime,
			EndTime:    endTime,
			Status:     SlotStatusAvailable,
			PricePaisa: 100000,
		}
		if err := s.Validate(); err != nil {
			t.Errorf("expected valid slot to pass validation, got: %v", err)
		}
	})

	t.Run("missing facility ID fails", func(t *testing.T) {
		s := Slot{
			StartTime:  startTime,
			EndTime:    endTime,
			Status:     SlotStatusAvailable,
			PricePaisa: 100000,
		}
		if err := s.Validate(); err == nil {
			t.Errorf("expected error for missing facility ID, got nil")
		}
	})

	t.Run("end time before or equal start time fails", func(t *testing.T) {
		s := Slot{
			FacilityID: validFacID,
			StartTime:  endTime,
			EndTime:    startTime,
			Status:     SlotStatusAvailable,
			PricePaisa: 100000,
		}
		if err := s.Validate(); err != ErrInvalidTimeRange {
			t.Errorf("expected ErrInvalidTimeRange, got: %v", err)
		}
	})

	t.Run("negative price fails", func(t *testing.T) {
		s := Slot{
			FacilityID: validFacID,
			StartTime:  startTime,
			EndTime:    endTime,
			Status:     SlotStatusAvailable,
			PricePaisa: -100,
		}
		if err := s.Validate(); err != ErrNegativePrice {
			t.Errorf("expected ErrNegativePrice, got: %v", err)
		}
	})

	t.Run("held slot without holder fails", func(t *testing.T) {
		s := Slot{
			FacilityID:    validFacID,
			StartTime:     startTime,
			EndTime:       endTime,
			Status:        SlotStatusHeld,
			HolderID:      nil,
			HoldExpiresAt: &futureExpiry,
			PricePaisa:    100000,
		}
		if err := s.Validate(); err != ErrInconsistentHold {
			t.Errorf("expected ErrInconsistentHold, got: %v", err)
		}
	})

	t.Run("available slot with holder fails", func(t *testing.T) {
		s := Slot{
			FacilityID:    validFacID,
			StartTime:     startTime,
			EndTime:       endTime,
			Status:        SlotStatusAvailable,
			HolderID:      &holderID,
			HoldExpiresAt: &futureExpiry,
			PricePaisa:    100000,
		}
		if err := s.Validate(); err != ErrInconsistentHold {
			t.Errorf("expected ErrInconsistentHold, got: %v", err)
		}
	})
}
