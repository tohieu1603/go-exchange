// Package grpcclient holds outbound gRPC adapters implementing application ports.
package grpcclient

import (
	"context"
	"fmt"

	"github.com/cryptox/shared/grpcutil"
	"github.com/cryptox/shared/proto/walletpb"
)

// WalletClient implements application.WalletGateway over gRPC to wallet-service.
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

func (c *WalletClient) Deduct(ctx context.Context, userID uint, currency string, amount float64) error {
	_, err := c.client.Deduct(ctx, &walletpb.DeductRequest{UserId: uint64(userID), Currency: currency, Amount: amount})
	return err
}

func (c *WalletClient) Credit(ctx context.Context, userID uint, currency string, amount float64) error {
	_, err := c.client.Credit(ctx, &walletpb.CreditRequest{UserId: uint64(userID), Currency: currency, Amount: amount})
	return err
}

func (c *WalletClient) Lock(ctx context.Context, userID uint, currency string, amount float64) error {
	resp, err := c.client.Lock(ctx, &walletpb.LockRequest{UserId: uint64(userID), Currency: currency, Amount: amount})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("insufficient balance to lock")
	}
	return nil
}

func (c *WalletClient) Unlock(ctx context.Context, userID uint, currency string, amount float64) error {
	_, err := c.client.Unlock(ctx, &walletpb.UnlockRequest{UserId: uint64(userID), Currency: currency, Amount: amount})
	return err
}

func (c *WalletClient) GetBalance(ctx context.Context, userID uint, currency string) (balance, locked float64, err error) {
	resp, err := c.client.GetBalance(ctx, &walletpb.GetBalanceRequest{UserId: uint64(userID), Currency: currency})
	if err != nil {
		return 0, 0, err
	}
	return resp.Balance, resp.Locked, nil
}
