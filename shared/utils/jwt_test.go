package utils

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-do-not-use-in-prod"

func TestGenerateAndValidateAccessToken_Roundtrip(t *testing.T) {
	tok, err := GenerateAccessToken(42, "alice@example.com", "USER", testSecret)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if tok == "" {
		t.Fatal("empty token")
	}
	claims, err := ValidateToken(tok, testSecret)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.UserID != 42 {
		t.Errorf("userID: want 42 got %d", claims.UserID)
	}
	if claims.Email != "alice@example.com" {
		t.Errorf("email: want alice@example.com got %s", claims.Email)
	}
	if claims.Role != "USER" {
		t.Errorf("role: want USER got %s", claims.Role)
	}
}

func TestValidateToken_RejectsWrongSecret(t *testing.T) {
	tok, _ := GenerateAccessToken(1, "x@y", "USER", testSecret)
	if _, err := ValidateToken(tok, "different-secret"); err == nil {
		t.Fatal("expected error for wrong secret, got nil")
	}
}

func TestValidateToken_RejectsTempToken(t *testing.T) {
	// Temp (2FA-pending) tokens MUST NOT validate as access tokens.
	tok, _ := GenerateTempToken(7, "x@y", testSecret)
	if _, err := ValidateToken(tok, testSecret); err == nil {
		t.Fatal("ValidateToken accepted 2FA temp token; should reject")
	}
}

func TestValidateTempToken_RejectsAccessToken(t *testing.T) {
	// Symmetric: an access token must not pass the temp-token gate.
	tok, _ := GenerateAccessToken(1, "x@y", "USER", testSecret)
	if _, err := ValidateTempToken(tok, testSecret); err == nil {
		t.Fatal("ValidateTempToken accepted regular access token; should reject")
	}
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	// Forge an already-expired token using the jwt lib directly so we don't
	// have to sleep in the test.
	claims := Claims{
		UserID: 1, Email: "x@y", Role: "USER",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Minute)),
		},
	}
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if _, err := ValidateToken(tok, testSecret); err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestValidateToken_MalformedInput(t *testing.T) {
	cases := []string{"", "garbage", "a.b.c", strings.Repeat("x", 200)}
	for _, in := range cases {
		if _, err := ValidateToken(in, testSecret); err == nil {
			t.Errorf("expected error for input %q, got nil", in)
		}
	}
}
