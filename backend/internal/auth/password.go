package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	MinimumPasswordBytes = 8
	MaximumPasswordBytes = 1024
)

const (
	argon2IDMemoryKiB   = 19 * 1024
	argon2IDIterations  = 2
	argon2IDParallelism = 1
	argon2IDSaltBytes   = 16
	argon2IDKeyBytes    = 32
)

var (
	ErrEmailRequired     = errors.New("email is required")
	ErrPasswordRequired  = errors.New("password is required")
	ErrPasswordTooShort  = fmt.Errorf("password must be at least %d bytes", MinimumPasswordBytes)
	ErrPasswordTooLong   = fmt.Errorf("password must be at most %d bytes", MaximumPasswordBytes)
	ErrPasswordHashEmpty = errors.New("password hash is empty")
)

// NormalizeEmail returns the canonical form used for login lookup.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidateEmail applies the minimal product-side validation needed before signup/signin.
func ValidateEmail(email string) error {
	normalized := NormalizeEmail(email)
	if normalized == "" {
		return ErrEmailRequired
	}
	if strings.ContainsAny(normalized, " \t\r\n") || !strings.Contains(normalized, "@") {
		return fmt.Errorf("email is invalid")
	}
	return nil
}

// ValidatePassword checks whether a password can be stored with the configured hasher.
func ValidatePassword(password string) error {
	if password == "" {
		return ErrPasswordRequired
	}
	if len(password) < MinimumPasswordBytes {
		return ErrPasswordTooShort
	}
	if len(password) > MaximumPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}

// HashPassword stores a password using Argon2id with an encoded salt and parameters.
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, argon2IDSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argon2IDIterations, argon2IDMemoryKiB, argon2IDParallelism, argon2IDKeyBytes)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argon2IDMemoryKiB,
		argon2IDIterations,
		argon2IDParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether password matches the stored Argon2id hash.
func VerifyPassword(passwordHash string, password string) bool {
	if strings.TrimSpace(passwordHash) == "" || password == "" {
		return false
	}
	parameters, salt, expectedKey, err := parseArgon2IDHash(passwordHash)
	if err != nil {
		return false
	}
	actualKey := argon2.IDKey([]byte(password), salt, parameters.iterations, parameters.memoryKiB, parameters.parallelism, uint32(len(expectedKey)))
	return subtle.ConstantTimeCompare(actualKey, expectedKey) == 1
}

type argon2IDParameters struct {
	memoryKiB   uint32
	iterations  uint32
	parallelism uint8
}

func parseArgon2IDHash(encodedHash string) (argon2IDParameters, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return argon2IDParameters{}, nil, nil, fmt.Errorf("invalid password hash format")
	}
	if parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return argon2IDParameters{}, nil, nil, fmt.Errorf("unsupported argon2id version")
	}
	parameters, err := parseArgon2IDParameters(parts[3])
	if err != nil {
		return argon2IDParameters{}, nil, nil, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2IDParameters{}, nil, nil, fmt.Errorf("decode password salt: %w", err)
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2IDParameters{}, nil, nil, fmt.Errorf("decode password key: %w", err)
	}
	if len(salt) == 0 || len(key) == 0 {
		return argon2IDParameters{}, nil, nil, fmt.Errorf("password hash salt and key are required")
	}
	return parameters, salt, key, nil
}

func parseArgon2IDParameters(encodedParameters string) (argon2IDParameters, error) {
	var parameters argon2IDParameters
	for _, part := range strings.Split(encodedParameters, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return argon2IDParameters{}, fmt.Errorf("invalid argon2id parameter")
		}
		switch key {
		case "m":
			parsed, err := strconv.ParseUint(value, 10, 32)
			if err != nil || parsed == 0 {
				return argon2IDParameters{}, fmt.Errorf("invalid argon2id memory")
			}
			parameters.memoryKiB = uint32(parsed)
		case "t":
			parsed, err := strconv.ParseUint(value, 10, 32)
			if err != nil || parsed == 0 {
				return argon2IDParameters{}, fmt.Errorf("invalid argon2id iterations")
			}
			parameters.iterations = uint32(parsed)
		case "p":
			parsed, err := strconv.ParseUint(value, 10, 8)
			if err != nil || parsed == 0 {
				return argon2IDParameters{}, fmt.Errorf("invalid argon2id parallelism")
			}
			parameters.parallelism = uint8(parsed)
		default:
			return argon2IDParameters{}, fmt.Errorf("unknown argon2id parameter")
		}
	}
	if parameters.memoryKiB == 0 || parameters.iterations == 0 || parameters.parallelism == 0 {
		return argon2IDParameters{}, fmt.Errorf("missing argon2id parameters")
	}
	return parameters, nil
}
