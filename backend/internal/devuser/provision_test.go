package devuser

import (
	"bytes"
	"strings"
	"testing"

	"giki-sports-backend/internal/auth"
	"giki-sports-backend/internal/user"
)

func TestGetPreset(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantEmail   string
		wantRole    user.Role
		wantErrText string
	}{
		{
			name:      "valid director preset",
			input:     "director",
			wantEmail: "director@example.test",
			wantRole:  user.RoleSportsDirector,
		},
		{
			name:      "valid student preset",
			input:     "student",
			wantEmail: "student@example.test",
			wantRole:  user.RoleStudent,
		},
		{
			name:      "case-insensitive and trimmed director",
			input:     "  DiReCtOr  ",
			wantEmail: "director@example.test",
			wantRole:  user.RoleSportsDirector,
		},
		{
			name:      "case-insensitive and trimmed student",
			input:     "  STUDENT  ",
			wantEmail: "student@example.test",
			wantRole:  user.RoleStudent,
		},
		{
			name:        "disallowed super_admin role",
			input:       "super_admin",
			wantErrText: "unsupported preset \"super_admin\": allowed presets are 'director' or 'student'",
		},
		{
			name:        "disallowed faculty role",
			input:       "faculty",
			wantErrText: "unsupported preset \"faculty\": allowed presets are 'director' or 'student'",
		},
		{
			name:        "arbitrary string",
			input:       "admin",
			wantErrText: "unsupported preset \"admin\": allowed presets are 'director' or 'student'",
		},
		{
			name:        "empty string",
			input:       "",
			wantErrText: "unsupported preset \"\": allowed presets are 'director' or 'student'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def, err := GetPreset(tc.input)
			if tc.wantErrText != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrText)
				}
				if !strings.Contains(err.Error(), tc.wantErrText) {
					t.Errorf("expected error %q to contain %q", err.Error(), tc.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if def.Email != tc.wantEmail {
				t.Errorf("expected email %q, got %q", tc.wantEmail, def.Email)
			}
			if def.Role != tc.wantRole {
				t.Errorf("expected role %q, got %q", tc.wantRole, def.Role)
			}
		})
	}
}

func TestValidateTarget(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		dbName      string
		allowedDBs  []string
		wantErrText string
	}{
		{
			name:   "valid 127.0.0.1 loopback and default sports db",
			host:   "127.0.0.1",
			dbName: "giki_sports_complex",
		},
		{
			name:   "valid localhost and default sports db",
			host:   "localhost",
			dbName: "giki_sports_complex",
		},
		{
			name:   "valid IPv6 loopback ::1 and default sports db",
			host:   "::1",
			dbName: "giki_sports_complex",
		},
		{
			name:   "valid alternative loopback IP",
			host:   "127.0.0.2",
			dbName: "giki_sports_complex",
		},
		{
			name:        "empty host rejected",
			host:        "",
			dbName:      "giki_sports_complex",
			wantErrText: "database host cannot be empty",
		},
		{
			name:        "whitespace-only host rejected",
			host:        "   ",
			dbName:      "giki_sports_complex",
			wantErrText: "database host cannot be empty",
		},
		{
			name:        "non-loopback private IP rejected",
			host:        "192.168.1.100",
			dbName:      "giki_sports_complex",
			wantErrText: "target host \"192.168.1.100\" is not a loopback address",
		},
		{
			name:        "non-loopback public hostname rejected",
			host:        "db.example.com",
			dbName:      "giki_sports_complex",
			wantErrText: "target host \"db.example.com\" is not a loopback address",
		},
		{
			name:        "empty db name rejected",
			host:        "127.0.0.1",
			dbName:      "",
			wantErrText: "database name cannot be empty",
		},
		{
			name:        "disallowed db name giki_wallet_db rejected",
			host:        "127.0.0.1",
			dbName:      "giki_wallet_db",
			wantErrText: "target database \"giki_wallet_db\" is not permitted",
		},
		{
			name:        "disallowed db name postgres rejected",
			host:        "127.0.0.1",
			dbName:      "postgres",
			wantErrText: "target database \"postgres\" is not permitted",
		},
		{
			name:        "custom allowedDBs list matched",
			host:        "127.0.0.1",
			dbName:      "custom_disposable_db",
			allowedDBs:  []string{"custom_disposable_db"},
		},
		{
			name:        "custom allowedDBs list unmatched rejected",
			host:        "127.0.0.1",
			dbName:      "other_db",
			allowedDBs:  []string{"custom_disposable_db"},
			wantErrText: "target database \"other_db\" is not permitted",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTarget(tc.host, tc.dbName, tc.allowedDBs...)
			if tc.wantErrText != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrText)
				}
				if !strings.Contains(err.Error(), tc.wantErrText) {
					t.Errorf("expected error %q to contain %q", err.Error(), tc.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestReadPasswordInteractive(t *testing.T) {
	t.Run("refuses noninteractive input", func(t *testing.T) {
		var buf bytes.Buffer
		_, err := ReadPasswordInteractive(false, func() ([]byte, error) {
			return []byte("pass"), nil
		}, &buf, "user@example.test")

		if err == nil {
			t.Fatal("expected error for noninteractive input, got nil")
		}
		if !strings.Contains(err.Error(), "refusing noninteractive input") {
			t.Errorf("expected noninteractive error message, got %v", err)
		}
	})

	t.Run("accepts matching password", func(t *testing.T) {
		var buf bytes.Buffer
		calls := 0
		readFn := func() ([]byte, error) {
			calls++
			return []byte("test-dev-pass"), nil
		}

		pass, err := ReadPasswordInteractive(true, readFn, &buf, "user@example.test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pass != "test-dev-pass" {
			t.Errorf("expected password to match, got %q", pass)
		}
		if calls != 2 {
			t.Errorf("expected exactly 2 read calls (prompt and confirm), got %d", calls)
		}
	})

	t.Run("rejects mismatched passwords", func(t *testing.T) {
		var buf bytes.Buffer
		inputs := [][]byte{[]byte("firstPassword"), []byte("secondPassword")}
		idx := 0
		readFn := func() ([]byte, error) {
			val := inputs[idx]
			idx++
			return val, nil
		}

		_, err := ReadPasswordInteractive(true, readFn, &buf, "user@example.test")
		if err == nil {
			t.Fatal("expected mismatch error, got nil")
		}
		if !strings.Contains(err.Error(), "passwords do not match") {
			t.Errorf("expected 'passwords do not match' error, got %v", err)
		}
	})

	t.Run("rejects empty password", func(t *testing.T) {
		var buf bytes.Buffer
		readFn := func() ([]byte, error) {
			return []byte(""), nil
		}

		_, err := ReadPasswordInteractive(true, readFn, &buf, "user@example.test")
		if err == nil {
			t.Fatal("expected empty password error, got nil")
		}
		if !strings.Contains(err.Error(), "password cannot be empty") {
			t.Errorf("expected 'password cannot be empty' error, got %v", err)
		}
	})

	t.Run("rejects password exceeding max length", func(t *testing.T) {
		var buf bytes.Buffer
		longPass := strings.Repeat("A", auth.MaxPasswordLength+1)
		readFn := func() ([]byte, error) {
			return []byte(longPass), nil
		}

		_, err := ReadPasswordInteractive(true, readFn, &buf, "user@example.test")
		if err == nil {
			t.Fatal("expected length error, got nil")
		}
		if !strings.Contains(err.Error(), "password exceeds maximum allowed length") {
			t.Errorf("expected max length error, got %v", err)
		}
	})
}

