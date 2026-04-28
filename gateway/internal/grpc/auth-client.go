package grpc

import (
	"context"

	"github.com/cryptox/shared/grpcutil"
	"github.com/cryptox/shared/proto/authpb"
)

// AuthClient wraps the gRPC AuthService client.
type AuthClient struct {
	client authpb.AuthServiceClient
}

// NewAuthClient dials the Auth Service gRPC endpoint and returns a client.
func NewAuthClient(addr string) *AuthClient {
	conn := grpcutil.Dial(addr)
	return &AuthClient{client: authpb.NewAuthServiceClient(conn)}
}

// ValidateToken calls Auth Service to validate the given JWT token.
func (c *AuthClient) ValidateToken(ctx context.Context, token string) (*authpb.ValidateTokenResponse, error) {
	return c.client.ValidateToken(ctx, &authpb.ValidateTokenRequest{Token: token})
}

// ValidateAPIKey calls Auth Service to verify an HMAC-signed API request.
// Returns user ID, role, and permission CSV on success.
func (c *AuthClient) ValidateAPIKey(ctx context.Context, keyID, signature, timestamp, method, path string, body []byte, clientIP string) (*authpb.ValidateAPIKeyResponse, error) {
	return c.client.ValidateAPIKey(ctx, &authpb.ValidateAPIKeyRequest{
		KeyId:     keyID,
		Signature: signature,
		Timestamp: timestamp,
		Method:    method,
		Path:      path,
		Body:      body,
		ClientIp:  clientIP,
	})
}
