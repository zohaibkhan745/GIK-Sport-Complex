package padel

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHoldCleanerWorker_Lifecycle(t *testing.T) {
	repo := newMockRepository()
	cfg := CleanerConfig{
		Interval:     50 * time.Millisecond,
		BatchTimeout: 100 * time.Millisecond,
	}
	worker := NewHoldCleanerWorker(repo, cfg)

	if worker.IsRunning() {
		t.Errorf("worker should not be running before Start()")
	}

	// First start
	worker.Start()
	if !worker.IsRunning() {
		t.Errorf("worker should be running after Start()")
	}

	// Idempotent double start
	worker.Start()
	if !worker.IsRunning() {
		t.Errorf("worker should still be running after double Start()")
	}

	// Stop
	worker.Stop()
	if worker.IsRunning() {
		t.Errorf("worker should not be running after Stop()")
	}

	// Idempotent double stop
	worker.Stop()
	if worker.IsRunning() {
		t.Errorf("worker should not be running after double Stop()")
	}
}

func TestHoldCleanerWorker_RunOnce(t *testing.T) {
	repo := newMockRepository()
	facID := uuid.New()
	aliceID := uuid.New()
	bobID := uuid.New()

	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	pastExpiry := now.Add(-1 * time.Minute)
	futureExpiry := now.Add(4 * time.Minute)

	// Expired slot
	slot1ID := uuid.New()
	repo.slots[slot1ID] = Slot{
		ID:            slot1ID,
		FacilityID:    facID,
		StartTime:     now.Add(1 * time.Hour),
		EndTime:       now.Add(2 * time.Hour),
		Status:        SlotStatusHeld,
		HolderID:      &aliceID,
		HoldExpiresAt: &pastExpiry,
		PricePaisa:    100000,
	}

	// Unexpired slot
	slot2ID := uuid.New()
	repo.slots[slot2ID] = Slot{
		ID:            slot2ID,
		FacilityID:    facID,
		StartTime:     now.Add(2 * time.Hour),
		EndTime:       now.Add(3 * time.Hour),
		Status:        SlotStatusHeld,
		HolderID:      &bobID,
		HoldExpiresAt: &futureExpiry,
		PricePaisa:    100000,
	}

	worker := NewHoldCleanerWorkerWithClock(repo, staticClock{t: now}, CleanerConfig{
		Interval:     10 * time.Second,
		BatchTimeout: 1 * time.Second,
	}, nil)

	cleaned, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error running cleaner: %v", err)
	}
	if cleaned != 1 {
		t.Fatalf("expected 1 slot cleaned, got %d", cleaned)
	}

	// Verify slot 1 was released to AVAILABLE
	slot1 := repo.slots[slot1ID]
	if slot1.Status != SlotStatusAvailable {
		t.Errorf("expected slot 1 to be AVAILABLE, got %s", slot1.Status)
	}
	if slot1.HolderID != nil || slot1.HoldExpiresAt != nil {
		t.Errorf("expected slot 1 holder and expiry to be nil")
	}

	// Verify slot 2 remains HELD
	slot2 := repo.slots[slot2ID]
	if slot2.Status != SlotStatusHeld {
		t.Errorf("expected slot 2 to remain HELD, got %s", slot2.Status)
	}
	if slot2.HolderID == nil || *slot2.HolderID != bobID {
		t.Errorf("expected slot 2 to remain held by Bob")
	}
}

func TestHoldCleanerWorker_BackgroundTicker(t *testing.T) {
	repo := newMockRepository()
	facID := uuid.New()
	aliceID := uuid.New()

	baseTime := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	pastExpiry := baseTime.Add(-1 * time.Minute)

	slotID := uuid.New()
	repo.slots[slotID] = Slot{
		ID:            slotID,
		FacilityID:    facID,
		StartTime:     baseTime.Add(1 * time.Hour),
		EndTime:       baseTime.Add(2 * time.Hour),
		Status:        SlotStatusHeld,
		HolderID:      &aliceID,
		HoldExpiresAt: &pastExpiry,
		PricePaisa:    100000,
	}

	// Start worker with very short 10ms interval
	worker := NewHoldCleanerWorkerWithClock(repo, staticClock{t: baseTime}, CleanerConfig{
		Interval:     10 * time.Millisecond,
		BatchTimeout: 100 * time.Millisecond,
	}, nil)

	worker.Start()
	defer worker.Stop()

	// Wait up to 250ms for ticker to fire
	success := false
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		s := repo.slots[slotID]
		if s.Status == SlotStatusAvailable {
			success = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !success {
		t.Errorf("expected background cleaner ticker to release expired hold within deadline")
	}
}
