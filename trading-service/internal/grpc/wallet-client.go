package grpc

import (
	"context"
	"fmt"

	"github.com/cryptox/shared/grpcutil"
	"github.com/cryptox/shared/proto/walletpb"
)

// WalletClient wraps the gRPC wallet service client.
// Used for pre-check and lock operations that need account-level validation.
type WalletClient struct {
	client walletpb.WalletServiceClient
}

func NewWalletClient(addr string) *WalletClient {
	conn := grpcutil.Dial(addr)
	return &WalletClient{client: walletpb.NewWalletServiceClient(conn)}
}

func (c *WalletClient) CheckBalance(ctx context.Context, userID uint, currency string, needed float64) error {
	resp, err := c.client.CheckBalance(ctx, &walletpb.CheckBalanceRequest{
		UserId: uint64(userID), Currency: currency, Needed: needed,
	})
	if err != nil {
		return err
	}
	if !resp.Sufficient {
		return fmt.Errorf("insufficient balance")
	}
	return nil
}

func (c *WalletClient) LockBalance(ctx context.Context, userID uint, currency string, amount float64) error {
	resp, err := c.client.Lock(ctx, &walletpb.LockRequest{
		UserId: uint64(userID), Currency: currency, Amount: amount,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("lock failed")
	}
	return nil
}

func (c *WalletClient) UnlockBalance(ctx context.Context, userID uint, currency string, amount float64) error {
	_, err := c.client.Unlock(ctx, &walletpb.UnlockRequest{
		UserId: uint64(userID), Currency: currency, Amount: amount,
	})
	return err
}
