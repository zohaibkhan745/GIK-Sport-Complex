//go:build integration

package seed_test

import (
	"context"
	"testing"
	"time"

	"giki-sports-backend/internal/seed"
)

func TestSeedCampus_FirstAndRepeatedExecution(t *testing.T) {
	pool, cleanup := setupDisposableDB(t, true)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Initial execution: must insert all 8 campus facilities and their schedules
	summary1, err := seed.ApplyCampus(ctx, pool)
	if err != nil {
		t.Fatalf("first ApplyCampus failed: %v", err)
	}

	if summary1.FacilitiesInserted != 8 {
		t.Errorf("expected 8 facilities inserted, got %d", summary1.FacilitiesInserted)
	}
	if summary1.FacilitiesSkipped != 0 {
		t.Errorf("expected 0 facilities skipped on first run, got %d", summary1.FacilitiesSkipped)
	}
	if summary1.SchedulesInserted != 50 {
		t.Errorf("expected 50 schedules inserted, got %d", summary1.SchedulesInserted)
	}

	// Repeated execution: idempotency check (all 8 skipped, 0 inserted)
	summary2, err := seed.ApplyCampus(ctx, pool)
	if err != nil {
		t.Fatalf("repeated ApplyCampus failed: %v", err)
	}

	if summary2.FacilitiesInserted != 0 {
		t.Errorf("expected 0 facilities inserted on second run, got %d", summary2.FacilitiesInserted)
	}
	if summary2.FacilitiesSkipped != 8 {
		t.Errorf("expected 8 facilities skipped on second run, got %d", summary2.FacilitiesSkipped)
	}
	if summary2.SchedulesInserted != 0 {
		t.Errorf("expected 0 schedules inserted on second run, got %d", summary2.SchedulesInserted)
	}
	if summary2.SchedulesSkipped != 50 {
		t.Errorf("expected 50 schedules skipped on second run, got %d", summary2.SchedulesSkipped)
	}
}
