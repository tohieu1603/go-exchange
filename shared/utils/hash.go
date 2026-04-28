package utils

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// Password hashing strategy:
//
//   New writes  → Argon2id (RFC 9106) parameters chosen for ~50ms work cost on
//                  modern x86_64 hardware. Resistant to GPU/ASIC brute force.
//   Reads       → format-detect then dispatch:
//                  • prefix "$argon2id$"  → Argon2id verify
//                  • anything else        → bcrypt verify (legacy)
//   Migration   → on successful login with a bcrypt hash, the caller (auth-service)
//                  rehashes with Argon2id and persists, transparently upgrading
//                  every active user over their next login cycle.

const (
	argon2Time    uint32 = 2
	argon2Memory  uint32 = 64 * 1024 // 64 MB
	argon2Threads uint8  = 4
	argon2KeyLen  uint32 = 32
	argon2SaltLen        = 16

	argon2Prefix = "$argon2id$"
)

// HashPassword returns an Argon2id-encoded hash of the password.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	encoded := fmt.Sprintf(
		"%sv=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Prefix, argon2.Version,
		argon2Memory, argon2Time, argon2Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
	return encoded, nil
}

// CheckPassword verifies password against a hash. Auto-detects format.
func CheckPassword(password, hash string) bool {
	if strings.HasPrefix(hash, argon2Prefix) {
		return verifyArgon2(password, hash)
	}
	// Legacy bcrypt path.
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// IsLegacyHash returns true for bcrypt-style hashes that should be migrated
// to Argon2id on next successful login.
func IsLegacyHash(hash string) bool {
	return !strings.HasPrefix(hash, argon2Prefix) && hash != ""
}

func verifyArgon2(password, encoded string) bool {
	rest := strings.TrimPrefix(encoded, argon2Prefix)
	parts := strings.Split(rest, "$")
	// expect: v=…  m=…,t=…,p=…  saltB64  keyB64
	if len(parts) != 4 {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[0], "v=%d", &version); err != nil {
		return false
	}
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[1], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ErrPasswordTooShort returned for empty inputs (defensive — handlers also validate).
var ErrPasswordTooShort = errors.New("password too short")
