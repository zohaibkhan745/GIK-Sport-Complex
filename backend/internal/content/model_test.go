package content

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidateKind(t *testing.T) {
	tests := []struct {
		name    string
		kind    Kind
		wantErr bool
	}{
		{
			name:    "valid announcement",
			kind:    KindAnnouncement,
			wantErr: false,
		},
		{
			name:    "valid achievement",
			kind:    KindAchievement,
			wantErr: false,
		},
		{
			name:    "empty kind string",
			kind:    "",
			wantErr: true,
		},
		{
			name:    "arbitrary unknown kind",
			kind:    "press_release",
			wantErr: true,
		},
		{
			name:    "case sensitive mismatch",
			kind:    "ANNOUNCEMENT",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKind(tt.kind)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for kind %q, got nil", tt.kind)
				}
				if !errors.Is(err, ErrInvalidKind) {
					t.Errorf("expected error to wrap ErrInvalidKind, got: %v", err)
				}
				var invalidKindErr *InvalidKindError
				if !errors.As(err, &invalidKindErr) {
					t.Errorf("expected error to be *InvalidKindError, got: %T", err)
				} else if invalidKindErr.Kind != string(tt.kind) {
					t.Errorf("expected InvalidKindError.Kind to be %q, got %q", tt.kind, invalidKindErr.Kind)
				}
			} else {
				if err != nil {
					t.Fatalf("expected nil error for kind %q, got: %v", tt.kind, err)
				}
			}
		})
	}
}

func TestValidateDraftTitle(t *testing.T) {
	t.Run("nil title is valid", func(t *testing.T) {
		if err := validateDraftTitle(nil); err != nil {
			t.Fatalf("expected nil error for nil title, got: %v", err)
		}
	})

	t.Run("empty title is valid", func(t *testing.T) {
		title := ""
		if err := validateDraftTitle(&title); err != nil {
			t.Fatalf("expected nil error for empty title, got: %v", err)
		}
	})

	t.Run("title up to 255 chars is valid", func(t *testing.T) {
		title := strings.Repeat("A", MaxTitleLength)
		if err := validateDraftTitle(&title); err != nil {
			t.Fatalf("expected nil error for 255-char title, got: %v", err)
		}
	})

	t.Run("title exceeding 255 chars is rejected", func(t *testing.T) {
		title := strings.Repeat("A", MaxTitleLength+1)
		err := validateDraftTitle(&title)
		if err == nil {
			t.Fatalf("expected validation error for title > 255 chars, got nil")
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected error wrapping ErrInvalidInput, got: %v", err)
		}
	})
}

func TestStructuredErrors(t *testing.T) {
	t.Run("DraftNotFoundError", func(t *testing.T) {
		id := uuid.New()
		err := &DraftNotFoundError{ID: id}
		if !errors.Is(err, ErrDraftNotFound) {
			t.Errorf("expected error to match ErrDraftNotFound")
		}
		if !strings.Contains(err.Error(), id.String()) {
			t.Errorf("expected error message to include ID %s, got: %s", id, err.Error())
		}
	})

	t.Run("NotADraftError", func(t *testing.T) {
		id := uuid.New()
		err := &NotADraftError{ID: id, Status: "published"}
		if !errors.Is(err, ErrNotADraft) {
			t.Errorf("expected error to match ErrNotADraft")
		}
		if !strings.Contains(err.Error(), "published") {
			t.Errorf("expected error message to include status 'published', got: %s", err.Error())
		}
	})

	t.Run("DraftValidationError", func(t *testing.T) {
		err := &DraftValidationError{Message: "custom message"}
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected error to match ErrInvalidInput")
		}
	})

	t.Run("IncompleteDraftError", func(t *testing.T) {
		id := uuid.New()
		err := &IncompleteDraftError{ID: id, Reason: "missing title"}
		if !errors.Is(err, ErrIncompleteDraft) {
			t.Errorf("expected error to match ErrIncompleteDraft")
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected error to also match ErrInvalidInput")
		}
		if !strings.Contains(err.Error(), "missing title") {
			t.Errorf("expected error message to include reason, got: %s", err.Error())
		}
	})
}
