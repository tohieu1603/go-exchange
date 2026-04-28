package grpc

import (
	"context"

	"github.com/cryptox/auth-service/internal/service"
	"github.com/cryptox/shared/proto/authpb"
	"github.com/cryptox/shared/utils"
)

type AuthGRPCServer struct {
	authpb.UnimplementedAuthServiceServer
	authSvc   *service.AuthService
	apiKeySvc *service.APIKeyService
	jwtSecret string
}

func NewAuthGRPCServer(authSvc *service.AuthService, apiKeySvc *service.APIKeyService, jwtSecret string) *AuthGRPCServer {
	return &AuthGRPCServer{authSvc: authSvc, apiKeySvc: apiKeySvc, jwtSecret: jwtSecret}
}

func (s *AuthGRPCServer) ValidateToken(ctx context.Context, req *authpb.ValidateTokenRequest) (*authpb.ValidateTokenResponse, error) {
	claims, err := utils.ValidateToken(req.Token, s.jwtSecret)
	if err != nil {
		return &authpb.ValidateTokenResponse{Valid: false}, nil
	}
	return &authpb.ValidateTokenResponse{
		Valid:  true,
		UserId: uint64(claims.UserID),
		Email:  claims.Email,
		Role:   claims.Role,
	}, nil
}

func (s *AuthGRPCServer) GetUser(ctx context.Context, req *authpb.GetUserRequest) (*authpb.UserResponse, error) {
	user, err := s.authSvc.GetUserByID(uint(req.UserId))
	if err != nil {
		return nil, err
	}
	return &authpb.UserResponse{
		Id:        uint64(user.ID),
		Email:     user.Email,
		FullName:  user.FullName,
		Role:      user.Role,
		KycStatus: user.KYCStatus,
		Is_2Fa:    user.Is2FA,
	}, nil
}

// Verify2FA returns whether a TOTP code is valid for the given user.
// Used by wallet-service before high-value actions like withdrawals.
//
// `required` is true when the user has 2FA enabled — caller must enforce
// the check; `valid` is meaningful only when required.
func (s *AuthGRPCServer) Verify2FA(ctx context.Context, req *authpb.Verify2FARequest) (*authpb.Verify2FAResponse, error) {
	user, err := s.authSvc.GetUserByID(uint(req.UserId))
	if err != nil {
		return &authpb.Verify2FAResponse{Valid: false, Required: false}, nil
	}
	if !user.Is2FA {
		// 2FA not enabled — caller may still want to enforce based on policy,
		// but we report Required=false so default behavior is to allow.
		return &authpb.Verify2FAResponse{Valid: true, Required: false}, nil
	}
	totp := service.NewTOTPService()
	return &authpb.Verify2FAResponse{
		Valid:    totp.ValidateCode(user.TwoFASecret, req.Code),
		Required: true,
	}, nil
}

// ValidateAPIKey verifies HMAC-SHA256 signed requests from algorithmic traders.
// Returns the resolved user, role, and permission CSV.
func (s *AuthGRPCServer) ValidateAPIKey(ctx context.Context, req *authpb.ValidateAPIKeyRequest) (*authpb.ValidateAPIKeyResponse, error) {
	if s.apiKeySvc == nil {
		return &authpb.ValidateAPIKeyResponse{Valid: false, Error: "api key validation not configured"}, nil
	}
	key, err := s.apiKeySvc.Authenticate(
		req.KeyId, req.Signature, req.Timestamp,
		req.Method, req.Path, req.Body, req.ClientIp,
	)
	if err != nil {
		return &authpb.ValidateAPIKeyResponse{Valid: false, Error: err.Error()}, nil
	}
	user, err := s.authSvc.GetUserByID(key.UserID)
	if err != nil || user.IsLocked {
		return &authpb.ValidateAPIKeyResponse{Valid: false, Error: "user not available"}, nil
	}
	return &authpb.ValidateAPIKeyResponse{
		Valid:       true,
		UserId:      uint64(key.UserID),
		Role:        user.Role,
		Permissions: key.Permissions,
	}, nil
}
