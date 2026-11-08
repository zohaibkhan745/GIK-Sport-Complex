package facility

// FacilityResponse represents the public JSON response for a sports facility.
type FacilityResponse struct {
	ID          string             `json:"id"`
	Slug        string             `json:"slug"`
	Name        string             `json:"name"`
	Description *string            `json:"description"`
	Location    *string            `json:"location"`
	Status      string             `json:"status"`
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

	return FacilityResponse{
		ID:          f.ID.String(),
		Slug:        f.Slug,
		Name:        f.Name,
		Description: f.Description,
		Location:    f.Location,
		Status:      string(f.Status),
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
