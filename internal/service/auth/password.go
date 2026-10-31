package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemoryKiB   = 19 * 1024
	argonIterations  = 2
	argonParallelism = 1
	argonSaltLen     = 16
	argonKeyLen      = 32
	minPasswordLen   = 8
	maxPasswordLen   = 72
	maxArgonMemory   = 1024 * 1024
	maxArgonTime     = 10
	maxArgonThreads  = 8
)

// HashPassword returns a PHC string. The cost parameters sit inside the string so a
// later change still verifies hashes written today.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}
	sum := argon2.IDKey([]byte(password), salt, argonIterations, argonMemoryKiB, argonParallelism, argonKeyLen)
	return formatPHC(salt, sum), nil
}

// VerifyPassword reports whether password matches a PHC string from HashPassword.
func VerifyPassword(password, encoded string) (bool, error) {
	parsed, err := parsePHC(encoded)
	if err != nil {
		return false, err
	}
	sum := argon2.IDKey([]byte(password), parsed.salt, parsed.iterations, parsed.memory, parsed.threads, argonKeyLen)
	return subtle.ConstantTimeCompare(sum, parsed.sum) == 1, nil
}

func formatPHC(salt, sum []byte) string {
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemoryKiB, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum),
	)
}

type phcHash struct {
	memory     uint32
	iterations uint32
	threads    uint8
	salt       []byte
	sum        []byte
}

func parsePHC(encoded string) (phcHash, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return phcHash{}, errors.New("invalid password hash")
	}
	parsed, err := parseArgonCost(parts[3])
	if err != nil {
		return phcHash{}, err
	}
	parsed.salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return phcHash{}, errors.New("invalid password hash")
	}
	parsed.sum, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(parsed.sum) != argonKeyLen || len(parsed.salt) != argonSaltLen {
		return phcHash{}, errors.New("invalid password hash")
	}
	return parsed, nil
}

func parseArgonCost(field string) (phcHash, error) {
	var memory, iterations uint32
	var threads uint8
	_, err := fmt.Sscanf(field, "m=%d,t=%d,p=%d", &memory, &iterations, &threads)
	if err != nil || memory == 0 || memory > maxArgonMemory || iterations == 0 || iterations > maxArgonTime {
		return phcHash{}, errors.New("invalid password hash")
	}
	if threads == 0 || threads > maxArgonThreads {
		return phcHash{}, errors.New("invalid password hash")
	}
	return phcHash{memory: memory, iterations: iterations, threads: threads}, nil
}
