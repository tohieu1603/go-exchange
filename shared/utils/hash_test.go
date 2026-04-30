package utils

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword_RoundtripArgon2id(t *testing.T) {
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, argon2Prefix) {
		t.Fatalf("expected argon2id prefix, got %q", hash[:min(20, len(hash))])
	}
	if !CheckPassword("hunter2", hash) {
		t.Fatal("CheckPassword failed for valid password")
	}
	if CheckPassword("wrong", hash) {
		t.Fatal("CheckPassword accepted wrong password")
	}
}

func TestCheckPassword_LegacyBcrypt(t *testing.T) {
	// Simulate a row migrated from a pre-Argon2id era. Auto-detect must
	// still verify it correctly so existing users can log in.
	bh, err := bcrypt.GenerateFromPassword([]byte("legacy-pass"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	if !CheckPassword("legacy-pass", string(bh)) {
		t.Fatal("did not verify bcrypt hash via auto-detect")
	}
	if CheckPassword("wrong", string(bh)) {
		t.Fatal("auto-detect accepted wrong bcrypt password")
	}
}

func TestIsLegacyHash(t *testing.T) {
	bh, _ := bcrypt.GenerateFromPassword([]byte("x"), bcrypt.MinCost)
	if !IsLegacyHash(string(bh)) {
		t.Error("bcrypt hash should be flagged legacy")
	}
	a, _ := HashPassword("x")
	if IsLegacyHash(a) {
		t.Error("argon2 hash should not be flagged legacy")
	}
	if IsLegacyHash("") {
		t.Error("empty string must not be flagged legacy (avoids spurious migrations)")
	}
}

func TestCheckPassword_RejectsCorruptedArgon2(t *testing.T) {
	// Truncated/garbled argon2 hashes must not panic and must return false.
	cases := []string{
		argon2Prefix,
		argon2Prefix + "v=19$x$y",
		argon2Prefix + "garbage",
	}
	for _, h := range cases {
		if CheckPassword("anything", h) {
			t.Errorf("expected false for corrupted hash %q", h)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
