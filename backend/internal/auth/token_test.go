package auth

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
)

// errFaultyReader is a test mock simulating entropy read failure.
type errFaultyReader struct{}

func (e *errFaultyReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated hardware entropy failure")
}

// shortReader provides fewer bytes than requested to simulate short entropy reads.
type shortReader struct {
	data []byte
}

func (s *shortReader) Read(p []byte) (n int, err error) {
	if len(s.data) == 0 {
		return 0, io.EOF
	}
	n = copy(p, s.data)
	s.data = s.data[n:]
	return n, nil
}

func TestToken_GenerateSessionToken_FormatAndLength(t *testing.T) {
	token, tokenHash, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("expected successful token generation, got error: %v", err)
	}

	// Verify raw token length and format
	if len(token) != SessionTokenEncodedLength {
		t.Errorf("expected raw token length %d, got %d", SessionTokenEncodedLength, len(token))
	}

	// Verify token hash length and format
	if len(tokenHash) != SessionTokenHashLength {
		t.Errorf("expected token hash length %d, got %d", SessionTokenHashLength, len(tokenHash))
	}

	// Decoded token must contain exactly 32 bytes of raw entropy
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("expected raw token to decode as unpadded URL-safe base64, got: %v", err)
	}
	if len(decoded) != SessionTokenEntropyBytes {
		t.Errorf("expected decoded entropy to be %d bytes, got %d", SessionTokenEntropyBytes, len(decoded))
	}

	// Validation must pass on freshly generated token
	if err := ValidateSessionToken(token); err != nil {
		t.Errorf("expected freshly generated token to pass validation, got: %v", err)
	}

	// Derived hash from HashSessionToken must match the one returned at generation
	recomputedHash, err := HashSessionToken(token)
	if err != nil {
		t.Fatalf("expected HashSessionToken to succeed, got: %v", err)
	}
	if recomputedHash != tokenHash {
		t.Errorf("expected recomputed hash %q to match returned hash %q", recomputedHash, tokenHash)
	}
}

func TestToken_DeterministicEntropy_KnownVectors(t *testing.T) {
	// Vector 1: 32 zero bytes
	zeroEntropy := make([]byte, SessionTokenEntropyBytes)
	token1, hash1, err := GenerateSessionTokenWithReader(bytes.NewReader(zeroEntropy))
	if err != nil {
		t.Fatalf("failed to generate token from zero entropy: %v", err)
	}

	// 32 zero bytes in unpadded base64url is exactly 43 'A' characters
	expectedToken1 := strings.Repeat("A", 43)
	if token1 != expectedToken1 {
		t.Errorf("expected token1 %q, got %q", expectedToken1, token1)
	}

	// Independently compute expected SHA-256 hash of expectedToken1
	expectedSum1 := sha256.Sum256([]byte(expectedToken1))
	expectedHash1 := hex.EncodeToString(expectedSum1[:])
	if hash1 != expectedHash1 {
		t.Errorf("expected hash1 %q, got %q", expectedHash1, hash1)
	}

	// Vector 2: Sequential bytes 0x00 through 0x1F (32 bytes)
	seqEntropy := make([]byte, SessionTokenEntropyBytes)
	for i := 0; i < SessionTokenEntropyBytes; i++ {
		seqEntropy[i] = byte(i)
	}
	token2, hash2, err := GenerateSessionTokenWithReader(bytes.NewReader(seqEntropy))
	if err != nil {
		t.Fatalf("failed to generate token from sequential entropy: %v", err)
	}

	expectedToken2 := base64.RawURLEncoding.EncodeToString(seqEntropy)
	if token2 != expectedToken2 {
		t.Errorf("expected token2 %q, got %q", expectedToken2, token2)
	}

	expectedSum2 := sha256.Sum256([]byte(expectedToken2))
	expectedHash2 := hex.EncodeToString(expectedSum2[:])
	if hash2 != expectedHash2 {
		t.Errorf("expected hash2 %q, got %q", expectedHash2, hash2)
	}

	// Tokens and hashes from different entropy must differ
	if token1 == token2 {
		t.Error("expected different entropy to yield different tokens")
	}
	if hash1 == hash2 {
		t.Error("expected different tokens to yield different hashes")
	}
}

func TestToken_HashSessionToken_RepeatedCallsConsistent(t *testing.T) {
	token, initialHash, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	for i := 0; i < 5; i++ {
		h, err := HashSessionToken(token)
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
		if h != initialHash {
			t.Errorf("call %d returned hash %q, expected %q", i, h, initialHash)
		}
	}
}

func TestToken_Validation_RejectsInvalidFormats(t *testing.T) {
	// Create a valid baseline token to construct targeted malformations
	validToken, _, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("failed to generate valid baseline token: %v", err)
	}

	// Create a non-canonical token by modifying the unused bits of the 43rd character.
	// For 32 bytes (256 bits), 42 * 6 = 252 bits. Character 43 encodes 4 data bits and 2 unused bits.
	// In canonical encoding of 32 zero bytes ("AAAA...A"), character 43 is 'A' (value 0).
	// If we replace the last character with 'B' (value 1, binary 000001), the 4 data bits are still 0,
	// but the unused bit is 1. base64.RawURLEncoding decodes it to the same 32 zero bytes,
	// but re-encoding yields 'A', proving non-canonicity!
	nonCanonicalToken := strings.Repeat("A", 42) + "B"

	tests := []struct {
		name        string
		candidate   string
		expectedErr error
	}{
		{
			name:        "empty string",
			candidate:   "",
			expectedErr: ErrTokenEmpty,
		},
		{
			name:        "too short (42 characters)",
			candidate:   validToken[:42],
			expectedErr: ErrInvalidTokenLength,
		},
		{
			name:        "too long (44 characters)",
			candidate:   validToken + "A",
			expectedErr: ErrInvalidTokenLength,
		},
		{
			name:        "padded base64 (44 characters with trailing equal sign)",
			candidate:   validToken + "=",
			expectedErr: ErrInvalidTokenLength,
		},
		{
			name:        "contains standard base64 plus sign (+)",
			candidate:   strings.Replace(validToken[:42]+"+", "+", "+", 1),
			expectedErr: ErrInvalidTokenFormat,
		},
		{
			name:        "contains standard base64 slash sign (/)",
			candidate:   strings.Replace(validToken[:42]+"/", "/", "/", 1),
			expectedErr: ErrInvalidTokenFormat,
		},
		{
			name:        "contains internal whitespace",
			candidate:   validToken[:20] + " " + validToken[21:],
			expectedErr: ErrInvalidTokenFormat,
		},
		{
			name:        "leading whitespace preserved and rejected (no trimming)",
			candidate:   " " + validToken[:42],
			expectedErr: ErrInvalidTokenFormat,
		},
		{
			name:        "trailing whitespace preserved and rejected (no trimming)",
			candidate:   validToken[:42] + " ",
			expectedErr: ErrInvalidTokenFormat,
		},
		{
			name:        "contains invalid punctuation (!)",
			candidate:   validToken[:42] + "!",
			expectedErr: ErrInvalidTokenFormat,
		},
		{
			name:        "non-canonical base64 representation",
			candidate:   nonCanonicalToken,
			expectedErr: ErrNonCanonicalToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valErr := ValidateSessionToken(tt.candidate)
			if valErr == nil {
				t.Fatalf("expected error %v for %q, got nil", tt.expectedErr, tt.name)
			}
			if !errors.Is(valErr, tt.expectedErr) {
				t.Errorf("expected error %v, got %v", tt.expectedErr, valErr)
			}

			// HashSessionToken must also reject the candidate with the same error
			hash, hashErr := HashSessionToken(tt.candidate)
			if hash != "" {
				t.Errorf("expected empty hash on invalid token, got %q", hash)
			}
			if !errors.Is(hashErr, tt.expectedErr) {
				t.Errorf("HashSessionToken expected error %v, got %v", tt.expectedErr, hashErr)
			}
		})
	}
}

func TestToken_EntropySourceFailures(t *testing.T) {
	// Case 1: Nil entropy reader
	_, _, errNil := GenerateSessionTokenWithReader(nil)
	if !errors.Is(errNil, ErrEntropyReadFailed) {
		t.Errorf("expected ErrEntropyReadFailed for nil reader, got %v", errNil)
	}

	// Case 2: Faulty reader returning immediate error
	_, _, errFaulty := GenerateSessionTokenWithReader(&errFaultyReader{})
	if !errors.Is(errFaulty, ErrEntropyReadFailed) {
		t.Errorf("expected ErrEntropyReadFailed for faulty reader, got %v", errFaulty)
	}

	// Case 3: Short reader providing only 16 bytes instead of 32
	_, _, errShort := GenerateSessionTokenWithReader(&shortReader{data: make([]byte, 16)})
	if !errors.Is(errShort, ErrEntropyReadFailed) {
		t.Errorf("expected ErrEntropyReadFailed for short reader, got %v", errShort)
	}
}

func TestToken_ErrorsDoNotLeakSubmittedToken(t *testing.T) {
	sensitiveCandidate := "SECRET_TOKEN_DO_NOT_LEAK_IN_ANY_ERROR_MSG__"
	if len(sensitiveCandidate) != SessionTokenEncodedLength {
		t.Fatalf("test setup error: sensitive candidate length %d != %d", len(sensitiveCandidate), SessionTokenEncodedLength)
	}

	valErr := ValidateSessionToken(sensitiveCandidate)
	if valErr != nil && strings.Contains(valErr.Error(), sensitiveCandidate) {
		t.Errorf("ValidateSessionToken leaked sensitive token in error message: %v", valErr)
	}

	_, hashErr := HashSessionToken(sensitiveCandidate)
	if hashErr != nil && strings.Contains(hashErr.Error(), sensitiveCandidate) {
		t.Errorf("HashSessionToken leaked sensitive token in error message: %v", hashErr)
	}
}

func TestToken_ValidateTokenHash(t *testing.T) {
	validHash := strings.Repeat("a", 64)
	if err := ValidateTokenHash(validHash); err != nil {
		t.Fatalf("expected valid hash to pass validation, got %v", err)
	}

	tests := []struct {
		name        string
		hash        string
		expectedErr error
	}{
		{
			name:        "empty hash",
			hash:        "",
			expectedErr: ErrInvalidTokenHashFormat,
		},
		{
			name:        "too short (63 characters)",
			hash:        strings.Repeat("a", 63),
			expectedErr: ErrInvalidTokenHashFormat,
		},
		{
			name:        "too long (65 characters)",
			hash:        strings.Repeat("a", 65),
			expectedErr: ErrInvalidTokenHashFormat,
		},
		{
			name:        "uppercase hex characters rejected",
			hash:        strings.Repeat("A", 64),
			expectedErr: ErrInvalidTokenHashFormat,
		},
		{
			name:        "non-hex characters rejected (g)",
			hash:        strings.Repeat("a", 63) + "g",
			expectedErr: ErrInvalidTokenHashFormat,
		},
		{
			name:        "spaces rejected",
			hash:        " " + strings.Repeat("a", 63),
			expectedErr: ErrInvalidTokenHashFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTokenHash(tt.hash)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tt.name)
			}
			if !errors.Is(err, tt.expectedErr) {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}
