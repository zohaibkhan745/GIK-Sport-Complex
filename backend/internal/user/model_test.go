package user_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"giki-sports-backend/internal/user"
)

func TestUser_NormalizeEmail(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "already normalized lowercase",
			input:    "student@giki.edu.pk",
			expected: "student@giki.edu.pk",
		},
		{
			name:     "uppercase converted to lowercase",
			input:    "STUDENT@GIKI.EDU.PK",
			expected: "student@giki.edu.pk",
		},
		{
			name:     "leading and trailing spaces trimmed",
			input:    "   faculty@giki.edu.pk   ",
			expected: "faculty@giki.edu.pk",
		},
		{
			name:     "tabs and mixed case normalized",
			input:    "\tSports.Director@GIKI.edu.pk\n",
			expected: "sports.director@giki.edu.pk",
		},
		{
			name:     "arbitrary external domain normalized without rejection",
			input:    "External.Coach@gmail.com",
			expected: "external.coach@gmail.com",
		},
		{
			name:     "empty string returns empty",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only returns empty",
			input:    "   \t\n  ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := user.NormalizeEmail(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeEmail(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestUser_CredentialRedactionAndJSONOmission(t *testing.T) {
	const syntheticHash = "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$c29tZWhhc2g"

	creds := user.UserCredentials{
		User: user.User{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			Email:       "director@giki.edu.pk",
			DisplayName: "Sports Director",
			Status:      user.StatusActive,
			Roles:       []user.Role{user.RoleSportsDirector},
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
		PasswordHash: syntheticHash,
	}

	// 1. String formatting checks: must never leak the hash
	formats := []string{
		fmt.Sprintf("%s", creds),
		fmt.Sprintf("%v", creds),
		fmt.Sprintf("%+v", creds),
		fmt.Sprintf("%#v", creds),
	}

	for _, formatted := range formats {
		if strings.Contains(formatted, syntheticHash) {
			t.Errorf("credential formatting leaked raw password hash: %s", formatted)
		}
		if !strings.Contains(formatted, "[REDACTED]") {
			t.Errorf("credential formatting did not include [REDACTED]: %s", formatted)
		}
	}

	// 2. JSON serialization check: PasswordHash must be excluded
	jsonData, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("failed to marshal UserCredentials: %v", err)
	}

	jsonStr := string(jsonData)
	if strings.Contains(jsonStr, syntheticHash) {
		t.Errorf("JSON serialization leaked raw password hash: %s", jsonStr)
	}
	if strings.Contains(jsonStr, "PasswordHash") || strings.Contains(jsonStr, "password_hash") {
		t.Errorf("JSON serialization included PasswordHash field: %s", jsonStr)
	}
}

func TestUser_NotFoundError(t *testing.T) {
	err := &user.NotFoundError{Identifier: "unknown@giki.edu.pk"}

	if !errors.Is(err, user.ErrNotFound) {
		t.Errorf("expected errors.Is(err, user.ErrNotFound) to be true")
	}

	expectedMsg := `user "unknown@giki.edu.pk" not found`
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}
