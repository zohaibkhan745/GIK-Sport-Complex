package auth

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// faultyReader simulates a failure in the cryptographic randomness source.
type faultyReader struct{}

func (f *faultyReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated entropy depletion error")
}

func TestPassword_HashAndVerify_CorrectPassword(t *testing.T) {
	password := "correct-horse-battery-staple-99!"
	encoded, err := HashWithParams(password, TestParams, nil)
	if err != nil {
		t.Fatalf("HashWithParams failed: %v", err)
	}

	match, err := Verify(password, encoded)
	if err != nil {
		t.Fatalf("Verify returned unexpected error: %v", err)
	}
	if !match {
		t.Fatalf("expected password to match, but match was false")
	}
}

func TestPassword_HashAndVerify_IncorrectPassword(t *testing.T) {
	password := "correct-horse-battery-staple-99!"
	wrongPassword := "wrong-horse-battery-staple-00!"

	encoded, err := HashWithParams(password, TestParams, nil)
	if err != nil {
		t.Fatalf("HashWithParams failed: %v", err)
	}

	match, err := Verify(wrongPassword, encoded)
	if err != nil {
		t.Fatalf("Verify returned unexpected error: %v", err)
	}
	if match {
		t.Fatalf("expected wrong password to NOT match, but match was true")
	}
}

func TestPassword_Salting_DifferentHashesForIdenticalPasswords(t *testing.T) {
	password := "identical-password-string"

	hash1, err := HashWithParams(password, TestParams, nil)
	if err != nil {
		t.Fatalf("Hash 1 failed: %v", err)
	}
	hash2, err := HashWithParams(password, TestParams, nil)
	if err != nil {
		t.Fatalf("Hash 2 failed: %v", err)
	}

	if hash1 == hash2 {
		t.Fatalf("expected two salted hashes of the same password to differ, but both were: %s", hash1)
	}

	// Verify both distinct hashes still match the same password
	match1, err := Verify(password, hash1)
	if err != nil || !match1 {
		t.Fatalf("hash1 failed to verify: match=%v, err=%v", match1, err)
	}
	match2, err := Verify(password, hash2)
	if err != nil || !match2 {
		t.Fatalf("hash2 failed to verify: match=%v, err=%v", match2, err)
	}
}

func TestPassword_PreservesWhitespaceAndUnicode(t *testing.T) {
	testCases := []struct {
		name     string
		password string
		variant  string // slightly modified variant to prove exact byte sensitivity
	}{
		{
			name:     "leading whitespace preserved",
			password: "   spaces-at-front",
			variant:  "spaces-at-front",
		},
		{
			name:     "trailing whitespace preserved",
			password: "spaces-at-back   ",
			variant:  "spaces-at-back",
		},
		{
			name:     "surrounding and internal tabs and spaces",
			password: " \t internal \t spaces \t ",
			variant:  "internal spaces",
		},
		{
			name:     "multibyte unicode with emojis and RTL text",
			password: "🔒-P@sswørd-پاکستان-Urdu-Key-1234",
			variant:  "🔒-P@ssword-پاکستان-Urdu-Key-1234", // 'o' vs 'ø'
		},
		{
			name:     "case sensitivity preserved",
			password: "CaseSensitivePassword",
			variant:  "casesensitivepassword",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := HashWithParams(tc.password, TestParams, nil)
			if err != nil {
				t.Fatalf("failed to hash password for %s: %v", tc.name, err)
			}

			// Must match exact password
			match, err := Verify(tc.password, encoded)
			if err != nil || !match {
				t.Fatalf("expected exact password to match for %s: match=%v, err=%v", tc.name, match, err)
			}

			// Variant must NOT match
			variantMatch, err := Verify(tc.variant, encoded)
			if err != nil {
				t.Fatalf("unexpected error on variant verify: %v", err)
			}
			if variantMatch {
				t.Fatalf("variant must NOT match for %s, but Verify returned true", tc.name)
			}
		})
	}
}

func TestPassword_DefensiveParsing_MalformedAndUnsupportedHashes(t *testing.T) {
	validSalt := base64.RawStdEncoding.EncodeToString([]byte("1234567890123456"))
	validKey := base64.RawStdEncoding.EncodeToString(make([]byte, 32))

	malformedCases := []struct {
		name        string
		encodedHash string
		expectedErr error
	}{
		{
			name:        "empty string",
			encodedHash: "",
			expectedErr: ErrInvalidHashFormat,
		},
		{
			name:        "exceeds maximum encoded length",
			encodedHash: "$" + strings.Repeat("a", MaxEncodedHashLength+1),
			expectedErr: ErrInvalidHashFormat,
		},
		{
			name:        "missing leading dollar sign",
			encodedHash: fmt.Sprintf("argon2id$v=19$m=8192,t=1,p=2$%s$%s", validSalt, validKey),
			expectedErr: ErrInvalidHashFormat,
		},
		{
			name:        "insufficient fields",
			encodedHash: "$argon2id$v=19$m=8192,t=1,p=2",
			expectedErr: ErrInvalidHashFormat,
		},
		{
			name:        "too many fields",
			encodedHash: fmt.Sprintf("$argon2id$v=19$m=8192,t=1,p=2$%s$%s$extra", validSalt, validKey),
			expectedErr: ErrInvalidHashFormat,
		},
		{
			name:        "unsupported algorithm (bcrypt)",
			encodedHash: fmt.Sprintf("$bcrypt$v=19$m=8192,t=1,p=2$%s$%s", validSalt, validKey),
			expectedErr: ErrUnsupportedAlgorithm,
		},
		{
			name:        "unsupported algorithm (argon2i)",
			encodedHash: fmt.Sprintf("$argon2i$v=19$m=8192,t=1,p=2$%s$%s", validSalt, validKey),
			expectedErr: ErrUnsupportedAlgorithm,
		},
		{
			name:        "unsupported version (v=18)",
			encodedHash: fmt.Sprintf("$argon2id$v=18$m=8192,t=1,p=2$%s$%s", validSalt, validKey),
			expectedErr: ErrUnsupportedVersion,
		},
		{
			name:        "malformed version prefix",
			encodedHash: fmt.Sprintf("$argon2id$version=19$m=8192,t=1,p=2$%s$%s", validSalt, validKey),
			expectedErr: ErrInvalidHashFormat,
		},
		{
			name:        "missing memory parameter (m)",
			encodedHash: fmt.Sprintf("$argon2id$v=19$t=1,p=2$%s$%s", validSalt, validKey),
			expectedErr: ErrInvalidHashFormat,
		},
		{
			name:        "duplicate memory parameter",
			encodedHash: fmt.Sprintf("$argon2id$v=19$m=8192,m=8192,t=1,p=2$%s$%s", validSalt, validKey),
			expectedErr: ErrInvalidHashFormat,
		},
		{
			name:        "non-numeric parameter value",
			encodedHash: fmt.Sprintf("$argon2id$v=19$m=invalid,t=1,p=2$%s$%s", validSalt, validKey),
			expectedErr: ErrInvalidHashFormat,
		},
		{
			name:        "invalid base64 in salt",
			encodedHash: fmt.Sprintf("$argon2id$v=19$m=8192,t=1,p=2$invalid%%base64!$%s", validKey),
			expectedErr: ErrInvalidHashFormat,
		},
		{
			name:        "invalid base64 in key",
			encodedHash: fmt.Sprintf("$argon2id$v=19$m=8192,t=1,p=2$%s$invalid%%base64!", validSalt),
			expectedErr: ErrInvalidHashFormat,
		},
		{
			name:        "salt too short (< 16 bytes)",
			encodedHash: fmt.Sprintf("$argon2id$v=19$m=8192,t=1,p=2$%s$%s", base64.RawStdEncoding.EncodeToString([]byte("short")), validKey),
			expectedErr: ErrInvalidHashFormat,
		},
		{
			name:        "key too short (< 16 bytes)",
			encodedHash: fmt.Sprintf("$argon2id$v=19$m=8192,t=1,p=2$%s$%s", validSalt, base64.RawStdEncoding.EncodeToString([]byte("short"))),
			expectedErr: ErrInvalidHashFormat,
		},
	}

	for _, tc := range malformedCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := ParseHash(tc.encodedHash)
			if err == nil {
				t.Fatalf("expected error for %s, but ParseHash succeeded", tc.name)
			}
			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("expected error wrapping %v, got: %v", tc.expectedErr, err)
			}

			// Verify that Verify() also returns the error rather than panicking
			match, verifyErr := Verify("test-password", tc.encodedHash)
			if verifyErr == nil {
				t.Fatalf("expected Verify() to fail for %s, but it succeeded", tc.name)
			}
			if match {
				t.Fatalf("expected match to be false on malformed input")
			}
		})
	}
}

func TestPassword_DefensiveParsing_ExcessiveResourceParametersRejected(t *testing.T) {
	validSalt := base64.RawStdEncoding.EncodeToString([]byte("1234567890123456"))
	validKey := base64.RawStdEncoding.EncodeToString(make([]byte, 32))

	excessiveCases := []struct {
		name        string
		encodedHash string
	}{
		{
			name:        "memory below minimum (4 MiB)",
			encodedHash: fmt.Sprintf("$argon2id$v=19$m=4096,t=1,p=2$%s$%s", validSalt, validKey),
		},
		{
			name:        "memory above maximum (512 MiB)",
			encodedHash: fmt.Sprintf("$argon2id$v=19$m=524288,t=1,p=2$%s$%s", validSalt, validKey),
		},
		{
			name:        "iterations above maximum (50 passes)",
			encodedHash: fmt.Sprintf("$argon2id$v=19$m=8192,t=50,p=2$%s$%s", validSalt, validKey),
		},
		{
			name:        "iterations zero",
			encodedHash: fmt.Sprintf("$argon2id$v=19$m=8192,t=0,p=2$%s$%s", validSalt, validKey),
		},
		{
			name:        "parallelism above maximum (32 threads)",
			encodedHash: fmt.Sprintf("$argon2id$v=19$m=8192,t=1,p=32$%s$%s", validSalt, validKey),
		},
		{
			name:        "parallelism zero",
			encodedHash: fmt.Sprintf("$argon2id$v=19$m=8192,t=1,p=0$%s$%s", validSalt, validKey),
		},
	}

	for _, tc := range excessiveCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := ParseHash(tc.encodedHash)
			if err == nil {
				t.Fatalf("expected error for excessive parameter '%s', but ParseHash succeeded", tc.name)
			}
			if !errors.Is(err, ErrExcessiveParameters) {
				t.Fatalf("expected ErrExcessiveParameters, got: %v", err)
			}

			// Verify that Verify() rejects the excessive hash before running expensive key derivation
			match, verifyErr := Verify("any-password", tc.encodedHash)
			if verifyErr == nil || !errors.Is(verifyErr, ErrExcessiveParameters) {
				t.Fatalf("expected Verify() to fail with ErrExcessiveParameters, got err=%v", verifyErr)
			}
			if match {
				t.Fatalf("expected match to be false")
			}
		})
	}
}

func TestPassword_SaltGenerationFailure(t *testing.T) {
	faulty := &faultyReader{}
	_, err := HashWithParams("test-password", TestParams, faulty)
	if err == nil {
		t.Fatalf("expected error on failed salt generation, but HashWithParams succeeded")
	}
	if !errors.Is(err, ErrSaltGenerationFailed) {
		t.Fatalf("expected ErrSaltGenerationFailed, got: %v", err)
	}
}

func TestPassword_TechnicalLengthBound(t *testing.T) {
	oversizedPassword := strings.Repeat("A", MaxPasswordLength+1)

	_, err := Hash(oversizedPassword)
	if err == nil || !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("expected ErrPasswordTooLong on Hash, got: %v", err)
	}

	match, err := Verify(oversizedPassword, "$argon2id$v=19$m=8192,t=1,p=2$1234567890123456$12345678901234567890123456789012")
	if err == nil || !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("expected ErrPasswordTooLong on Verify, got: %v", err)
	}
	if match {
		t.Fatalf("expected match to be false")
	}
}

func TestPassword_ProductionDefaultsRoundTrip(t *testing.T) {
	// Verifies that production DefaultParams (64 MiB, 3 iterations, 4 threads, 16-byte salt, 32-byte key)
	// execute a complete round trip successfully.
	password := "production-roundtrip-test-password-88#"

	encoded, err := Hash(password)
	if err != nil {
		t.Fatalf("Hash() with production defaults failed: %v", err)
	}

	// Verify the encoded string adheres to expected production format
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("expected production parameters in hash, got format: %s", encoded)
	}

	match, err := Verify(password, encoded)
	if err != nil {
		t.Fatalf("Verify() failed for production hash: %v", err)
	}
	if !match {
		t.Fatalf("expected production hash to match correct password")
	}

	wrongMatch, err := Verify("incorrect-password", encoded)
	if err != nil {
		t.Fatalf("Verify() failed on wrong password check: %v", err)
	}
	if wrongMatch {
		t.Fatalf("expected wrong password to not match production hash")
	}
}
