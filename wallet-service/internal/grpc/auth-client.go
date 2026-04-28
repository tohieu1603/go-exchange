package grpc

import (
	"context"

	"github.com/cryptox/shared/grpcutil"
	"github.com/cryptox/shared/proto/authpb"
)

// AuthClient calls auth-service for cross-service security checks (e.g.
// TOTP verification before a withdrawal).
type AuthClient struct {
	client authpb.AuthServiceClient
}

func NewAuthClient(addr string) *AuthClient {
	conn := grpcutil.Dial(addr)
	return &AuthClient{client: authpb.NewAuthServiceClient(conn)}
}

// Verify2FA returns (valid, required, err). When 2FA is not enabled for the
// user, Required=false — callers may decide whether to allow the action
// without a code based on their own policy (here: deny withdrawal w/o 2FA).
func (c *AuthClient) Verify2FA(ctx context.Context, userID uint, code string) (valid, required bool, err error) {
	resp, e := c.client.Verify2FA(ctx, &authpb.Verify2FARequest{
		UserId: uint64(userID), Code: code,
	})
	if e != nil {
		return false, false, e
	}
	return resp.Valid, resp.Required, nil
}
