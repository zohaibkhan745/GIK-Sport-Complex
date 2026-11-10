package content

import (
	"time"

	"github.com/google/uuid"
)

// PublicContentResponse represents the public, sanitized JSON payload for announcements and achievements.
// It exposes only public-facing fields and intentionally excludes creator identity, draft status,
// internal timestamps (created_at, updated_at), and audit details.
type PublicContentResponse struct {
	ID          uuid.UUID `json:"id"`
	Kind        string    `json:"kind"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	PublishedAt time.Time `json:"publishedAt"`
}

// CreateDraftRequest defines the JSON payload for creating a new draft content item.
// Incomplete drafts are valid; title and content may be omitted or null.
// Unexpected fields (e.g. actor, status, published_at) are strictly rejected.
type CreateDraftRequest struct {
	Kind    string  `json:"kind"`
	Title   *string `json:"title"`
	Content *string `json:"content"`
}

// UpdateDraftRequest defines the JSON payload for updating an existing draft's title and content.
// PUT replaces both fields; omitted or null values clear that field (set to NULL in storage).
// Preserves kind, creator, and creation time.
type UpdateDraftRequest struct {
	Title   *string `json:"title"`
	Content *string `json:"content"`
}

// DraftResponse represents the administrative JSON response for an internal content draft.
type DraftResponse struct {
	ID          uuid.UUID  `json:"id"`
	Kind        string     `json:"kind"`
	Title       *string    `json:"title"`
	Content     *string    `json:"content"`
	Status      string     `json:"status"`
	CreatorID   uuid.UUID  `json:"creatorId"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
}

// DraftSummaryResponse represents the compact administrative JSON payload for a draft list item.
// It exposes only the minimal metadata required for listing (id, kind, title, updatedAt) and
// intentionally excludes the full content body, creator details, and internal audit fields.
// Full draft details remain accessible via GET /api/admin/content/drafts/:id.
// Initial listing returns at most 50 items; pagination is deferred to future work.
type DraftSummaryResponse struct {
	ID        uuid.UUID `json:"id"`
	Kind      string    `json:"kind"`
	Title     *string   `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ErrorResponse represents a standardized, safe JSON error payload.
type ErrorResponse struct {
	Error string `json:"error"`
}

// ToPublicContentResponse converts a domain PublishedItem into an external PublicContentResponse DTO.
func ToPublicContentResponse(item PublishedItem) PublicContentResponse {
	return PublicContentResponse{
		ID:          item.ID,
		Kind:        string(item.Kind),
		Title:       item.Title,
		Content:     item.Content,
		PublishedAt: item.PublishedAt,
	}
}

// ToPublicContentResponses converts a slice of domain PublishedItems into external PublicContentResponse DTOs.
// Guarantees an allocated non-nil empty slice ([]) if input is empty or nil.
func ToPublicContentResponses(items []PublishedItem) []PublicContentResponse {
	res := make([]PublicContentResponse, 0, len(items))
	for _, item := range items {
		res = append(res, ToPublicContentResponse(item))
	}
	return res
}

// ToDraftResponse converts a domain Draft into an external DraftResponse DTO.
func ToDraftResponse(d Draft) DraftResponse {
	return DraftResponse{
		ID:          d.ID,
		Kind:        string(d.Kind),
		Title:       d.Title,
		Content:     d.Content,
		Status:      d.Status,
		CreatorID:   d.CreatorID,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
		PublishedAt: d.PublishedAt,
	}
}

// ToDraftSummaryResponse converts a domain DraftSummary into an external DraftSummaryResponse DTO.
func ToDraftSummaryResponse(s DraftSummary) DraftSummaryResponse {
	return DraftSummaryResponse{
		ID:        s.ID,
		Kind:      string(s.Kind),
		Title:     s.Title,
		UpdatedAt: s.UpdatedAt,
	}
}

// ToDraftSummaryResponses converts a slice of domain DraftSummaries into external DraftSummaryResponse DTOs.
// Guarantees an allocated non-nil empty slice ([]) if input is empty or nil.
func ToDraftSummaryResponses(items []DraftSummary) []DraftSummaryResponse {
	res := make([]DraftSummaryResponse, 0, len(items))
	for _, item := range items {
		res = append(res, ToDraftSummaryResponse(item))
	}
	return res
}
