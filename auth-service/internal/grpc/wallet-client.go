package grpc

import (
	"context"

	"github.com/cryptox/shared/grpcutil"
	"github.com/cryptox/shared/proto/walletpb"
)

// WalletClient is a thin gRPC wrapper used by auth-service for referral payouts.
// Auth-service does NOT own wallets, so it must call wallet-service.
type WalletClient struct {
	client walletpb.WalletServiceClient
}

func NewWalletClient(addr string) *WalletClient {
	conn := grpcutil.Dial(addr)
	return &WalletClient{client: walletpb.NewWalletServiceClient(conn)}
}

// Credit adds amount to a user's wallet via wallet-service.
func (c *WalletClient) Credit(ctx context.Context, userID uint, currency string, amount float64) error {
	_, err := c.client.Credit(ctx, &walletpb.CreditRequest{
		UserId: uint64(userID), Currency: currency, Amount: amount,
	})
	return err
}
