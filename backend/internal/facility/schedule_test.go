package facility_test

import (
	"errors"
	"testing"

	"giki-sports-backend/internal/facility"
)

func TestUpdateScheduleInput_Validate(t *testing.T) {
	validInput := facility.UpdateScheduleInput{
		DayOfWeek: 1,
		StartTime: "09:00:00",
		EndTime:   "12:00:00",
		Audience:  "student",
	}

	t.Run("conforming input passes validation", func(t *testing.T) {
		if err := validInput.Validate(); err != nil {
			t.Fatalf("expected nil error for conforming input, got: %v", err)
		}
	})

	t.Run("all valid weekdays 1 through 7 pass", func(t *testing.T) {
		for day := 1; day <= 7; day++ {
			in := validInput
			in.DayOfWeek = day
			if err := in.Validate(); err != nil {
				t.Errorf("expected day %d to be valid, got: %v", day, err)
			}
		}
	})

	t.Run("invalid weekdays are rejected", func(t *testing.T) {
		for _, day := range []int{-1, 0, 8, 99} {
			in := validInput
			in.DayOfWeek = day
			err := in.Validate()
			if err == nil {
				t.Errorf("expected day %d to fail validation", day)
			}
			if !errors.Is(err, facility.ErrInvalidInput) {
				t.Errorf("expected errors.Is(err, facility.ErrInvalidInput) for day %d, got: %v", day, err)
			}
		}
	})

	t.Run("all supported audiences pass", func(t *testing.T) {
		for _, aud := range []string{"general", "student", "faculty", "women_only", "staff"} {
			in := validInput
			in.Audience = aud
			if err := in.Validate(); err != nil {
				t.Errorf("expected audience %q to be valid, got: %v", aud, err)
			}
		}
	})

	t.Run("unsupported audiences are rejected", func(t *testing.T) {
		for _, aud := range []string{"", "admin", "super_admin", "cashier", "unknown", "STUDENT"} {
			in := validInput
			in.Audience = aud
			err := in.Validate()
			if err == nil {
				t.Errorf("expected audience %q to fail validation", aud)
			}
			if !errors.Is(err, facility.ErrInvalidInput) {
				t.Errorf("expected errors.Is(err, facility.ErrInvalidInput) for audience %q, got: %v", aud, err)
			}
		}
	})

	t.Run("allows 24:00:00 as end time only", func(t *testing.T) {
		in := validInput
		in.StartTime = "18:00:00"
		in.EndTime = "24:00:00"
		if err := in.Validate(); err != nil {
			t.Fatalf("expected 24:00:00 to be valid end time, got: %v", err)
		}
	})

	t.Run("rejects 24:00:00 as start time", func(t *testing.T) {
		in := validInput
		in.StartTime = "24:00:00"
		in.EndTime = "24:00:00"
		err := in.Validate()
		if err == nil {
			t.Fatal("expected 24:00:00 start time to fail validation")
		}
		if !errors.Is(err, facility.ErrInvalidInput) {
			t.Fatalf("expected errors.Is(err, facility.ErrInvalidInput), got: %v", err)
		}
	})

	t.Run("rejects start time equal to end time", func(t *testing.T) {
		in := validInput
		in.StartTime = "10:00:00"
		in.EndTime = "10:00:00"
		err := in.Validate()
		if err == nil {
			t.Fatal("expected equal start and end time to fail validation")
		}
		if !errors.Is(err, facility.ErrInvalidInput) {
			t.Fatalf("expected errors.Is(err, facility.ErrInvalidInput), got: %v", err)
		}
	})

	t.Run("rejects start time after end time", func(t *testing.T) {
		in := validInput
		in.StartTime = "15:00:00"
		in.EndTime = "10:00:00"
		err := in.Validate()
		if err == nil {
			t.Fatal("expected inverted start and end time to fail validation")
		}
		if !errors.Is(err, facility.ErrInvalidInput) {
			t.Fatalf("expected errors.Is(err, facility.ErrInvalidInput), got: %v", err)
		}
	})

	t.Run("rejects malformed time strings", func(t *testing.T) {
		malformedTimes := []string{
			"9:00",
			"09:00",
			"9:00:00",
			"25:00:00",
			"12:60:00",
			"12:00:60",
			"invalid",
			"12:00:00PM",
			"12-00-00",
			"",
		}

		for _, mt := range malformedTimes {
			// Test malformed start time
			inStart := validInput
			inStart.StartTime = mt
			if err := inStart.Validate(); err == nil {
				t.Errorf("expected malformed start time %q to fail validation", mt)
			} else if !errors.Is(err, facility.ErrInvalidInput) {
				t.Errorf("expected errors.Is(err, facility.ErrInvalidInput) for start time %q, got: %v", mt, err)
			}

			// Test malformed end time
			inEnd := validInput
			inEnd.EndTime = mt
			if err := inEnd.Validate(); err == nil {
				t.Errorf("expected malformed end time %q to fail validation", mt)
			} else if !errors.Is(err, facility.ErrInvalidInput) {
				t.Errorf("expected errors.Is(err, facility.ErrInvalidInput) for end time %q, got: %v", mt, err)
			}
		}
	})
}

func TestCreateScheduleInput_Validate(t *testing.T) {
	validInput := facility.CreateScheduleInput{
		DayOfWeek: 1,
		StartTime: "11:00:00",
		EndTime:   "13:00:00",
		Audience:  "student",
	}

	t.Run("conforming input passes validation", func(t *testing.T) {
		if err := validInput.Validate(); err != nil {
			t.Fatalf("expected nil error for conforming input, got: %v", err)
		}
	})

	t.Run("all valid weekdays 1 through 7 pass", func(t *testing.T) {
		for day := 1; day <= 7; day++ {
			in := validInput
			in.DayOfWeek = day
			if err := in.Validate(); err != nil {
				t.Errorf("expected day %d to be valid, got: %v", day, err)
			}
		}
	})

	t.Run("invalid weekdays are rejected", func(t *testing.T) {
		for _, day := range []int{-1, 0, 8, 99} {
			in := validInput
			in.DayOfWeek = day
			err := in.Validate()
			if err == nil {
				t.Errorf("expected day %d to fail validation", day)
			}
			if !errors.Is(err, facility.ErrInvalidInput) {
				t.Errorf("expected errors.Is(err, facility.ErrInvalidInput) for day %d, got: %v", day, err)
			}
		}
	})

	t.Run("all supported audiences pass", func(t *testing.T) {
		for _, aud := range []string{"general", "student", "faculty", "women_only", "staff"} {
			in := validInput
			in.Audience = aud
			if err := in.Validate(); err != nil {
				t.Errorf("expected audience %q to be valid, got: %v", aud, err)
			}
		}
	})

	t.Run("unsupported audiences are rejected", func(t *testing.T) {
		for _, aud := range []string{"", "admin", "super_admin", "coach", "invalid"} {
			in := validInput
			in.Audience = aud
			err := in.Validate()
			if err == nil {
				t.Errorf("expected audience %q to fail validation", aud)
			}
			if !errors.Is(err, facility.ErrInvalidInput) {
				t.Errorf("expected errors.Is(err, facility.ErrInvalidInput) for audience %q, got: %v", aud, err)
			}
		}
	})

	t.Run("allows 24:00:00 as end time only", func(t *testing.T) {
		in := validInput
		in.StartTime = "20:00:00"
		in.EndTime = "24:00:00"
		if err := in.Validate(); err != nil {
			t.Fatalf("expected 24:00:00 to be valid end time, got: %v", err)
		}
	})

	t.Run("rejects 24:00:00 as start time", func(t *testing.T) {
		in := validInput
		in.StartTime = "24:00:00"
		in.EndTime = "24:00:00"
		err := in.Validate()
		if err == nil {
			t.Fatal("expected 24:00:00 start time to fail validation")
		}
		if !errors.Is(err, facility.ErrInvalidInput) {
			t.Fatalf("expected errors.Is(err, facility.ErrInvalidInput), got: %v", err)
		}
	})

	t.Run("rejects start time equal to or after end time", func(t *testing.T) {
		inEqual := validInput
		inEqual.StartTime = "12:00:00"
		inEqual.EndTime = "12:00:00"
		if err := inEqual.Validate(); !errors.Is(err, facility.ErrInvalidInput) {
			t.Errorf("expected equal start/end to fail validation, got: %v", err)
		}

		inInverted := validInput
		inInverted.StartTime = "15:00:00"
		inInverted.EndTime = "14:00:00"
		if err := inInverted.Validate(); !errors.Is(err, facility.ErrInvalidInput) {
			t.Errorf("expected inverted start/end to fail validation, got: %v", err)
		}
	})
}

