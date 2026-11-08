package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"
)

// Session token format and size constants.
const (
	// SessionTokenEntropyBytes specifies the raw cryptographic randomness (32 bytes = 256 bits)
	// required for high-entropy opaque session tokens.
	SessionTokenEntropyBytes = 32

	// SessionTokenEncodedLength specifies the exact character length of a 32-byte value
	// encoded in unpadded URL-safe Base64 (ceil(32 * 8 / 6) = 43 characters).
	SessionTokenEncodedLength = 43

	// SessionTokenHashLength specifies the exact character length of a 32-byte SHA-256 digest
	// formatted as lowercase hexadecimal (32 * 2 = 64 characters).
	SessionTokenHashLength = 64
)

// Sentinel errors returned by session token operations.
// These errors never interpolate or log candidate tokens to prevent leaking sensitive credentials.
var (
	ErrTokenEmpty         = errors.New("auth: session token must not be empty")
	ErrInvalidTokenLength = errors.New("auth: session token length is invalid")
	ErrInvalidTokenFormat = errors.New("auth: session token format is invalid")
	ErrNonCanonicalToken  = errors.New("auth: session token is not in canonical unpadded base64url format")
	ErrEntropyReadFailed  = errors.New("auth: failed to read secure random bytes for session token")
)

// GenerateSessionTokenWithReader generates a high-entropy session token using an injectable
// entropy source, returning both the raw token string (to be issued to the client as a cookie)
// and its SHA-256 hash (to be persisted in giki_sports.sessions.token_hash).
//
// HASHING CONVENTION:
// SHA-256 hashing operates directly on the canonical ASCII bytes of the unpadded URL-safe
// Base64-encoded token string ([]byte(token)), matching the GIKI Wallet sha256Hex convention.
func GenerateSessionTokenWithReader(r io.Reader) (string, string, error) {
	if r == nil {
		return "", "", ErrEntropyReadFailed
	}

	buf := make([]byte, SessionTokenEntropyBytes)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", "", ErrEntropyReadFailed
	}

	token := base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(sum[:])

	return token, tokenHash, nil
}

// GenerateSessionToken generates a cryptographically secure 32-byte random session token
// using crypto/rand.Reader. It returns the raw canonical token string (43 characters) and
// its lowercase hexadecimal SHA-256 hash (64 characters).
func GenerateSessionToken() (string, string, error) {
	return GenerateSessionTokenWithReader(rand.Reader)
}

// ValidateSessionToken strictly verifies that candidateToken conforms to the canonical
// session token format produced by this package:
//   - Must not be empty.
//   - Must be exactly 43 characters long.
//   - Must not contain padding characters ('=').
//   - Must consist strictly of URL-safe Base64 alphabet characters ([A-Za-z0-9_-]).
//   - Must decode to exactly 32 bytes.
//   - Must be strictly canonical (re-encoding decoded bytes reproduces the input exactly).
//
// IMPORTANT SECURITY BOUNDARY:
// This function validates token format only. It does NOT verify session existence, expiry,
// revocation, or user identity. Those checks require database verification in Step 6D.
func ValidateSessionToken(candidateToken string) error {
	if len(candidateToken) == 0 {
		return ErrTokenEmpty
	}

	if len(candidateToken) != SessionTokenEncodedLength {
		return ErrInvalidTokenLength
	}

	if strings.ContainsRune(candidateToken, '=') {
		return ErrInvalidTokenFormat
	}

	decoded, err := base64.RawURLEncoding.DecodeString(candidateToken)
	if err != nil {
		return ErrInvalidTokenFormat
	}

	if len(decoded) != SessionTokenEntropyBytes {
		return ErrInvalidTokenLength
	}

	if base64.RawURLEncoding.EncodeToString(decoded) != candidateToken {
		return ErrNonCanonicalToken
	}

	return nil
}

// HashSessionToken validates candidateToken format and derives its lowercase hexadecimal
// SHA-256 hash for database query lookup.
//
// If candidateToken is malformed or non-canonical, HashSessionToken fails fast before
// computing the hash and returns a static sentinel error.
//
// HASHING CONVENTION:
// Derives SHA-256 from the canonical ASCII bytes of the unpadded URL-safe Base64 token
// string ([]byte(token)), returning a 64-character lowercase hexadecimal string.
func HashSessionToken(candidateToken string) (string, error) {
	if err := ValidateSessionToken(candidateToken); err != nil {
		return "", err
	}

	sum := sha256.Sum256([]byte(candidateToken))
	return hex.EncodeToString(sum[:]), nil
}

// ErrInvalidTokenHashFormat is returned when a candidate token hash fails format validation.
var ErrInvalidTokenHashFormat = errors.New("auth: session token hash format is invalid")

// ValidateTokenHash strictly verifies that candidateHash matches the canonical format
// of a SHA-256 session token digest:
//   - Exactly 64 characters long.
//   - Composed exclusively of lowercase hexadecimal characters ([0-9a-f]).
func ValidateTokenHash(candidateHash string) error {
	if len(candidateHash) != SessionTokenHashLength {
		return ErrInvalidTokenHashFormat
	}
	for i := 0; i < len(candidateHash); i++ {
		c := candidateHash[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return ErrInvalidTokenHashFormat
		}
	}
	return nil
}
