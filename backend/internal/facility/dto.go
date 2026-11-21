package facility

import "time"

// FacilityResponse represents the public JSON response for a sports facility.
type FacilityResponse struct {
	ID          string             `json:"id"`
	Slug        string             `json:"slug"`
	Name        string             `json:"name"`
	Description *string            `json:"description"`
	Status      string             `json:"status"`
	StatusUntil *string            `json:"statusUntil,omitempty"`
	StatusNote  *string            `json:"statusNote,omitempty"`
	ReservedFor *string            `json:"reservedFor,omitempty"`
	Schedules   []ScheduleResponse `json:"schedules"`
}

// ScheduleResponse represents the public JSON response for a recurring weekly operating window.
type ScheduleResponse struct {
	ID        string `json:"id"`
	DayOfWeek int    `json:"dayOfWeek"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Audience  string `json:"audience"`
}

// ToScheduleResponse converts a domain Schedule into an external ScheduleResponse DTO.
// Preserves exact wall-clock string representations, including "24:00:00".
func ToScheduleResponse(s Schedule) ScheduleResponse {
	return ScheduleResponse{
		ID:        s.ID.String(),
		DayOfWeek: s.DayOfWeek,
		StartTime: s.StartTime,
		EndTime:   s.EndTime,
		Audience:  s.Audience,
	}
}

// ErrorResponse represents a standardized, safe JSON error payload.
type ErrorResponse struct {
	Error string `json:"error"`
}

// ToFacilityResponse converts a domain Facility into an external FacilityResponse DTO.
// Ensures that Schedules is always a non-nil slice ([]).
// If a non-active status had a statusUntil time that has elapsed, it resolves to "active".
func ToFacilityResponse(f Facility) FacilityResponse {
	schedules := make([]ScheduleResponse, 0, len(f.Schedules))
	for _, s := range f.Schedules {
		schedules = append(schedules, ScheduleResponse{
			ID:        s.ID.String(),
			DayOfWeek: s.DayOfWeek,
			StartTime: s.StartTime,
			EndTime:   s.EndTime,
			Audience:  s.Audience,
		})
	}

	effStatus := f.Status
	var statusUntil *string
	var statusNote *string
	var reservedFor *string

	if f.Status != StatusActive && f.StatusUntil != nil && !time.Now().Before(*f.StatusUntil) {
		effStatus = StatusActive
	} else {
		if f.StatusUntil != nil {
			formatted := f.StatusUntil.UTC().Format(time.RFC3339)
			statusUntil = &formatted
		}
		statusNote = f.StatusNote
		reservedFor = f.ReservedFor
	}

	return FacilityResponse{
		ID:          f.ID.String(),
		Slug:        f.Slug,
		Name:        f.Name,
		Description: f.Description,
		Status:      string(effStatus),
		StatusUntil: statusUntil,
		StatusNote:  statusNote,
		ReservedFor: reservedFor,
		Schedules:   schedules,
	}
}

// ToFacilityResponses converts a slice of domain Facilities into external FacilityResponse DTOs.
// Guarantees a non-nil empty slice ([]) if input is empty or nil.
func ToFacilityResponses(facilities []Facility) []FacilityResponse {
	res := make([]FacilityResponse, 0, len(facilities))
	for _, f := range facilities {
		res = append(res, ToFacilityResponse(f))
	}
	return res
}
