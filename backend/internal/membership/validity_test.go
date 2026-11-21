package membership

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestPassValidity_HalfOpenInterval_QR06 verifies QR-06 / TC-QR-06:
// A 30-day gym pass spans [T_start, T_start + 30 days) and is valid 1 second before expiry
// and strictly expired at the exact expiry instant.
func TestPassValidity_HalfOpenInterval_QR06(t *testing.T) {
	epoch := time.Date(2026, time.November, 1, 10, 0, 0, 0, time.UTC)
	start, end := CalculateTerm(epoch)

	// Invariant 1: Term duration is exactly 30 days (720 hours = 2,592,000 seconds)
	expectedDuration := 30 * 24 * time.Hour
	actualDuration := end.Sub(start)
	if actualDuration != expectedDuration {
		t.Fatalf("expected pass duration %v, got %v", expectedDuration, actualDuration)
	}

	mbr := &Membership{
		ID:         uuid.New(),
		UserID:     uuid.New(),
		PlanType:   PlanTypeStandard30Day,
		Status:     MembershipStatusActive,
		StartTime:  start,
		EndTime:    end,
		PricePaisa: DefaultGymMembershipFeePaisa,
	}

	clock := NewControlledClock(start)

	// Case 1: 1 second BEFORE start time -> strictly inactive
	clock.Set(start.Add(-1 * time.Second))
	if IsActiveAt(mbr, clock.Now()) {
		t.Fatalf("pass should NOT be active 1 second before start time")
	}

	// Case 2: Exact start time instant -> active (inclusive start)
	clock.Set(start)
	if !IsActiveAt(mbr, clock.Now()) {
		t.Fatalf("pass MUST be active at the exact start instant")
	}

	// Case 3: 15 days into pass -> active
	clock.Set(start.Add(15 * 24 * time.Hour))
	if !IsActiveAt(mbr, clock.Now()) {
		t.Fatalf("pass MUST be active midway through term")
	}

	// Case 4: 1 second BEFORE expiry -> strictly active (QR-06)
	clock.Set(end.Add(-1 * time.Second))
	if !IsActiveAt(mbr, clock.Now()) {
		t.Fatalf("pass MUST be active 1 second before expiry (QR-06)")
	}

	// Case 5: Exact expiry instant -> strictly expired / inactive (QR-06)
	clock.Set(end)
	if IsActiveAt(mbr, clock.Now()) {
		t.Fatalf("pass MUST be expired at the exact expiry instant (QR-06)")
	}

	// Case 6: 1 second AFTER expiry -> strictly inactive
	clock.Set(end.Add(1 * time.Second))
	if IsActiveAt(mbr, clock.Now()) {
		t.Fatalf("pass should NOT be active after expiry")
	}

	// Case 7: Inactive / Cancelled status is never active regardless of timestamp
	mbrCancelled := &Membership{
		ID:        uuid.New(),
		Status:    MembershipStatusCancelled,
		StartTime: start,
		EndTime:   end,
	}
	clock.Set(start.Add(5 * time.Minute))
	if IsActiveAt(mbrCancelled, clock.Now()) {
		t.Fatalf("cancelled membership should never be active")
	}
}

// TestEarlyRenewal_Adds30DaysToExistingExpiry_QR06 verifies QR-06 / TC-QR-06:
// Early renewal adds 30 days to max(T_existing_expiry, T_renewal), NOT current timestamp.
func TestEarlyRenewal_Adds30DaysToExistingExpiry_QR06(t *testing.T) {
	// Active pass from Nov 1 to Dec 1
	initialStart := time.Date(2026, time.November, 1, 12, 0, 0, 0, time.UTC)
	initialEnd := initialStart.Add(30 * 24 * time.Hour) // Dec 1, 12:00:00

	// User renews on Nov 15 (16 days early before initialEnd)
	renewalTime := time.Date(2026, time.November, 15, 9, 30, 0, 0, time.UTC)

	newStart, newEnd := CalculateRenewalTerm(&initialEnd, renewalTime)

	// Verification 1: New pass start matches existing expiry
	if !newStart.Equal(initialEnd) {
		t.Fatalf("expected renewal pass to start at existing expiry %v, got %v", initialEnd, newStart)
	}

	// Verification 2: New pass end adds exactly 30 days onto initialEnd (Dec 31, 12:00:00)
	expectedNewEnd := initialEnd.Add(30 * 24 * time.Hour)
	if !newEnd.Equal(expectedNewEnd) {
		t.Fatalf("expected renewal pass to end at %v, got %v", expectedNewEnd, newEnd)
	}

	// Verification 3: Renewal term duration is exactly 30 days
	if newEnd.Sub(newStart) != 30*24*time.Hour {
		t.Fatalf("renewal term duration must be exactly 30 days, got %v", newEnd.Sub(newStart))
	}

	// Verification 4: Seamless transition across boundary
	pass1 := &Membership{
		Status:    MembershipStatusActive,
		StartTime: initialStart,
		EndTime:   initialEnd,
	}
	pass2 := &Membership{
		Status:    MembershipStatusActive,
		StartTime: newStart,
		EndTime:   newEnd,
	}

	// 1 second before initialEnd: pass1 is active, pass2 is not yet active
	tBoundaryMinus1 := initialEnd.Add(-1 * time.Second)
	if !IsActiveAt(pass1, tBoundaryMinus1) || IsActiveAt(pass2, tBoundaryMinus1) {
		t.Fatalf("at 1s before boundary: pass1 must be active and pass2 inactive")
	}

	// At exact boundary (initialEnd = newStart): pass1 expires, pass2 becomes active seamlessly
	tBoundary := initialEnd
	if IsActiveAt(pass1, tBoundary) {
		t.Fatalf("at boundary: pass1 must be strictly expired")
	}
	if !IsActiveAt(pass2, tBoundary) {
		t.Fatalf("at boundary: pass2 must be strictly active")
	}

	// 1 second before newEnd: pass2 is active
	tNewEndMinus1 := newEnd.Add(-1 * time.Second)
	if !IsActiveAt(pass2, tNewEndMinus1) {
		t.Fatalf("at 1s before newEnd: pass2 must be active")
	}

	// At exact newEnd: pass2 is strictly expired
	if IsActiveAt(pass2, newEnd) {
		t.Fatalf("at exact newEnd: pass2 must be strictly expired")
	}
}

// TestRenewal_WhenExpiredOrFirstPurchase verifies that if the previous pass has already expired
// or does not exist, the new 30-day pass starts immediately at the renewal timestamp.
func TestRenewal_WhenExpiredOrFirstPurchase(t *testing.T) {
	renewalTime := time.Date(2026, time.November, 20, 14, 0, 0, 0, time.UTC)

	// Subtest 1: First purchase (no previous expiry)
	start1, end1 := CalculateRenewalTerm(nil, renewalTime)
	if !start1.Equal(renewalTime) {
		t.Fatalf("first purchase must start at renewal time, got %v", start1)
	}
	if !end1.Equal(renewalTime.Add(30 * 24 * time.Hour)) {
		t.Fatalf("first purchase must end 30 days after renewal time, got %v", end1)
	}

	// Subtest 2: Prior pass expired 5 days ago (Nov 15)
	expiredPassEnd := time.Date(2026, time.November, 15, 10, 0, 0, 0, time.UTC)
	start2, end2 := CalculateRenewalTerm(&expiredPassEnd, renewalTime)
	if !start2.Equal(renewalTime) {
		t.Fatalf("renewal after expiry must start at renewal time, got %v", start2)
	}
	if !end2.Equal(renewalTime.Add(30 * 24 * time.Hour)) {
		t.Fatalf("renewal after expiry must end 30 days after renewal time, got %v", end2)
	}
}

// TestControlledClock_StepByStepTransition verifies time step advancement with ControlledClock.
func TestControlledClock_StepByStepTransition(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(30 * 24 * time.Hour)

	mbr := &Membership{
		Status:    MembershipStatusActive,
		StartTime: start,
		EndTime:   end,
	}

	clock := NewControlledClock(start)

	// Step through: day 0 -> active
	if !IsActiveAt(mbr, clock.Now()) {
		t.Fatalf("expected active at day 0")
	}

	// Advance 29 days, 23 hours, 59 minutes, 59 seconds (1s before expiry)
	clock.Advance(30*24*time.Hour - 1*time.Second)
	if !IsActiveAt(mbr, clock.Now()) {
		t.Fatalf("expected active at 1s before expiry")
	}

	// Advance 1 second (exact expiry)
	clock.Advance(1 * time.Second)
	if IsActiveAt(mbr, clock.Now()) {
		t.Fatalf("expected inactive at exact expiry")
	}
}

// TestComputeMembershipSummary verifies aggregation of active and historical passes.
func TestComputeMembershipSummary(t *testing.T) {
	now := time.Date(2026, time.November, 10, 12, 0, 0, 0, time.UTC)

	// Pass 1: Expired in October
	p1 := &Membership{
		ID:        uuid.New(),
		Status:    MembershipStatusActive,
		StartTime: time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, time.October, 31, 0, 0, 0, 0, time.UTC),
	}

	// Pass 2: Active (Nov 5 to Dec 5)
	p2 := &Membership{
		ID:        uuid.New(),
		Status:    MembershipStatusActive,
		StartTime: time.Date(2026, time.November, 5, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, time.December, 5, 0, 0, 0, 0, time.UTC),
	}

	// Pass 3: Stacked early renewal (Dec 5 to Jan 4)
	p3 := &Membership{
		ID:        uuid.New(),
		Status:    MembershipStatusActive,
		StartTime: time.Date(2026, time.December, 5, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2027, time.January, 4, 0, 0, 0, 0, time.UTC),
	}

	summary := ComputeMembershipSummary([]*Membership{p1, p2, p3}, now)

	if !summary.IsActive {
		t.Fatalf("expected user to be active")
	}
	if summary.CurrentMembership == nil || summary.CurrentMembership.ID != p2.ID {
		t.Fatalf("expected current membership to be p2")
	}
	if summary.EffectiveExpiry == nil || !summary.EffectiveExpiry.Equal(p3.EndTime) {
		t.Fatalf("expected effective expiry to be p3.EndTime (%v), got %v", p3.EndTime, summary.EffectiveExpiry)
	}
	if summary.DaysRemaining < 54 {
		t.Fatalf("expected days remaining to be around 54 days (Nov 10 to Jan 4), got %d", summary.DaysRemaining)
	}
	if len(summary.History) != 3 {
		t.Fatalf("expected 3 history items, got %d", len(summary.History))
	}
}

func TestCalculateTermWithCustomDuration(t *testing.T) {
	epoch := time.Date(2026, time.November, 1, 10, 0, 0, 0, time.UTC)

	// 15-day custom duration
	dur15 := 15 * 24 * time.Hour
	start15, end15 := CalculateTermWithDuration(epoch, dur15)
	if end15.Sub(start15) != dur15 {
		t.Fatalf("expected 15 days, got %v", end15.Sub(start15))
	}

	// 60-day custom duration
	dur60 := 60 * 24 * time.Hour
	start60, end60 := CalculateTermWithDuration(epoch, dur60)
	if end60.Sub(start60) != dur60 {
		t.Fatalf("expected 60 days, got %v", end60.Sub(start60))
	}

	// Renewal with custom 45-day duration
	dur45 := 45 * 24 * time.Hour
	existingExpiry := time.Date(2026, time.November, 20, 10, 0, 0, 0, time.UTC)
	renewalTime := time.Date(2026, time.November, 15, 10, 0, 0, 0, time.UTC)
	startRenew, endRenew := CalculateRenewalTermWithDuration(&existingExpiry, renewalTime, dur45)
	if !startRenew.Equal(existingExpiry) {
		t.Fatalf("expected start of renewal to chain from existingExpiry, got %v", startRenew)
	}
	if endRenew.Sub(startRenew) != dur45 {
		t.Fatalf("expected renewed duration to be 45 days, got %v", endRenew.Sub(startRenew))
	}
}
