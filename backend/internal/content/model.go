package content

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Kind represents the content item kind discriminator.
type Kind string

const (
	KindAnnouncement Kind = "announcement"
	KindAchievement  Kind = "achievement"
)

// MaxTitleLength defines the maximum character length for content item titles (schema limit VARCHAR(255)).
const MaxTitleLength = 255

// MaxPublishedItemsPerKind defines the maximum number of items returned by ListPublishedByKind in a single query.
const MaxPublishedItemsPerKind = 50

var (
	// ErrInvalidKind is returned when an unsupported content kind is specified.
	ErrInvalidKind = errors.New("invalid content kind")

	// ErrInvalidInput is returned when draft parameters fail data contract validation.
	ErrInvalidInput = errors.New("invalid content input")

	// ErrDraftNotFound is returned when a requested draft does not exist.
	ErrDraftNotFound = errors.New("draft not found")

	// ErrNotADraft is returned when an operation targeting a draft encounters a non-draft or published item.
	ErrNotADraft = errors.New("content item is not a draft")

	// ErrIncompleteDraft is returned when attempting to publish a draft with missing or blank title/content.
	ErrIncompleteDraft = errors.New("draft is incomplete and cannot be published")
)

// InvalidKindError provides structured context for an unsupported content kind.
type InvalidKindError struct {
	Kind string
}

func (e *InvalidKindError) Error() string {
	return fmt.Sprintf("invalid content kind: %q (supported: %q, %q)", e.Kind, KindAnnouncement, KindAchievement)
}

func (e *InvalidKindError) Is(target error) bool {
	return target == ErrInvalidKind
}

// DraftValidationError provides structured context for invalid draft input fields.
type DraftValidationError struct {
	Message string
}

func (e *DraftValidationError) Error() string {
	return fmt.Sprintf("invalid draft input: %s", e.Message)
}

func (e *DraftValidationError) Is(target error) bool {
	return target == ErrInvalidInput
}

// DraftNotFoundError provides structured context when a draft ID cannot be resolved.
type DraftNotFoundError struct {
	ID uuid.UUID
}

func (e *DraftNotFoundError) Error() string {
	return fmt.Sprintf("draft %s not found", e.ID)
}

func (e *DraftNotFoundError) Is(target error) bool {
	return target == ErrDraftNotFound
}

// NotADraftError provides structured context when an operation targeting a draft encounters a published or non-draft item.
type NotADraftError struct {
	ID     uuid.UUID
	Status string
}

func (e *NotADraftError) Error() string {
	return fmt.Sprintf("content item %s is not a draft (current status: %q)", e.ID, e.Status)
}

func (e *NotADraftError) Is(target error) bool {
	return target == ErrNotADraft
}

// IncompleteDraftError provides structured context when an incomplete draft cannot be published.
type IncompleteDraftError struct {
	ID     uuid.UUID
	Reason string
}

func (e *IncompleteDraftError) Error() string {
	return fmt.Sprintf("cannot publish incomplete draft %s: %s", e.ID, e.Reason)
}

func (e *IncompleteDraftError) Is(target error) bool {
	return target == ErrIncompleteDraft || target == ErrInvalidInput
}

// ValidateKind verifies that the kind is either announcement or achievement.
func ValidateKind(k Kind) error {
	switch k {
	case KindAnnouncement, KindAchievement:
		return nil
	default:
		return &InvalidKindError{Kind: string(k)}
	}
}

func validateDraftTitle(title *string) error {
	if title != nil && len([]rune(*title)) > MaxTitleLength {
		return &DraftValidationError{
			Message: fmt.Sprintf("title exceeds maximum allowed length of %d characters", MaxTitleLength),
		}
	}
	return nil
}

// PublishedItem is the public read model for published announcements and achievements.
// It exposes only public-facing fields and intentionally omits creator details,
// internal mutation timestamps (created_at, updated_at), and draft state flags.
type PublishedItem struct {
	ID          uuid.UUID `json:"id"`
	Kind        Kind      `json:"kind"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	PublishedAt time.Time `json:"publishedAt"`
}

// Draft represents an internal draft announcement or sports achievement.
// It is used exclusively by authenticated administrative/authoring operations
// and is never exposed through the public feed.
type Draft struct {
	ID          uuid.UUID  `json:"id"`
	Kind        Kind       `json:"kind"`
	Title       *string    `json:"title"`
	Content     *string    `json:"content"`
	Status      string     `json:"status"`
	CreatorID   uuid.UUID  `json:"creatorId"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
}

// CreateDraftInput contains parameters for creating a new draft content item.
type CreateDraftInput struct {
	Kind    Kind
	Title   *string
	Content *string
}

// Validate verifies input parameters for draft creation.
func (in CreateDraftInput) Validate() error {
	if err := ValidateKind(in.Kind); err != nil {
		return err
	}
	return validateDraftTitle(in.Title)
}

// UpdateDraftInput contains parameters for updating an existing draft's title and body.
type UpdateDraftInput struct {
	Title   *string
	Content *string
}

// Validate verifies input parameters for draft update.
func (in UpdateDraftInput) Validate() error {
	return validateDraftTitle(in.Title)
}

// AuditParams holds trusted actor metadata for administrative audit logging.
type AuditParams struct {
	ActorID   uuid.UUID
	IPAddress string
	UserAgent string
}

// StringPtr is a helper that returns a pointer to the given string.
func StringPtr(s string) *string {
	return &s
}
