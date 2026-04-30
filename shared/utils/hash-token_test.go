package utils

import (
	"testing"
)

func TestHMACHex_DeterministicAndKeyed(t *testing.T) {
	a := HMACHex("k", "msg")
	b := HMACHex("k", "msg")
	if a != b {
		t.Fatal("HMACHex must be deterministic for same key+payload")
	}
	if HMACHex("k1", "msg") == HMACHex("k2", "msg") {
		t.Fatal("HMACHex must differ when keys differ")
	}
	// Output is hex, 64 chars (sha256).
	if len(a) != 64 {
		t.Errorf("expected 64-char hex output, got %d", len(a))
	}
}

func TestHashToken_Stable(t *testing.T) {
	a := HashToken("opaque-token")
	if a != HashToken("opaque-token") {
		t.Fatal("HashToken must be stable for the same input")
	}
	if HashToken("a") == HashToken("b") {
		t.Fatal("HashToken collisions on trivial inputs — broken")
	}
	if len(a) != 64 {
		t.Errorf("expected sha256 hex (64), got %d", len(a))
	}
}

func TestNewOpaqueToken_UniqueAndNonEmpty(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 100; i++ {
		tok, err := NewOpaqueToken()
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if tok == "" {
			t.Fatal("empty token returned")
		}
		if _, dup := seen[tok]; dup {
			t.Fatal("duplicate token in 100 generations — RNG broken")
		}
		seen[tok] = struct{}{}
	}
}

func TestNewFamilyID_LooksLikeUUID(t *testing.T) {
	id := NewFamilyID()
	// UUID v4 textual form is 36 chars with 4 dashes.
	if len(id) != 36 {
		t.Errorf("expected 36-char UUID, got %q (len %d)", id, len(id))
	}
}
