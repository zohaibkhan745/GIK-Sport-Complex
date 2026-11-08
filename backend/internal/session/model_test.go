package session_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"giki-sports-backend/internal/session"
)

func TestSession_ConstantsAndInvariants(t *testing.T) {
	if session.IdleTimeout != 30*time.Minute {
		t.Errorf("expected IdleTimeout = 30m, got %v", session.IdleTimeout)
	}
	if session.AbsoluteLifetime != 8*time.Hour {
		t.Errorf("expected AbsoluteLifetime = 8h, got %v", session.AbsoluteLifetime)
	}
}

func TestSession_JSONExcludesHash(t *testing.T) {
	s := session.Session{
		ID:                uuid.New(),
		UserID:            uuid.New(),
		CreatedAt:         time.Now().UTC(),
		LastActivityAt:    time.Now().UTC(),
		AbsoluteExpiresAt: time.Now().UTC().Add(session.AbsoluteLifetime),
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("failed to marshal Session: %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, "token_hash") || strings.Contains(jsonStr, "tokenHash") {
		t.Errorf("Session JSON exposed token hash field: %s", jsonStr)
	}
}

func TestSession_RealClock(t *testing.T) {
	clock := session.RealClock{}
	now := clock.Now()
	if now.IsZero() {
		t.Error("expected non-zero time from RealClock")
	}
	if now.Location() != time.UTC {
		t.Errorf("expected UTC location, got %v", now.Location())
	}
}

func TestSession_TestClock(t *testing.T) {
	t0 := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	clock := session.NewTestClock(t0)

	if !clock.Now().Equal(t0) {
		t.Errorf("expected initial time %v, got %v", t0, clock.Now())
	}

	t1 := t0.Add(15 * time.Minute)
	clock.Set(t1)
	if !clock.Now().Equal(t1) {
		t.Errorf("expected time after Set %v, got %v", t1, clock.Now())
	}

	clock.Advance(5 * time.Minute)
	t2 := t1.Add(5 * time.Minute)
	if !clock.Now().Equal(t2) {
		t.Errorf("expected time after Advance %v, got %v", t2, clock.Now())
	}
}

