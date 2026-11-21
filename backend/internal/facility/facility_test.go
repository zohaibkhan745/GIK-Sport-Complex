package facility_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"giki-sports-backend/internal/facility"
)

func TestSlugify(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Gymnasium", "gymnasium"},
		{"Table Tennis", "table-tennis"},
		{"Floodlit Futsal Pitch (Ground A)", "floodlit-futsal-pitch-ground-a"},
		{"Padel Court #1 - VIP", "padel-court-1-vip"},
		{"   Multi / Purpose  Hall   ", "multi-purpose-hall"},
		{"---Leading-and-Trailing---", "leading-and-trailing"},
		{"", ""},
		{"123 Numbers 456", "123-numbers-456"},
	}

	for _, c := range cases {
		got := facility.Slugify(c.input)
		if got != c.expected {
			t.Errorf("Slugify(%q) = %q; expected %q", c.input, got, c.expected)
		}
	}
}

func TestCreateFacilityInput_Validate(t *testing.T) {
	t.Run("valid input with explicit slug", func(t *testing.T) {
		in := facility.CreateFacilityInput{
			Name:   "Squash Court",
			Slug:   "squash-court",
			Status: facility.StatusActive,
		}
		if err := in.Validate(); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if in.Slug != "squash-court" {
			t.Errorf("expected slug 'squash-court', got %q", in.Slug)
		}
	})

	t.Run("auto-generates slug when blank", func(t *testing.T) {
		in := facility.CreateFacilityInput{
			Name: "Olympic Swimming Pool",
		}
		if err := in.Validate(); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if in.Slug != "olympic-swimming-pool" {
			t.Errorf("expected auto-generated slug 'olympic-swimming-pool', got %q", in.Slug)
		}
		if in.Status != facility.StatusActive {
			t.Errorf("expected default status 'active', got %q", in.Status)
		}
	})

	t.Run("rejects empty name", func(t *testing.T) {
		in := facility.CreateFacilityInput{
			Name: "   ",
		}
		err := in.Validate()
		if err == nil {
			t.Fatal("expected error for empty name, got nil")
		}
		if !errors.Is(err, facility.ErrFacilityValidation) {
			t.Errorf("expected ErrFacilityValidation, got %v", err)
		}
	})

	t.Run("rejects oversized name", func(t *testing.T) {
		in := facility.CreateFacilityInput{
			Name: strings.Repeat("A", 256),
		}
		if err := in.Validate(); err == nil {
			t.Fatal("expected error for name > 255 chars")
		}
	})

	t.Run("rejects invalid status", func(t *testing.T) {
		in := facility.CreateFacilityInput{
			Name:   "Arena",
			Status: "invalid_status",
		}
		if err := in.Validate(); err == nil {
			t.Fatal("expected error for invalid status")
		}
	})

	t.Run("rejects invalid slug format", func(t *testing.T) {
		in := facility.CreateFacilityInput{
			Name: "Arena",
			Slug: "Invalid--Slug!!",
		}
		if err := in.Validate(); err == nil {
			t.Fatal("expected error for malformed slug")
		}
	})

	t.Run("trims description", func(t *testing.T) {
		desc := "  A great facility for athletes  "
		in := facility.CreateFacilityInput{
			Name:        "Arena",
			Description: &desc,
		}
		if err := in.Validate(); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if *in.Description != "A great facility for athletes" {
			t.Errorf("expected trimmed description, got %q", *in.Description)
		}
	})
}

func TestUpdateFacilityInput_Validate(t *testing.T) {
	t.Run("valid partial update with name", func(t *testing.T) {
		name := "New Arena Name"
		in := facility.UpdateFacilityInput{
			Name: &name,
		}
		if err := in.Validate(); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("valid update with all fields", func(t *testing.T) {
		name := "New Arena Name"
		st := facility.StatusMaintenance
		desc := "Upgraded turf"
		in := facility.UpdateFacilityInput{
			Name:        &name,
			Status:      &st,
			Description: &desc,
		}
		if err := in.Validate(); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("rejects empty update with no fields", func(t *testing.T) {
		in := facility.UpdateFacilityInput{}
		if err := in.Validate(); err == nil {
			t.Fatal("expected error for empty update")
		}
	})

	t.Run("rejects empty name update", func(t *testing.T) {
		empty := "   "
		in := facility.UpdateFacilityInput{
			Name: &empty,
		}
		if err := in.Validate(); err == nil {
			t.Fatal("expected error for empty name update")
		}
	})

	t.Run("rejects invalid status", func(t *testing.T) {
		invalid := facility.OperationalStatus("pending")
		in := facility.UpdateFacilityInput{
			Status: &invalid,
		}
		if err := in.Validate(); err == nil {
			t.Fatal("expected error for invalid status")
		}
	})

	t.Run("accepts reserved status and time-bound metadata", func(t *testing.T) {
		status := facility.StatusReserved
		note := "ACM Hackathon"
		reservedFor := "ACM Society"
		in := facility.UpdateFacilityInput{
			Status:      &status,
			StatusNote:  &note,
			ReservedFor: &reservedFor,
		}
		if err := in.Validate(); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})
}

func TestFacility_AutoExpiration(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(2 * time.Hour)

	t.Run("auto-reverts reserved status to active when statusUntil has elapsed", func(t *testing.T) {
		note := "Netronix Gaming Event"
		resFor := "Netronix"
		f := facility.Facility{
			ID:          uuid.New(),
			Slug:        "gymnasium",
			Name:        "Gymnasium",
			Status:      facility.StatusReserved,
			StatusUntil: &past,
			StatusNote:  &note,
			ReservedFor: &resFor,
		}

		if f.EffectiveStatus() != facility.StatusActive {
			t.Errorf("expected EffectiveStatus to be %q, got %q", facility.StatusActive, f.EffectiveStatus())
		}

		dto := facility.ToFacilityResponse(f)
		if dto.Status != "active" {
			t.Errorf("expected DTO status to be 'active', got %q", dto.Status)
		}
		if dto.StatusUntil != nil {
			t.Errorf("expected expired statusUntil to be omitted, got %v", *dto.StatusUntil)
		}
	})

	t.Run("retains reserved status when statusUntil is in the future", func(t *testing.T) {
		note := "Netronix Gaming Event"
		resFor := "Netronix"
		f := facility.Facility{
			ID:          uuid.New(),
			Slug:        "gymnasium",
			Name:        "Gymnasium",
			Status:      facility.StatusReserved,
			StatusUntil: &future,
			StatusNote:  &note,
			ReservedFor: &resFor,
		}

		if f.EffectiveStatus() != facility.StatusReserved {
			t.Errorf("expected EffectiveStatus to be %q, got %q", facility.StatusReserved, f.EffectiveStatus())
		}

		dto := facility.ToFacilityResponse(f)
		if dto.Status != "reserved" {
			t.Errorf("expected DTO status to be 'reserved', got %q", dto.Status)
		}
		if dto.StatusUntil == nil {
			t.Error("expected statusUntil to be present")
		}
		if dto.ReservedFor == nil || *dto.ReservedFor != "Netronix" {
			t.Errorf("expected reservedFor 'Netronix', got %v", dto.ReservedFor)
		}
	})

	t.Run("auto-reverts maintenance to active when statusUntil has elapsed", func(t *testing.T) {
		f := facility.Facility{
			ID:          uuid.New(),
			Slug:        "squash-court",
			Name:        "Squash Court",
			Status:      facility.StatusMaintenance,
			StatusUntil: &past,
		}

		if f.EffectiveStatus() != facility.StatusActive {
			t.Errorf("expected EffectiveStatus to be %q, got %q", facility.StatusActive, f.EffectiveStatus())
		}
	})
}
