package grpc

import (
	"context"
	"fmt"

	"github.com/cryptox/shared/grpcutil"
	"github.com/cryptox/shared/proto/walletpb"
)

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

func (c *WalletClient) Deduct(ctx context.Context, userID uint, currency string, amount float64) error {
	_, err := c.client.Deduct(ctx, &walletpb.DeductRequest{
		UserId: uint64(userID), Currency: currency, Amount: amount,
	})
	return err
}

func (c *WalletClient) Credit(ctx context.Context, userID uint, currency string, amount float64) error {
	_, err := c.client.Credit(ctx, &walletpb.CreditRequest{
		UserId: uint64(userID), Currency: currency, Amount: amount,
	})
	return err
}

// GetBalance returns (available, locked) for a user/currency.
// Used by liquidation-engine to compute equity without touching wallet's DB.
func (c *WalletClient) GetBalance(ctx context.Context, userID uint, currency string) (balance, locked float64, err error) {
	resp, err := c.client.GetBalance(ctx, &walletpb.GetBalanceRequest{
		UserId: uint64(userID), Currency: currency,
	})
	if err != nil {
		return 0, 0, err
	}
	return resp.Balance, resp.Locked, nil
}
