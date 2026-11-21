package membership

import (
	"math"
	"time"
)

// CalculateTerm calculates the strict 30-day half-open term [T_start, T_start + 30 days).
func CalculateTerm(startTime time.Time) (time.Time, time.Time) {
	return CalculateTermWithDuration(startTime, PassDuration)
}

// CalculateTermWithDuration calculates the term with a customizable plan duration.
func CalculateTermWithDuration(startTime time.Time, duration time.Duration) (time.Time, time.Time) {
	if duration <= 0 {
		duration = PassDuration
	}
	start := startTime.UTC().Truncate(time.Second)
	end := start.Add(duration)
	return start, end
}

// CalculateRenewalTerm enforces the renewal rule (QR-06):
// New expiration adds 30 days to max(T_existing_expiry, T_renewal).
// If the user has an existing active pass expiring in the future, the new pass starts at
// existingExpiry and extends for 30 days, creating an uninterrupted contiguous active window.
// If existingExpiry is nil or in the past (already expired), the new 30-day term starts immediately at renewalTime.
func CalculateRenewalTerm(existingExpiry *time.Time, renewalTime time.Time) (time.Time, time.Time) {
	return CalculateRenewalTermWithDuration(existingExpiry, renewalTime, PassDuration)
}

// CalculateRenewalTermWithDuration enforces the renewal rule with a customizable plan duration.
func CalculateRenewalTermWithDuration(existingExpiry *time.Time, renewalTime time.Time, duration time.Duration) (time.Time, time.Time) {
	if duration <= 0 {
		duration = PassDuration
	}
	renewal := renewalTime.UTC().Truncate(time.Second)

	if existingExpiry == nil || existingExpiry.Before(renewal) {
		start := renewal
		end := start.Add(duration)
		return start, end
	}

	start := existingExpiry.UTC().Truncate(time.Second)
	end := start.Add(duration)
	return start, end
}

// IsActiveAt checks whether a membership is valid at timestamp 'at' according to the
// half-open interval rule [T_start, T_end) (QR-06):
// - Valid 1 second before end_time.
// - Strictly invalid/expired at the exact end_time instant (at >= end_time).
func IsActiveAt(m *Membership, at time.Time) bool {
	if m == nil || m.Status != MembershipStatusActive {
		return false
	}
	t := at.UTC()
	return !t.Before(m.StartTime) && t.Before(m.EndTime)
}

// ComputeMembershipSummary aggregates all user memberships to determine current status,
// effective expiry date, and remaining validity time.
func ComputeMembershipSummary(history []*Membership, now time.Time) UserMembershipSummary {
	currentTime := now.UTC()
	var currentActive *Membership
	var maxExpiry *time.Time

	for _, m := range history {
		if m.Status != MembershipStatusActive {
			continue
		}

		// Check if active right now
		if IsActiveAt(m, currentTime) {
			if currentActive == nil || m.EndTime.After(currentActive.EndTime) {
				currentActive = m
			}
		}

		// Track maximum end_time for all active passes valid into the future
		if m.EndTime.After(currentTime) {
			if maxExpiry == nil || m.EndTime.After(*maxExpiry) {
				exp := m.EndTime
				maxExpiry = &exp
			}
		}
	}

	summary := UserMembershipSummary{
		IsActive:          currentActive != nil,
		CurrentMembership: currentActive,
		EffectiveExpiry:   maxExpiry,
		History:           history,
	}

	if maxExpiry != nil && maxExpiry.After(currentTime) {
		diff := maxExpiry.Sub(currentTime)
		summary.SecondsRemaining = int64(diff.Seconds())
		summary.DaysRemaining = int(math.Ceil(diff.Hours() / 24.0))
	}

	return summary
}
