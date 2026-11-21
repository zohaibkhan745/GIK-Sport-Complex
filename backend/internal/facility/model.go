package facility

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is a sentinel error returned when a requested facility does not exist.
var ErrNotFound = errors.New("facility not found")

// ErrDuplicateFacility is a sentinel error returned when a facility slug or name collides with an existing facility.
var ErrDuplicateFacility = errors.New("facility already exists")

// ErrFacilityValidation is a sentinel error returned when facility creation or update parameters fail validation.
var ErrFacilityValidation = errors.New("invalid facility input")

// NotFoundError provides structured error context for an unresolvable facility slug.
type NotFoundError struct {
	Slug string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("facility with slug %q not found", e.Slug)
}

func (e *NotFoundError) Is(target error) bool {
	return target == ErrNotFound
}

// DuplicateFacilityError provides structured error context when a facility creation collides with an existing facility.
type DuplicateFacilityError struct {
	Field string
	Value string
}

func (e *DuplicateFacilityError) Error() string {
	return fmt.Sprintf("facility with %s %q already exists", e.Field, e.Value)
}

func (e *DuplicateFacilityError) Is(target error) bool {
	return target == ErrDuplicateFacility
}

// FacilityValidationError provides structured error context for invalid facility parameters.
type FacilityValidationError struct {
	Message string
}

func (e *FacilityValidationError) Error() string {
	return fmt.Sprintf("invalid facility: %s", e.Message)
}

func (e *FacilityValidationError) Is(target error) bool {
	return target == ErrFacilityValidation || target == ErrInvalidInput
}

// OperationalStatus represents the current availability state of a facility.
type OperationalStatus string

const (
	StatusActive      OperationalStatus = "active"
	StatusMaintenance OperationalStatus = "maintenance"
	StatusClosed      OperationalStatus = "closed"
	StatusReserved    OperationalStatus = "reserved"
)

// IsValidStatus returns true if the status is one of the supported operational statuses.
func IsValidStatus(status OperationalStatus) bool {
	return status == StatusActive || status == StatusMaintenance || status == StatusClosed || status == StatusReserved
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Slugify converts an arbitrary string into a URL-friendly slug.
func Slugify(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	b.Grow(len(s))
	lastWasHyphen := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastWasHyphen = false
		} else if r == ' ' || r == '-' || r == '_' || r == '/' {
			if !lastWasHyphen && b.Len() > 0 {
				b.WriteRune('-')
				lastWasHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// CreateFacilityInput defines the parameters for creating a new facility.
type CreateFacilityInput struct {
	Name        string            `json:"name"`
	Slug        string            `json:"slug"`
	Description *string           `json:"description,omitempty"`
	Status      OperationalStatus `json:"status"`
	StatusUntil *time.Time        `json:"statusUntil,omitempty"`
	StatusNote  *string           `json:"statusNote,omitempty"`
	ReservedFor *string           `json:"reservedFor,omitempty"`
}

// Validate verifies that the proposed facility parameters conform to business and database constraints.
func (in *CreateFacilityInput) Validate() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return &FacilityValidationError{Message: "name is required and cannot be empty"}
	}
	if len(in.Name) > 255 {
		return &FacilityValidationError{Message: "name cannot exceed 255 characters"}
	}

	in.Slug = strings.TrimSpace(strings.ToLower(in.Slug))
	if in.Slug == "" {
		in.Slug = Slugify(in.Name)
	}
	if in.Slug == "" {
		return &FacilityValidationError{Message: "slug could not be generated from name; please provide a valid slug"}
	}
	if len(in.Slug) > 100 {
		return &FacilityValidationError{Message: "slug cannot exceed 100 characters"}
	}
	if !slugPattern.MatchString(in.Slug) {
		return &FacilityValidationError{Message: "slug must contain only lowercase alphanumeric characters separated by single hyphens"}
	}

	if in.Status == "" {
		in.Status = StatusActive
	} else if !IsValidStatus(in.Status) {
		return &FacilityValidationError{Message: fmt.Sprintf("invalid status %q; must be 'active', 'maintenance', 'closed', or 'reserved'", in.Status)}
	}

	if in.Description != nil {
		trimmed := strings.TrimSpace(*in.Description)
		if len(trimmed) > 2000 {
			return &FacilityValidationError{Message: "description cannot exceed 2000 characters"}
		}
		if trimmed == "" {
			in.Description = nil
		} else {
			in.Description = &trimmed
		}
	}

	if in.StatusNote != nil {
		trimmed := strings.TrimSpace(*in.StatusNote)
		if len(trimmed) > 1000 {
			return &FacilityValidationError{Message: "status note cannot exceed 1000 characters"}
		}
		if trimmed == "" {
			in.StatusNote = nil
		} else {
			in.StatusNote = &trimmed
		}
	}

	if in.ReservedFor != nil {
		trimmed := strings.TrimSpace(*in.ReservedFor)
		if len(trimmed) > 255 {
			return &FacilityValidationError{Message: "reserved for cannot exceed 255 characters"}
		}
		if trimmed == "" {
			in.ReservedFor = nil
		} else {
			in.ReservedFor = &trimmed
		}
	}

	return nil
}

// UpdateFacilityInput defines the modifiable parameters of an existing facility.
type UpdateFacilityInput struct {
	Name             *string            `json:"name,omitempty"`
	Description      *string            `json:"description,omitempty"`
	Status           *OperationalStatus `json:"status,omitempty"`
	StatusUntil      *time.Time         `json:"statusUntil,omitempty"`
	ClearStatusUntil bool               `json:"clearStatusUntil,omitempty"`
	StatusNote       *string            `json:"statusNote,omitempty"`
	ClearStatusNote  bool               `json:"clearStatusNote,omitempty"`
	ReservedFor      *string            `json:"reservedFor,omitempty"`
	ClearReservedFor bool               `json:"clearReservedFor,omitempty"`
}

// Validate verifies that the proposed facility updates conform to business and database constraints.
func (in *UpdateFacilityInput) Validate() error {
	hasUpdate := false

	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		if trimmed == "" {
			return &FacilityValidationError{Message: "name cannot be empty"}
		}
		if len(trimmed) > 255 {
			return &FacilityValidationError{Message: "name cannot exceed 255 characters"}
		}
		in.Name = &trimmed
		hasUpdate = true
	}

	if in.Status != nil {
		if !IsValidStatus(*in.Status) {
			return &FacilityValidationError{Message: fmt.Sprintf("invalid status %q; must be 'active', 'maintenance', 'closed', or 'reserved'", *in.Status)}
		}
		hasUpdate = true
	}

	if in.Description != nil {
		trimmed := strings.TrimSpace(*in.Description)
		if len(trimmed) > 2000 {
			return &FacilityValidationError{Message: "description cannot exceed 2000 characters"}
		}
		if trimmed == "" {
			in.Description = nil
		} else {
			in.Description = &trimmed
		}
		hasUpdate = true
	}

	if in.StatusUntil != nil || in.ClearStatusUntil {
		hasUpdate = true
	}

	if in.StatusNote != nil {
		trimmed := strings.TrimSpace(*in.StatusNote)
		if len(trimmed) > 1000 {
			return &FacilityValidationError{Message: "status note cannot exceed 1000 characters"}
		}
		if trimmed == "" {
			in.StatusNote = nil
			in.ClearStatusNote = true
		} else {
			in.StatusNote = &trimmed
		}
		hasUpdate = true
	} else if in.ClearStatusNote {
		hasUpdate = true
	}

	if in.ReservedFor != nil {
		trimmed := strings.TrimSpace(*in.ReservedFor)
		if len(trimmed) > 255 {
			return &FacilityValidationError{Message: "reserved for cannot exceed 255 characters"}
		}
		if trimmed == "" {
			in.ReservedFor = nil
			in.ClearReservedFor = true
		} else {
			in.ReservedFor = &trimmed
		}
		hasUpdate = true
	} else if in.ClearReservedFor {
		hasUpdate = true
	}

	if !hasUpdate {
		return &FacilityValidationError{Message: "at least one field (name, status, description, statusUntil, statusNote, or reservedFor) must be provided"}
	}

	return nil
}

// Facility is the domain read model for a sports complex facility.
// It matches the relational schema in giki_sports.facilities.
type Facility struct {
	ID          uuid.UUID         `json:"id"`
	Slug        string            `json:"slug"`
	Name        string            `json:"name"`
	Description *string           `json:"description,omitempty"`
	Status      OperationalStatus `json:"status"`
	StatusUntil *time.Time        `json:"statusUntil,omitempty"`
	StatusNote  *string           `json:"statusNote,omitempty"`
	ReservedFor *string           `json:"reservedFor,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	Schedules   []Schedule        `json:"schedules"`
}

// EffectiveStatus resolves the operational status, auto-reverting non-active statuses
// to StatusActive when statusUntil has elapsed.
func (f *Facility) EffectiveStatus() OperationalStatus {
	if f.Status != StatusActive && f.StatusUntil != nil && !time.Now().Before(*f.StatusUntil) {
		return StatusActive
	}
	return f.Status
}

// Schedule is the domain read model for a recurring weekly operating window.
// Times are local wall-clock representations in canonical Asia/Karachi (PKT, UTC+5) timezone.
// They use string "HH:MM:SS" format to preserve exact wall-clock values (including "24:00:00")
// without attaching arbitrary calendar dates or timezone offsets.
type Schedule struct {
	ID         uuid.UUID `json:"id"`
	FacilityID uuid.UUID `json:"facilityId"`
	DayOfWeek  int       `json:"dayOfWeek"` // ISO-8601 numeric weekday: 1 = Monday ... 7 = Sunday
	StartTime  string    `json:"startTime"` // 24-hour local format: "HH:MM:SS" (e.g. "06:00:00")
	EndTime    string    `json:"endTime"`   // 24-hour local format: "HH:MM:SS" (e.g. "24:00:00")
	Audience   string    `json:"audience"`  // Provisional contract: "general", "student", "faculty", "women_only", "staff"
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// ErrInvalidInput is a sentinel error returned when schedule update parameters fail data contract validation.
var ErrInvalidInput = errors.New("invalid schedule window input")

// ErrScheduleNotFound is a sentinel error returned when no schedule matches the provided facility slug and schedule ID.
var ErrScheduleNotFound = errors.New("schedule not found for facility")

// ErrDuplicateSchedule is a sentinel error returned when the proposed window duplicates an existing window for the facility.
var ErrDuplicateSchedule = errors.New("duplicate schedule window for facility")

// ScheduleValidationError provides structured error context for invalid schedule parameters.
type ScheduleValidationError struct {
	Message string
}

func (e *ScheduleValidationError) Error() string {
	return fmt.Sprintf("invalid schedule window: %s", e.Message)
}

func (e *ScheduleValidationError) Is(target error) bool {
	return target == ErrInvalidInput
}

// ScheduleNotFoundError provides structured error context when a facility slug and schedule ID pair cannot be resolved.
type ScheduleNotFoundError struct {
	FacilitySlug string
	ScheduleID   uuid.UUID
}

func (e *ScheduleNotFoundError) Error() string {
	return fmt.Sprintf("schedule %s not found for facility %q", e.ScheduleID, e.FacilitySlug)
}

func (e *ScheduleNotFoundError) Is(target error) bool {
	return target == ErrScheduleNotFound
}

// DuplicateScheduleError provides structured error context when an update collides with an existing window.
type DuplicateScheduleError struct {
	FacilitySlug string
	ScheduleID   uuid.UUID
	DayOfWeek    int
	StartTime    string
	EndTime      string
	Audience     string
}

func (e *DuplicateScheduleError) Error() string {
	return fmt.Sprintf("schedule window duplicates an existing window for facility %q (day %d, %s-%s, %s)",
		e.FacilitySlug, e.DayOfWeek, e.StartTime, e.EndTime, e.Audience)
}

func (e *DuplicateScheduleError) Is(target error) bool {
	return target == ErrDuplicateSchedule
}

// SupportedAudiences defines the allowable audience values for campus cohorts and GIKI varsity teams.
var SupportedAudiences = map[string]struct{}{
	// Campus Cohorts
	"general":           {},
	"student":           {},
	"faculty":           {},
	"women_only":        {},
	"staff":             {},
	// GIKI Sports Teams & Dedicated Cohorts
	"badminton_team":    {},
	"basketball_boys":   {},
	"basketball_girls":  {},
	"volleyball_boys":   {},
	"volleyball_girls":  {},
	"cricket_team":      {},
	"football_team":     {},
	"tennis_team":       {},
	"squash_team":       {},
	"squash_faculty":    {},
	"table_tennis_team": {},
}

// UpdateScheduleInput defines the modifiable properties of a weekly operating schedule window.
type UpdateScheduleInput struct {
	DayOfWeek int    `json:"dayOfWeek"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Audience  string `json:"audience"`
}

// CreateScheduleInput defines the input parameters for creating a new weekly operating schedule window.
type CreateScheduleInput struct {
	DayOfWeek int    `json:"dayOfWeek"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Audience  string `json:"audience"`
}

// Validate verifies that the proposed schedule window conforms strictly to data contract rules.
func (in CreateScheduleInput) Validate() error {
	return UpdateScheduleInput{
		DayOfWeek: in.DayOfWeek,
		StartTime: in.StartTime,
		EndTime:   in.EndTime,
		Audience:  in.Audience,
	}.Validate()
}

// AuditParams captures actor and network context required for administrative audit logging (FR-09).
type AuditParams struct {
	ActorID   uuid.UUID `json:"actorId"`
	IPAddress string    `json:"ipAddress"`
	UserAgent string    `json:"userAgent"`
}

// Validate verifies that the proposed schedule window conforms strictly to data contract rules.
func (in UpdateScheduleInput) Validate() error {
	if in.DayOfWeek < 1 || in.DayOfWeek > 7 {
		return &ScheduleValidationError{
			Message: fmt.Sprintf("dayOfWeek must be between 1 (Monday) and 7 (Sunday), got %d", in.DayOfWeek),
		}
	}

	if _, ok := SupportedAudiences[in.Audience]; !ok {
		return &ScheduleValidationError{
			Message: fmt.Sprintf("unsupported audience %q; allowed: general, student, faculty, women_only, staff", in.Audience),
		}
	}

	startSec, err := ParseWallClockSeconds(in.StartTime, false)
	if err != nil {
		return &ScheduleValidationError{
			Message: fmt.Sprintf("invalid start_time: %v", err),
		}
	}

	endSec, err := ParseWallClockSeconds(in.EndTime, true)
	if err != nil {
		return &ScheduleValidationError{
			Message: fmt.Sprintf("invalid end_time: %v", err),
		}
	}

	if startSec >= endSec {
		return &ScheduleValidationError{
			Message: fmt.Sprintf("start_time (%s) must be strictly before end_time (%s)", in.StartTime, in.EndTime),
		}
	}

	return nil
}

// ParseWallClockSeconds parses a 24-hour wall-clock time string formatted as HH:MM:SS.
// When allow24 is true, "24:00:00" is recognized as end of day (86400 seconds).
func ParseWallClockSeconds(t string, allow24 bool) (int, error) {
	if len(t) != 8 || t[2] != ':' || t[5] != ':' {
		return 0, fmt.Errorf("must be formatted as HH:MM:SS (got %q)", t)
	}
	h1, h2 := t[0], t[1]
	m1, m2 := t[3], t[4]
	s1, s2 := t[6], t[7]
	if h1 < '0' || h1 > '9' || h2 < '0' || h2 > '9' ||
		m1 < '0' || m1 > '9' || m2 < '0' || m2 > '9' ||
		s1 < '0' || s1 > '9' || s2 < '0' || s2 > '9' {
		return 0, fmt.Errorf("must contain only numeric digits in HH:MM:SS (got %q)", t)
	}
	h := int(h1-'0')*10 + int(h2-'0')
	m := int(m1-'0')*10 + int(m2-'0')
	s := int(s1-'0')*10 + int(s2-'0')

	if allow24 && t == "24:00:00" {
		return 86400, nil
	}

	if h < 0 || h > 23 {
		return 0, fmt.Errorf("hour must be between 00 and 23 (got %02d)", h)
	}
	if m < 0 || m > 59 {
		return 0, fmt.Errorf("minute must be between 00 and 59 (got %02d)", m)
	}
	if s < 0 || s > 59 {
		return 0, fmt.Errorf("second must be between 00 and 59 (got %02d)", s)
	}

	return h*3600 + m*60 + s, nil
}
