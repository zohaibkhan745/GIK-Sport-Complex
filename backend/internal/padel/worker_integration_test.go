//go:build integration

package padel

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHoldCleanerWorker_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Seed facility
	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000001")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-court-cleaner', 'Cleaner Test Court', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	// Seed users Alice and Bob
	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000001")
	bobID := uuid.MustParse("a0000000-0000-0000-0000-000000000002")
	for _, u := range []struct {
		id    uuid.UUID
		email string
		name  string
	}{
		{aliceID, "alice.cleaner@example.test", "Alice Cleaner"},
		{bobID, "bob.cleaner@example.test", "Bob Cleaner"},
	} {
		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
			VALUES ($1, $2, $3, 'fakehash', 'active');
		`, u.id, u.email, u.name)
		if err != nil {
			t.Fatalf("failed to seed user %s: %v", u.name, err)
		}
	}

	now := time.Now().UTC()

	// Slot 1: Expired hold (Alice)
	slot1ID := uuid.New()
	slot1Start := now.Add(24 * time.Hour)
	slot1End := slot1Start.Add(60 * time.Minute)
	slot1HoldExpiry := now.Add(-2 * time.Minute)

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.padel_slots (id, facility_id, start_time, end_time, status, holder_id, hold_expires_at, price_paisa)
		VALUES ($1, $2, $3, $4, 'HELD', $5, $6, 100000);
	`, slot1ID, facID, slot1Start, slot1End, aliceID, slot1HoldExpiry)
	if err != nil {
		t.Fatalf("failed to insert expired slot: %v", err)
	}

	// Slot 2: Active unexpired hold (Bob)
	slot2ID := uuid.New()
	slot2Start := slot1End
	slot2End := slot2Start.Add(60 * time.Minute)
	slot2HoldExpiry := now.Add(4 * time.Minute)

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.padel_slots (id, facility_id, start_time, end_time, status, holder_id, hold_expires_at, price_paisa)
		VALUES ($1, $2, $3, $4, 'HELD', $5, $6, 100000);
	`, slot2ID, facID, slot2Start, slot2End, bobID, slot2HoldExpiry)
	if err != nil {
		t.Fatalf("failed to insert unexpired slot: %v", err)
	}

	// 1. Run worker with 25ms interval
	worker := NewHoldCleanerWorker(repo, CleanerConfig{
		Interval:     25 * time.Millisecond,
		BatchTimeout: 2 * time.Second,
	})

	worker.Start()

	// Wait up to 500ms for background cleaner ticker to sweep
	deadline := time.Now().Add(500 * time.Millisecond)
	var slot1Status string
	var slot1Holder *uuid.UUID
	var slot1Expiry *time.Time

	for time.Now().Before(deadline) {
		err = pool.QueryRow(ctx, "SELECT status, holder_id, hold_expires_at FROM giki_sports.padel_slots WHERE id = $1;", slot1ID).
			Scan(&slot1Status, &slot1Holder, &slot1Expiry)
		if err == nil && slot1Status == "AVAILABLE" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Stop worker gracefully
	worker.Stop()
	if worker.IsRunning() {
		t.Errorf("expected worker to be stopped")
	}

	// Verify slot 1 was updated in real database
	if slot1Status != "AVAILABLE" {
		t.Errorf("expected slot 1 to be transitioned to AVAILABLE by background worker, got %s", slot1Status)
	}
	if slot1Holder != nil || slot1Expiry != nil {
		t.Errorf("expected slot 1 holder and expiry to be cleared, got holder=%v, expiry=%v", slot1Holder, slot1Expiry)
	}

	// Verify slot 2 remains HELD with Bob's active reservation
	var slot2Status string
	var slot2Holder uuid.UUID
	err = pool.QueryRow(ctx, "SELECT status, holder_id FROM giki_sports.padel_slots WHERE id = $1;", slot2ID).
		Scan(&slot2Status, &slot2Holder)
	if err != nil {
		t.Fatalf("failed to query slot 2: %v", err)
	}
	if slot2Status != "HELD" {
		t.Errorf("expected unexpired slot 2 to remain HELD, got %s", slot2Status)
	}
	if slot2Holder != bobID {
		t.Errorf("expected slot 2 to remain held by Bob (%s), got %s", bobID, slot2Holder)
	}

	t.Log("Verified: HoldCleanerWorker automatically released expired hold to AVAILABLE while preserving active unexpired holds.")
}
