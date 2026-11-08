package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Algorithm identifiers and RFC 9106 conventions.
const (
	AlgorithmArgon2id = "argon2id"
	Argon2Version     = argon2.Version // 19 (0x13)
)

// Technical input and parameter safety limits.
// These defensive limits ensure that stored or untrusted inputs cannot trigger
// algorithmic denial-of-service, excessive memory allocation, or unbounded CPU consumption.
const (
	MaxPasswordLength    = 1024  // Maximum password length in bytes to bound hashing input
	MaxEncodedHashLength = 512   // Maximum encoded hash string length to prevent parsing DoS
	MinMemory            = 8192  // Minimum allowed memory: 8 MiB (8192 KiB)
	MaxMemory            = 262144 // Maximum allowed memory: 256 MiB (262144 KiB)
	MinIterations        = 1     // Minimum allowed iterations (passes)
	MaxIterations        = 10    // Maximum allowed iterations to bound CPU time
	MinParallelism       = 1     // Minimum lanes/threads
	MaxParallelism       = 16    // Maximum lanes/threads to bound thread overhead
	MinSaltLength        = 16    // Minimum salt length: 16 bytes (128 bits)
	MaxSaltLength        = 64    // Maximum salt length: 64 bytes
	MinKeyLength         = 16    // Minimum key length: 16 bytes (128 bits)
	MaxKeyLength         = 64    // Maximum key length: 64 bytes
)

// Common authentication errors.
var (
	ErrInvalidHashFormat     = errors.New("auth: invalid encoded hash format")
	ErrUnsupportedAlgorithm  = errors.New("auth: unsupported password hashing algorithm")
	ErrUnsupportedVersion    = errors.New("auth: unsupported argon2 version")
	ErrExcessiveParameters   = errors.New("auth: hash parameters exceed resource bounds")
	ErrPasswordTooLong       = errors.New("auth: password exceeds maximum length of 1024 bytes")
	ErrSaltGenerationFailed  = errors.New("auth: failed to generate cryptographically secure random salt")
	ErrInvalidParams         = errors.New("auth: invalid hashing parameters")
)

// Params defines the configuration parameters for Argon2id key derivation.
// Units:
//   - Memory: Kibibytes (KiB). E.g., 65536 KiB = 64 MiB.
//   - Iterations: Number of passes over the memory buffer.
//   - Parallelism: Number of independent lanes/threads.
//   - SaltLength: Length of the cryptographic salt in bytes.
//   - KeyLength: Length of the generated derived key in bytes.
type Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultParams represents the deliberate production baseline configuration.
// Memory: 64 MiB (65536 KiB), Iterations: 3 passes, Parallelism: 4 threads,
// Salt: 16 bytes (128 bits), Key: 32 bytes (256 bits).
// These are implementation choices balancing defense against GPU brute-force attacks
// with server memory consumption, not measured performance claims.
var DefaultParams = Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

// TestParams provides inexpensive parameters specifically for fast automated unit tests
// to prevent testing suites from needlessly consuming CPU cycles and execution time.
var TestParams = Params{
	Memory:      8 * 1024,
	Iterations:  1,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// ValidateParams ensures that a Params struct satisfies all documented resource bounds.
func ValidateParams(p Params) error {
	if p.Memory < MinMemory || p.Memory > MaxMemory {
		return fmt.Errorf("%w: memory must be between %d and %d KiB", ErrInvalidParams, MinMemory, MaxMemory)
	}
	if p.Iterations < MinIterations || p.Iterations > MaxIterations {
		return fmt.Errorf("%w: iterations must be between %d and %d", ErrInvalidParams, MinIterations, MaxIterations)
	}
	if p.Parallelism < MinParallelism || p.Parallelism > MaxParallelism {
		return fmt.Errorf("%w: parallelism must be between %d and %d", ErrInvalidParams, MinParallelism, MaxParallelism)
	}
	if p.SaltLength < MinSaltLength || p.SaltLength > MaxSaltLength {
		return fmt.Errorf("%w: salt length must be between %d and %d bytes", ErrInvalidParams, MinSaltLength, MaxSaltLength)
	}
	if p.KeyLength < MinKeyLength || p.KeyLength > MaxKeyLength {
		return fmt.Errorf("%w: key length must be between %d and %d bytes", ErrInvalidParams, MinKeyLength, MaxKeyLength)
	}
	return nil
}

// Hash derives a salted Argon2id password hash using DefaultParams and crypto/rand.
// The output is an encoded string adhering to RFC 9106 conventions:
//
//	$argon2id$v=19$m=65536,t=3,p=4$<base64-salt>$<base64-hash>
//
// Passwords are never truncated, trimmed, or lowercased.
func Hash(password string) (string, error) {
	return HashWithParams(password, DefaultParams, rand.Reader)
}

// HashWithParams derives a salted Argon2id password hash with explicit parameters
// and a custom salt source, facilitating testing and parameter evolution.
func HashWithParams(password string, p Params, saltSource io.Reader) (string, error) {
	if len(password) > MaxPasswordLength {
		return "", ErrPasswordTooLong
	}
	if err := ValidateParams(p); err != nil {
		return "", err
	}
	if saltSource == nil {
		saltSource = rand.Reader
	}

	salt := make([]byte, p.SaltLength)
	if _, err := io.ReadFull(saltSource, salt); err != nil {
		return "", fmt.Errorf("%w: %v", ErrSaltGenerationFailed, err)
	}

	derivedKey := argon2.IDKey(
		[]byte(password),
		salt,
		p.Iterations,
		p.Memory,
		p.Parallelism,
		p.KeyLength,
	)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(derivedKey)

	encoded := fmt.Sprintf(
		"$%s$v=%d$m=%d,t=%d,p=%d$%s$%s",
		AlgorithmArgon2id,
		Argon2Version,
		p.Memory,
		p.Iterations,
		p.Parallelism,
		b64Salt,
		b64Hash,
	)

	return encoded, nil
}

// Verify checks whether a plaintext password matches an encoded Argon2id hash.
// It parses the hash defensively, validates all parameters against resource bounds
// BEFORE running Argon2, and performs a constant-time comparison on the derived key.
// Returns (true, nil) on match, (false, nil) on mismatch, and (false, err) on malformed input.
func Verify(password, encodedHash string) (bool, error) {
	if len(password) > MaxPasswordLength {
		return false, ErrPasswordTooLong
	}

	params, salt, expectedHash, err := ParseHash(encodedHash)
	if err != nil {
		return false, err
	}

	derivedKey := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		uint32(len(expectedHash)),
	)

	// Constant-time comparison ensures resistance to timing attacks
	if subtle.ConstantTimeCompare(expectedHash, derivedKey) == 1 {
		return true, nil
	}

	return false, nil
}

// ParseHash defensively parses an encoded Argon2 hash string, validating algorithm,
// version, parameter uniqueness, numeric validity, and resource bounds.
// Expected format:
//
//	$argon2id$v=19$m=65536,t=3,p=4$<b64salt>$<b64hash>
func ParseHash(encodedHash string) (*Params, []byte, []byte, error) {
	if len(encodedHash) == 0 || len(encodedHash) > MaxEncodedHashLength {
		return nil, nil, nil, ErrInvalidHashFormat
	}

	parts := strings.Split(encodedHash, "$")
	// Standard Argon2 string starts with '$', resulting in parts[0] == ""
	// Expected parts: ["", "argon2id", "v=19", "m=65536,t=3,p=4", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[0] != "" {
		return nil, nil, nil, ErrInvalidHashFormat
	}

	// 1. Validate algorithm
	if parts[1] != AlgorithmArgon2id {
		return nil, nil, nil, fmt.Errorf("%w: expected '%s', got '%s'", ErrUnsupportedAlgorithm, AlgorithmArgon2id, parts[1])
	}

	// 2. Validate version
	if !strings.HasPrefix(parts[2], "v=") {
		return nil, nil, nil, ErrInvalidHashFormat
	}
	versionNum, err := strconv.ParseUint(strings.TrimPrefix(parts[2], "v="), 10, 32)
	if err != nil {
		return nil, nil, nil, ErrInvalidHashFormat
	}
	if versionNum != Argon2Version {
		return nil, nil, nil, fmt.Errorf("%w: expected %d, got %d", ErrUnsupportedVersion, Argon2Version, versionNum)
	}

	// 3. Parse parameters (m, t, p) defensively
	paramParts := strings.Split(parts[3], ",")
	if len(paramParts) != 3 {
		return nil, nil, nil, ErrInvalidHashFormat
	}

	var params Params
	var hasM, hasT, hasP bool

	for _, p := range paramParts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 || len(kv[0]) == 0 || len(kv[1]) == 0 {
			return nil, nil, nil, ErrInvalidHashFormat
		}

		val, parseErr := strconv.ParseUint(kv[1], 10, 32)
		if parseErr != nil {
			return nil, nil, nil, ErrInvalidHashFormat
		}

		switch kv[0] {
		case "m":
			if hasM {
				return nil, nil, nil, ErrInvalidHashFormat // duplicate field
			}
			hasM = true
			params.Memory = uint32(val)
		case "t":
			if hasT {
				return nil, nil, nil, ErrInvalidHashFormat // duplicate field
			}
			hasT = true
			params.Iterations = uint32(val)
		case "p":
			if hasP {
				return nil, nil, nil, ErrInvalidHashFormat // duplicate field
			}
			hasP = true
			if val > 255 {
				return nil, nil, nil, ErrInvalidHashFormat
			}
			params.Parallelism = uint8(val)
		default:
			return nil, nil, nil, ErrInvalidHashFormat // unknown field
		}
	}

	if !hasM || !hasT || !hasP {
		return nil, nil, nil, ErrInvalidHashFormat
	}

	// 4. Resource bounds check: reject excessive parameters before hashing
	if params.Memory < MinMemory || params.Memory > MaxMemory ||
		params.Iterations < MinIterations || params.Iterations > MaxIterations ||
		params.Parallelism < MinParallelism || params.Parallelism > MaxParallelism {
		return nil, nil, nil, ErrExcessiveParameters
	}

	// 5. Decode salt
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: invalid salt base64", ErrInvalidHashFormat)
	}
	if len(salt) < MinSaltLength || len(salt) > MaxSaltLength {
		return nil, nil, nil, fmt.Errorf("%w: salt length out of bounds", ErrInvalidHashFormat)
	}
	params.SaltLength = uint32(len(salt))

	// 6. Decode hash
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: invalid hash base64", ErrInvalidHashFormat)
	}
	if len(hash) < MinKeyLength || len(hash) > MaxKeyLength {
		return nil, nil, nil, fmt.Errorf("%w: key length out of bounds", ErrInvalidHashFormat)
	}
	params.KeyLength = uint32(len(hash))

	return &params, salt, hash, nil
}
