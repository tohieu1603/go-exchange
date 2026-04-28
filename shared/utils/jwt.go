package utils

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID uint   `json:"userId"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// AccessTokenTTL is the lifetime of an access token.
// Short — refresh rotation handles longer sessions.
const AccessTokenTTL = 15 * time.Minute

// RefreshTokenTTL is the maximum age of a refresh token (server-side).
const RefreshTokenTTL = 7 * 24 * time.Hour

// GenerateToken — backward-compat alias for GenerateAccessToken.
func GenerateToken(userID uint, email, role, secret string) (string, error) {
	return GenerateAccessToken(userID, email, role, secret)
}

// GenerateAccessToken creates a short-lived access token (15 min).
func GenerateAccessToken(userID uint, email, role, secret string) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(AccessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GenerateTempToken creates a short-lived token scoped for 2FA verification only (5 min).
// Role is set to "2FA_PENDING" so it cannot be used as a regular access token.
func GenerateTempToken(userID uint, email, secret string) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   "2FA_PENDING",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateToken parses and validates an access token.
// Rejects 2FA_PENDING temp tokens — those go through ValidateTempToken.
func ValidateToken(tokenStr, secret string) (*Claims, error) {
	claims, err := parseClaims(tokenStr, secret)
	if err != nil {
		return nil, err
	}
	if claims.Role == "2FA_PENDING" {
		return nil, errors.New("2FA verification required")
	}
	return claims, nil
}

// ValidateTempToken parses a 2FA temp token. Rejects anything else.
func ValidateTempToken(tokenStr, secret string) (*Claims, error) {
	claims, err := parseClaims(tokenStr, secret)
	if err != nil {
		return nil, err
	}
	if claims.Role != "2FA_PENDING" {
		return nil, errors.New("not a 2FA temp token")
	}
	return claims, nil
}

func parseClaims(tokenStr, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
