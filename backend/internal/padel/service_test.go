package padel

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

type mockRepository struct {
	facilities []FacilitySummaryDTO
	slots      map[uuid.UUID]Slot
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		facilities: make([]FacilitySummaryDTO, 0),
		slots:      make(map[uuid.UUID]Slot),
	}
}

func (m *mockRepository) GetFacilityByID(ctx context.Context, id uuid.UUID) (*FacilitySummaryDTO, error) {
	for _, f := range m.facilities {
		if f.ID == id.String() {
			return &f, nil
		}
	}
	return nil, ErrFacilityNotFound
}

func (m *mockRepository) GetPadelFacilities(ctx context.Context) ([]FacilitySummaryDTO, error) {
	return m.facilities, nil
}

func (m *mockRepository) FindSlots(ctx context.Context, facilityID uuid.UUID, startFrom, startBefore time.Time) ([]Slot, error) {
	var result []Slot
	for _, s := range m.slots {
		if s.FacilityID == facilityID && !s.StartTime.Before(startFrom) && s.StartTime.Before(startBefore) {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockRepository) GetSlotByID(ctx context.Context, id uuid.UUID) (*Slot, error) {
	s, ok := m.slots[id]
	if !ok {
		return nil, ErrSlotNotFound
	}
	return &s, nil
}

func (m *mockRepository) CreateSlot(ctx context.Context, slot *Slot) (*Slot, error) {
	m.slots[slot.ID] = *slot
	return slot, nil
}

func (m *mockRepository) BatchCreateSlots(ctx context.Context, slots []Slot) (int, int, error) {
	created := 0
	skipped := 0
	for _, s := range slots {
		exists := false
		for _, existing := range m.slots {
			if existing.FacilityID == s.FacilityID && existing.StartTime.Equal(s.StartTime) {
				exists = true
				break
			}
		}
		if exists {
			skipped++
		} else {
			m.slots[s.ID] = s
			created++
		}
	}
	return created, skipped, nil
}

func (m *mockRepository) UpdateSlot(ctx context.Context, id uuid.UUID, pricePaisa *int64, status *SlotStatus) (*Slot, error) {
	s, ok := m.slots[id]
	if !ok {
		return nil, ErrSlotNotFound
	}
	if s.Status == SlotStatusBooked {
		return nil, ErrSlotBooked
	}
	if pricePaisa != nil {
		s.PricePaisa = *pricePaisa
	}
	if status != nil {
		s.Status = *status
	}
	m.slots[id] = s
	return &s, nil
}

func (m *mockRepository) HoldSlot(ctx context.Context, slotID uuid.UUID, userID uuid.UUID, now time.Time, holdDuration time.Duration) (*Slot, error) {
	s, ok := m.slots[slotID]
	if !ok {
		return nil, ErrSlotNotFound
	}
	if s.Status == SlotStatusBooked || s.Status == SlotStatusBlocked {
		return nil, ErrSlotUnavailable
	}
	if s.Status == SlotStatusHeld {
		if s.HolderID != nil && *s.HolderID != userID && !s.IsHoldExpired(now) {
			return nil, ErrSlotConflict
		}
	}
	expiry := now.Add(holdDuration)
	s.Status = SlotStatusHeld
	s.HolderID = &userID
	s.HoldExpiresAt = &expiry
	m.slots[slotID] = s
	return &s, nil
}

func (m *mockRepository) ReleaseHold(ctx context.Context, slotID uuid.UUID, userID uuid.UUID) (*Slot, error) {
	s, ok := m.slots[slotID]
	if !ok {
		return nil, ErrSlotNotFound
	}
	if s.Status != SlotStatusHeld {
		return nil, ErrSlotUnavailable
	}
	if s.HolderID != nil && *s.HolderID != userID {
		return nil, ErrHoldNotOwned
	}
	s.Status = SlotStatusAvailable
	s.HolderID = nil
	s.HoldExpiresAt = nil
	m.slots[slotID] = s
	return &s, nil
}

func (m *mockRepository) CleanExpiredHolds(ctx context.Context, now time.Time) (int64, error) {
	var count int64
	for id, s := range m.slots {
		if s.Status == SlotStatusHeld && s.HoldExpiresAt != nil && !now.Before(*s.HoldExpiresAt) {
			s.Status = SlotStatusAvailable
			s.HolderID = nil
			s.HoldExpiresAt = nil
			m.slots[id] = s
			count++
		}
	}
	return count, nil
}

func (m *mockRepository) VerifyTicket(ctx context.Context, checkInCode string, bookingID *uuid.UUID, attendantID uuid.UUID, now time.Time) (*VerifyTicketResponse, error) {
	if checkInCode == "VALID1" {
		return &VerifyTicketResponse{
			BookingID:    uuid.New().String(),
			SlotID:       uuid.New().String(),
			FacilityID:   uuid.New().String(),
			FacilityName: "Mock Padel Arena",
			UserID:       uuid.New().String(),
			UserEmail:    "alice@example.test",
			UserName:     "Alice",
			StartTime:    now.Format(time.RFC3339),
			EndTime:      now.Add(time.Hour).Format(time.RFC3339),
			Status:       string(BookingStatusCheckedIn),
			FeePaisa:     "100000",
			CheckInCode:  checkInCode,
			CheckedInAt:  now.Format(time.RFC3339),
			CheckedInBy:  attendantID.String(),
		}, nil
	}
	if checkInCode == "ALREADY" {
		return nil, ErrTicketAlreadyCheckedIn
	}
	if checkInCode == "CANCELLED" {
		return nil, ErrBookingCancelled
	}
	return nil, ErrBookingNotFound
}

type staticClock struct {
	t time.Time
}

func (c staticClock) Now() time.Time {
	return c.t
}

func TestService_GenerateDailySlots(t *testing.T) {
	repo := newMockRepository()
	facID := uuid.New()
	repo.facilities = append(repo.facilities, FacilitySummaryDTO{
		ID:     facID.String(),
		Slug:   "padel-court-1",
		Name:   "Padel Court 1",
		Status: "active",
	})

	now := time.Date(2026, 11, 15, 6, 0, 0, 0, time.UTC)
	svc := NewServiceWithClock(repo, nil, nil, staticClock{t: now}, DefaultPakistanLocation())

	t.Run("generate standard 60-minute daily slots (08:00 to 23:00 = 15 slots)", func(t *testing.T) {
		resp, err := svc.GenerateDailySlots(context.Background(), GenerateSlotsRequest{
			FacilityID:          facID.String(),
			Date:                "2026-11-15",
			StartHour:           8,
			EndHour:             23,
			SlotDurationMinutes: 60,
			PricePaisa:          "150000",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.SlotsCreated != 15 {
			t.Fatalf("expected 15 slots created, got %d", resp.SlotsCreated)
		}
		if resp.SlotsSkipped != 0 {
			t.Fatalf("expected 0 slots skipped, got %d", resp.SlotsSkipped)
		}
		if len(resp.Slots) != 15 {
			t.Fatalf("expected 15 slots in response, got %d", len(resp.Slots))
		}
		if resp.Slots[0].PricePaisa != "150000" {
			t.Errorf("expected price 150000, got %s", resp.Slots[0].PricePaisa)
		}
	})

	t.Run("rerun generation skips duplicate slots without error", func(t *testing.T) {
		resp, err := svc.GenerateDailySlots(context.Background(), GenerateSlotsRequest{
			FacilityID:          facID.String(),
			Date:                "2026-11-15",
			StartHour:           8,
			EndHour:             23,
			SlotDurationMinutes: 60,
			PricePaisa:          "150000",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.SlotsCreated != 0 {
			t.Errorf("expected 0 slots created on rerun, got %d", resp.SlotsCreated)
		}
		if resp.SlotsSkipped != 15 {
			t.Errorf("expected 15 slots skipped on rerun, got %d", resp.SlotsSkipped)
		}
	})

	t.Run("invalid slot duration fails", func(t *testing.T) {
		_, err := svc.GenerateDailySlots(context.Background(), GenerateSlotsRequest{
			FacilityID:          facID.String(),
			Date:                "2026-11-16",
			StartHour:           8,
			EndHour:             11, // 3 hours = 180 min
			SlotDurationMinutes: 50, // 180 not divisible by 50
			PricePaisa:          "100000",
		})
		if err != ErrInvalidSlotDuration {
			t.Errorf("expected ErrInvalidSlotDuration, got: %v", err)
		}
	})

	t.Run("negative price fails", func(t *testing.T) {
		_, err := svc.GenerateDailySlots(context.Background(), GenerateSlotsRequest{
			FacilityID:          facID.String(),
			Date:                "2026-11-16",
			StartHour:           8,
			EndHour:             10,
			SlotDurationMinutes: 60,
			PricePaisa:          "-500",
		})
		if err != ErrNegativePrice {
			t.Errorf("expected ErrNegativePrice, got: %v", err)
		}
	})
}

func TestService_GetSlotsForDate_LazyHoldExpiration(t *testing.T) {
	repo := newMockRepository()
	facID := uuid.New()
	repo.facilities = append(repo.facilities, FacilitySummaryDTO{
		ID:     facID.String(),
		Slug:   "padel-court-1",
		Name:   "Padel Court 1",
		Status: "active",
	})

	loc := DefaultPakistanLocation()
	slotTimePKT := time.Date(2026, 11, 15, 14, 0, 0, 0, loc)
	slotID := uuid.New()
	aliceID := uuid.New()
	bobID := uuid.New()

	// Held slot with hold expiring at 14:05 PKT
	holdExpiryUTC := slotTimePKT.Add(5 * time.Minute).UTC()

	repo.slots[slotID] = Slot{
		ID:            slotID,
		FacilityID:    facID,
		StartTime:     slotTimePKT.UTC(),
		EndTime:       slotTimePKT.Add(60 * time.Minute).UTC(),
		Status:        SlotStatusHeld,
		HolderID:      &aliceID,
		HoldExpiresAt: &holdExpiryUTC,
		PricePaisa:    100000,
	}

	t.Run("unexpired hold shows as HELD and isHeldByMe true for Alice", func(t *testing.T) {
		clock := staticClock{t: slotTimePKT.Add(2 * time.Minute).UTC()}
		svc := NewServiceWithClock(repo, nil, nil, clock, loc)

		resp, err := svc.GetSlotsForDate(context.Background(), facID, "2026-11-15", &aliceID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Slots) != 1 {
			t.Fatalf("expected 1 slot, got %d", len(resp.Slots))
		}
		slot := resp.Slots[0]
		if slot.Status != "HELD" {
			t.Errorf("expected status HELD, got %s", slot.Status)
		}
		if !slot.IsHeldByMe {
			t.Errorf("expected isHeldByMe to be true for Alice")
		}
		if slot.HoldExpiresAt == nil {
			t.Errorf("expected holdExpiresAt to be populated")
		}
	})

	t.Run("unexpired hold shows as HELD and isHeldByMe false for Bob", func(t *testing.T) {
		clock := staticClock{t: slotTimePKT.Add(2 * time.Minute).UTC()}
		svc := NewServiceWithClock(repo, nil, nil, clock, loc)

		resp, err := svc.GetSlotsForDate(context.Background(), facID, "2026-11-15", &bobID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slot := resp.Slots[0]
		if slot.Status != "HELD" {
			t.Errorf("expected status HELD, got %s", slot.Status)
		}
		if slot.IsHeldByMe {
			t.Errorf("expected isHeldByMe to be false for Bob")
		}
	})

	t.Run("expired hold lazily evaluates as AVAILABLE for all viewers", func(t *testing.T) {
		// 6 minutes later -> hold is expired!
		clock := staticClock{t: slotTimePKT.Add(6 * time.Minute).UTC()}
		svc := NewServiceWithClock(repo, nil, nil, clock, loc)

		resp, err := svc.GetSlotsForDate(context.Background(), facID, "2026-11-15", &aliceID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slot := resp.Slots[0]
		if slot.Status != "AVAILABLE" {
			t.Errorf("expected lazy evaluation to resolve to AVAILABLE, got %s", slot.Status)
		}
		if slot.IsHeldByMe {
			t.Errorf("expected isHeldByMe to be false once expired")
		}
	})
}

func TestService_HoldAndReleaseSlot(t *testing.T) {
	repo := newMockRepository()
	facID := uuid.New()
	repo.facilities = append(repo.facilities, FacilitySummaryDTO{
		ID:     facID.String(),
		Slug:   "padel-court-1",
		Name:   "Padel Court 1",
		Status: "active",
	})

	slotID := uuid.New()
	startTime := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	repo.slots[slotID] = Slot{
		ID:         slotID,
		FacilityID: facID,
		StartTime:  startTime,
		EndTime:    startTime.Add(60 * time.Minute),
		Status:     SlotStatusAvailable,
		PricePaisa: 100000,
	}

	aliceID := uuid.New()
	bobID := uuid.New()

	baseTime := time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC)
	clock := staticClock{t: baseTime}
	svc := NewServiceWithClock(repo, nil, nil, clock, DefaultPakistanLocation())

	t.Run("Alice successfully acquires 5-minute hold on available slot", func(t *testing.T) {
		resp, err := svc.HoldSlot(context.Background(), slotID, aliceID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Status != "HELD" {
			t.Errorf("expected status HELD, got %s", resp.Status)
		}
		if resp.HolderID != aliceID.String() {
			t.Errorf("expected holder Alice, got %s", resp.HolderID)
		}
		if resp.RemainingSeconds != 300 {
			t.Errorf("expected 300 remaining seconds, got %d", resp.RemainingSeconds)
		}
	})

	t.Run("Bob attempting to hold the same slot while active receives ErrSlotConflict", func(t *testing.T) {
		_, err := svc.HoldSlot(context.Background(), slotID, bobID)
		if err != ErrSlotConflict {
			t.Errorf("expected ErrSlotConflict, got: %v", err)
		}
	})

	t.Run("Bob cannot release Alice's hold", func(t *testing.T) {
		_, err := svc.ReleaseHold(context.Background(), slotID, bobID)
		if err != ErrHoldNotOwned {
			t.Errorf("expected ErrHoldNotOwned, got: %v", err)
		}
	})

	t.Run("Alice can voluntarily release her hold", func(t *testing.T) {
		resp, err := svc.ReleaseHold(context.Background(), slotID, aliceID)
		if err != nil {
			t.Fatalf("unexpected error releasing hold: %v", err)
		}
		if resp.Status != "AVAILABLE" {
			t.Errorf("expected status AVAILABLE, got %s", resp.Status)
		}
	})

	t.Run("After release, Bob can acquire hold", func(t *testing.T) {
		resp, err := svc.HoldSlot(context.Background(), slotID, bobID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.HolderID != bobID.String() {
			t.Errorf("expected holder Bob, got %s", resp.HolderID)
		}
	})
}
