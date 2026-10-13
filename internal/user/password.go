package user

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
	passwordSaltLength  = 16
	passwordKeyLength   = 32
	passwordMemory      = 19 * 1024
	passwordIterations  = 2
	passwordParallelism = 1
)

var ErrInvalidPasswordHash = errors.New("invalid password hash")

// HashPassword returns an encoded Argon2id password hash.
// The caller must never store or log the original password.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("%w: password is empty", ErrInvalidUser)
	}

	salt := make([]byte, passwordSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	key := argon2.IDKey(
		[]byte(password),
		salt,
		passwordIterations,
		passwordMemory,
		passwordParallelism,
		passwordKeyLength,
	)

	enc := base64.RawStdEncoding
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		passwordMemory,
		passwordIterations,
		passwordParallelism,
		enc.EncodeToString(salt),
		enc.EncodeToString(key),
	), nil
}

// VerifyPassword checks a password against an encoded Argon2id hash.
func VerifyPassword(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, ErrInvalidPasswordHash
	}

	var memory, iterations, parallelism uint64

	for _, field := range strings.Split(parts[3], ",") {
		keyValue := strings.SplitN(field, "=", 2)
		if len(keyValue) != 2 {
			return false, ErrInvalidPasswordHash
		}

		value, err := strconv.ParseUint(keyValue[1], 10, 32)
		if err != nil {
			return false, ErrInvalidPasswordHash
		}

		switch keyValue[0] {
		case "m":
			memory = value
		case "t":
			iterations = value
		case "p":
			parallelism = value
		default:
			return false, ErrInvalidPasswordHash
		}
	}

	// Reject unexpected parameters and excessive costs before allocating memory.
	if memory != passwordMemory ||
		iterations != passwordIterations ||
		parallelism != passwordParallelism {
		return false, ErrInvalidPasswordHash
	}

	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil || len(salt) != passwordSaltLength {
		return false, ErrInvalidPasswordHash
	}

	expectedKey, err := enc.DecodeString(parts[5])
	if err != nil || len(expectedKey) != passwordKeyLength {
		return false, ErrInvalidPasswordHash
	}

	actualKey := argon2.IDKey(
		[]byte(password),
		salt,
		uint32(iterations),
		uint32(memory),
		uint8(parallelism),
		uint32(len(expectedKey)),
	)

	return subtle.ConstantTimeCompare(actualKey, expectedKey) == 1, nil
}
