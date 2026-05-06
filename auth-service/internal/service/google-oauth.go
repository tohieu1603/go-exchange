package service

import (
	"context"
	"errors"
	"fmt"
	"os"

	"google.golang.org/api/idtoken"
)

// GoogleClaims is the trimmed payload we need from a verified Google ID token.
type GoogleClaims struct {
	Sub           string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
}

// VerifyGoogleIDToken validates a Google-issued ID token (the `credential`
// returned by Google Identity Services) against the configured client ID.
// Returns the trimmed claims on success.
//
// GOOGLE_CLIENT_ID env must be set; otherwise returns a clear error so
// misconfiguration surfaces at first call instead of silently accepting tokens.
func VerifyGoogleIDToken(ctx context.Context, rawToken string) (*GoogleClaims, error) {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	if clientID == "" {
		return nil, errors.New("GOOGLE_CLIENT_ID not configured")
	}
	payload, err := idtoken.Validate(ctx, rawToken, clientID)
	if err != nil {
		return nil, fmt.Errorf("invalid google token: %w", err)
	}
	getStr := func(k string) string {
		if v, ok := payload.Claims[k].(string); ok {
			return v
		}
		return ""
	}
	getBool := func(k string) bool {
		if v, ok := payload.Claims[k].(bool); ok {
			return v
		}
		return false
	}
	return &GoogleClaims{
		Sub:           payload.Subject,
		Email:         getStr("email"),
		EmailVerified: getBool("email_verified"),
		Name:          getStr("name"),
		Picture:       getStr("picture"),
	}, nil
}
