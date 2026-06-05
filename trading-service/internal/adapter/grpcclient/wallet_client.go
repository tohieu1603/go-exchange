// Package grpcclient holds outbound gRPC adapters implementing application ports.
package grpcclient

import (
	"context"
	"fmt"

	"github.com/cryptox/shared/grpcutil"
	"github.com/cryptox/shared/proto/walletpb"
)

// WalletClient implements application.WalletGateway (balance pre-check + cache warm).
type WalletClient struct {
	client walletpb.WalletServiceClient
}

func NewWalletClient(addr string) *WalletClient {
	return &WalletClient{client: walletpb.NewWalletServiceClient(grpcutil.Dial(addr))}
}

func (c *WalletClient) CheckBalance(ctx context.Context, userID uint, currency string, needed float64) error {
	resp, err := c.client.CheckBalance(ctx, &walletpb.CheckBalanceRequest{UserId: uint64(userID), Currency: currency, Needed: needed})
	if err != nil {
		return err
	}
	if !resp.Sufficient {
		return fmt.Errorf("insufficient balance")
	}
	return nil
}
