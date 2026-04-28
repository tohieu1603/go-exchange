package utils

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"github.com/google/uuid"
)

// HMACHex returns hex-encoded HMAC-SHA256 of `payload` keyed by `secret`.
// Used for API key request signing.
func HMACHex(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// HashToken returns SHA-256 hex of a raw token string. Cheap and deterministic;
// used to index tokens in DB without exposing raw values.
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// NewOpaqueToken returns a 32-byte cryptographically random token, base64url-encoded.
// Used for refresh tokens and API key secrets — no payload, only opaque random bytes.
func NewOpaqueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewFamilyID returns a UUIDv4 string used to group rotation chains of refresh tokens.
func NewFamilyID() string {
	return uuid.NewString()
}
